package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
	"github.com/subguard/backend/internal/tgutil"
	"github.com/subguard/backend/internal/worker"
)

// Callback prefixes imported from tgutil — shared with worker package.

// Setup initializes the Telegram bot with handlers and returns the bot instance.
//
// appCtx is the server lifecycle context — every background goroutine the
// bot spawns (broadcast, async CSV export, etc.) must derive its own ctx
// from this so SIGTERM cancels them in time for the shutdown drain. wg
// is the same WaitGroup main.go waits on so those goroutines are part of
// the orderly drain instead of leaking past Close() of the DB/Redis pools.
func Setup(
	cfg *config.Config,
	db *gorm.DB,
	notifWorker *worker.NotificationWorker,
	rdb *redis.Client,
	appCtx context.Context,
	wg *sync.WaitGroup,
) (*tgbot.Bot, error) {
	adminRepo := repository.NewAdminRepo(db)
	panel := newAdminPanel(cfg, db, rdb, appCtx, wg)

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			// Route admin FSM text input before dropping unknown updates.
			if update.Message != nil && update.Message.From != nil && cfg.IsAdmin(update.Message.From.ID) {
				state := panel.getState(ctx, update.Message.From.ID)
				if state != stateNone {
					// Text messages go through the regular FSM text router.
					if update.Message.Text != "" {
						panel.handleText(ctx, b, update)
						return
					}
					// Non-text messages (photo/video/document/audio/sticker)
					// are only relevant for the broadcast content capture step.
					if state == stateAwaitBroadcastMsg {
						panel.broadcast.handleBroadcastContent(ctx, b, update)
						return
					}
				}
			}
		}),
	}

	b, err := tgbot.New(cfg.BotToken, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create bot: %w", err)
	}

	// /start command — handles regular starts and deep links
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "/start", tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleStart(ctx, b, update, cfg, db, adminRepo)
		},
	)

	// Inline-button callbacks from the reminder keyboard — see
	// internal/worker/notification.go for the buttons that produce these.
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, tgutil.RenewCallbackPrefix, tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleRenewCallback(ctx, b, update, db)
		},
	)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, tgutil.CancelCallbackPrefix, tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleCancelCallback(ctx, b, update, db)
		},
	)

	// /force_notify — admin-only test command that runs the notification
	// worker for the calling admin, ignoring time checks and dedup.
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "/force_notify", tgbot.MatchTypeExact,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleForceNotify(ctx, b, update, cfg, db, notifWorker)
		},
	)

	// ── Admin panel ────────────────────────────────────
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "/admin", tgbot.MatchTypeExact,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			panel.handleAdminCommand(ctx, b, update)
		},
	)
	b.RegisterHandler(tgbot.HandlerTypeMessageText, "/cancel", tgbot.MatchTypeExact,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			panel.handleCancel(ctx, b, update)
		},
	)
	// All admin_ callbacks routed through the panel
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "admin_", tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			panel.handleCallback(ctx, b, update)
		},
	)

	// ── Churn tracking ────────────────────────────────────
	// Telegram sends `my_chat_member` updates whenever a user blocks or
	// unblocks the bot. The go-telegram/bot lib does NOT expose a
	// HandlerType enum for these (the enum covers messages and callbacks
	// only), so we route via RegisterHandlerMatchFunc — fires on every
	// update whose MyChatMember field is non-nil.
	b.RegisterHandlerMatchFunc(
		func(update *models.Update) bool { return update.MyChatMember != nil },
		func(ctx context.Context, _ *tgbot.Bot, update *models.Update) {
			handleMyChatMember(ctx, update, db)
		},
	)

	return b, nil
}

// handleMyChatMember toggles users.is_active when a user blocks/unblocks
// the bot, so admin stats can compute churn rate without polling Telegram
// for every user's reachability.
//
// Telegram fires `my_chat_member` in three contexts; we only care about
// the first:
//
//  1. Private 1-1 chat (chat.type == "private") — the user blocked or
//     unblocked our bot. NewChatMember.Type == "kicked" (block) or
//     "member" (unblock).
//  2. Group / supergroup — the bot was added/removed/promoted. Not user
//     churn, ignored.
//  3. Channel — same, ignored.
//
// On unblock the user usually also fires /start, which handleStart will
// re-confirm is_active=true. We still process the my_chat_member path
// because a user can unblock without re-/starting (just unmute the chat).
//
// Status extraction is double-belted. The go-telegram lib has a custom
// UnmarshalJSON that fills the ChatMember.Type field from the Telegram
// wire "status" key. If for some reason that path fails (lib regression,
// future Telegram status code we don't know about), we re-marshal the
// NewChatMember and read the raw "status" string ourselves. Both paths
// reach the same decision tree below; the JSON fallback only kicks in
// when Type is empty.
func handleMyChatMember(ctx context.Context, update *models.Update, db *gorm.DB) {
	cmu := update.MyChatMember
	if cmu == nil {
		return
	}

	// One entry log per update — invaluable when prod looks idle and you
	// need to verify Telegram is actually delivering the events.
	status := string(cmu.NewChatMember.Type)
	if status == "" {
		status = extractStatusFromJSON(cmu.NewChatMember)
	}
	log.Printf("[bot/my_chat_member] event chat_type=%s from_tg=%d status=%q",
		cmu.Chat.Type, cmu.From.ID, status)

	if cmu.Chat.Type != "private" {
		return // group/supergroup/channel — not user churn
	}
	tgID := cmu.From.ID
	if tgID == 0 {
		log.Printf("[bot/my_chat_member] skip: from_tg=0 (malformed update)")
		return
	}

	var isActive bool
	switch status {
	case string(models.ChatMemberTypeBanned): // "kicked"
		isActive = false
	case string(models.ChatMemberTypeMember):
		isActive = true
	default:
		// "left", "restricted", "administrator", "creator" — not meaningful
		// for user-bot reachability in a private chat. Empty (status
		// extraction failed) also falls here.
		log.Printf("[bot/my_chat_member] skip tg=%d: unhandled status=%q", tgID, status)
		return
	}

	res := db.WithContext(ctx).Model(&model.User{}).
		Where("telegram_id = ?", tgID).
		Update("is_active", isActive)
	if res.Error != nil {
		log.Printf("[bot/my_chat_member] DB update tg=%d active=%v: %v", tgID, isActive, res.Error)
		return
	}
	log.Printf("[bot/my_chat_member] DB updated tg=%d is_active=%v status=%s rows=%d",
		tgID, isActive, status, res.RowsAffected)
}

// extractStatusFromJSON is the belt-and-suspenders fallback for reading
// NewChatMember.status when the library's UnmarshalJSON didn't populate
// the typed field. We re-marshal the (already-unmarshalled-once)
// ChatMember struct and pull the "status" key from the resulting JSON.
//
// On any error we return "" rather than propagating — the caller logs
// the empty status and skips the update, which is the safe default.
func extractStatusFromJSON(cm models.ChatMember) string {
	raw, err := json.Marshal(cm)
	if err != nil {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	if s, ok := m["status"].(string); ok {
		return s
	}
	return ""
}

func handleStart(ctx context.Context, b *tgbot.Bot, update *models.Update, cfg *config.Config, db *gorm.DB, adminRepo *repository.AdminRepo) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	tgUser := update.Message.From

	// Upsert user — Unscoped to find soft-deleted users too.
	var user model.User
	isNewUser := false
	err := db.Unscoped().Where("telegram_id = ?", tgUser.ID).First(&user).Error

	switch {
	case err == nil:
		// User exists (active or soft-deleted). Restore if needed.
		if user.DeletedAt.Valid {
			db.Unscoped().Model(&user).Update("deleted_at", gorm.Expr("NULL"))
			user.DeletedAt.Valid = false
		}
		// Force is_active=true on every /start so a user who blocked the
		// bot and is now back (the unblock fires a separate my_chat_member
		// update, but those can be missed on webhook outage) is counted as
		// retained again, not churned. No-op when already true.
		if !user.IsActive {
			db.Model(&user).Update("is_active", true)
			user.IsActive = true
		}

	case errors.Is(err, gorm.ErrRecordNotFound):
		isNewUser = true
		user = model.User{
			TelegramID: tgUser.ID,
			FirstName:  tgUser.FirstName,
			LastName:   tgUser.LastName,
			Username:   tgUser.Username,
		}
		if createErr := db.Create(&user).Error; createErr != nil {
			// Unique constraint race — another /start created the row.
			log.Printf("[bot/start] create failed tg=%d: %v, fallback lookup", tgUser.ID, createErr)
			isNewUser = false
			if fallbackErr := db.Unscoped().Where("telegram_id = ?", tgUser.ID).First(&user).Error; fallbackErr != nil {
				log.Printf("[bot/start] fallback also failed tg=%d: %v", tgUser.ID, fallbackErr)
				return
			}
			if user.DeletedAt.Valid {
				db.Unscoped().Model(&user).Update("deleted_at", gorm.Expr("NULL"))
				user.DeletedAt.Valid = false
			}
		}

	default:
		log.Printf("[bot/start] db error looking up tg=%d: %v", tgUser.ID, err)
		return
	}

	// Silently ignore banned users
	if user.IsBanned {
		return
	}

	// Parse deep link parameters: /start room_ABC123 or /start ad_campaign_tag
	parts := strings.SplitN(update.Message.Text, " ", 2)
	if len(parts) == 2 {
		param := parts[1]

		// Track traffic campaign
		if strings.HasPrefix(param, "ad_") {
			tag := param
			adminRepo.IncrementCampaign(tag, "bot_starts")
			if isNewUser {
				log.Printf("[bot/start] new user tg=%d, crediting signup to campaign %s", tgUser.ID, tag)
				adminRepo.IncrementCampaign(tag, "auths")
			}
			// First-touch attribution — never overwrite
			if user.TrafficSourceID == "" {
				if err := db.Model(&user).Update("traffic_source_id", tag).Error; err != nil {
					log.Printf("[bot] failed to update traffic_source_id for %d: %v", user.TelegramID, err)
				}
			}
		}

		// Room invite deep link
		if strings.HasPrefix(param, "room_") {
			inviteCode := strings.TrimPrefix(param, "room_")
			b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID: chatID,
				Text:   fmt.Sprintf("🔗 Join room: open the app and enter code `%s`", inviteCode),
				ParseMode: "Markdown",
				ReplyMarkup: &models.InlineKeyboardMarkup{
					InlineKeyboard: [][]models.InlineKeyboardButton{{
						{Text: "Open SubGuard", WebApp: &models.WebAppInfo{URL: cfg.BaseURL}},
					}},
				},
			})
			return
		}
	}

	// Default welcome message
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "👋 Welcome to SubGuard!\nTrack and split your subscriptions easily.",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "🚀 Open App", WebApp: &models.WebAppInfo{URL: cfg.BaseURL}},
			}},
		},
	})
}

// handleRenewCallback processes the inline "Paid (Renew)" button attached
// to renewal-reminder messages. It advances the subscription's
// NextPaymentAt by its Period, clears notified_at so the worker will fire
// again before the next renewal, acks the callback (to drop Telegram's
// loading spinner), and rewrites the original message into a "✅ paid /
// next payment on…" confirmation so the user has a visible audit trail.
//
// Authorization: the callback's From.ID must match the user's stored
// telegram_id — prevents another Telegram user who happened to see a
// forwarded message from advancing someone else's subscription.
func handleRenewCallback(ctx context.Context, b *tgbot.Bot, update *models.Update, db *gorm.DB) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}

	answerAndLog := func(text string) {
		if _, err := b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            text,
		}); err != nil {
			log.Printf("[bot.renew] AnswerCallbackQuery error: %v", err)
		}
	}

	idStr := strings.TrimPrefix(cb.Data, tgutil.RenewCallbackPrefix)
	subID, err := uuid.Parse(idStr)
	if err != nil {
		answerAndLog("invalid subscription id")
		return
	}

	// Single JOINed read — was two sequential SELECTs (sub then owner).
	// Audit A7. Preload follows the Subscription.User belongs-to relation
	// already declared on the model. Owner is dereferenced below; nil
	// guard catches the (impossible-in-practice but defensive) case
	// where the sub exists but its user has been hard-deleted.
	var sub model.Subscription
	if err := db.WithContext(ctx).Preload("User").First(&sub, "id = ?", subID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			answerAndLog("subscription not found")
		} else {
			log.Printf("[bot.renew] lookup error: %v", err)
			answerAndLog("lookup failed")
		}
		return
	}
	owner := &sub.User
	if owner == nil || owner.ID == 0 {
		log.Printf("[bot.renew] orphan sub %s — owner missing", sub.ID)
		answerAndLog("forbidden")
		return
	}
	if owner.TelegramID != cb.From.ID {
		answerAndLog("not your subscription")
		return
	}

	prev := sub.NextPaymentAt
	next := advancePayment(prev, sub.Period)

	// Persist with an optimistic lock: only update if NextPaymentAt is
	// still `prev`. A racing duplicate callback (Telegram retry or user
	// double-tap before AnswerCallbackQuery acks) would otherwise read
	// the already-advanced date and shift another full period. Adding
	// `WHERE next_payment_at = ?` makes the SECOND update affect zero
	// rows; we detect via RowsAffected and treat as "already renewed".
	updates := map[string]interface{}{
		"next_payment_at": next,
		"notified_at":     nil,
	}
	if sub.IsTrial {
		updates["is_trial"] = false
		updates["trial_ends_at"] = nil // clear historical trial mark on conversion
	}
	result := db.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ? AND next_payment_at = ?", sub.ID, prev).
		Updates(updates)
	if result.Error != nil {
		log.Printf("[bot.renew] update error: %v", result.Error)
		answerAndLog("save failed")
		return
	}
	if result.RowsAffected == 0 {
		// Another invocation already advanced the date. Ack the spinner
		// and bail — don't re-edit the message either, it's already in a
		// confirmation state.
		answerAndLog(renewToastLabel(owner.Locale))
		return
	}
	sub.NextPaymentAt = next
	sub.IsTrial = false

	answerAndLog(renewToastLabel(owner.Locale))

	// Rewrite the original message so the chat history reads as a
	// confirmation, not a stale reminder. Failure to edit is non-fatal —
	// the renewal already persisted.
	if cb.Message.Message != nil {
		newText := renewConfirmationText(&sub, owner.Locale)
		if _, err := b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    cb.Message.Message.Chat.ID,
			MessageID: cb.Message.Message.ID,
			Text:      newText,
			ParseMode: "Markdown",
		}); err != nil {
			log.Printf("[bot.renew] edit message error: %v", err)
		}
	}
}

// handleCancelCallback processes the inline "Cancelled" button. Hard-
// deletes the subscription, acks the callback to drop the spinner, and
// rewrites the original reminder message into a "🗑 deleted" confirmation
// so the chat log doesn't show a stale renewal ping.
//
// Same owner-authorization gate as renew — a forwarded message can't be
// used to delete someone else's subscription.
func handleCancelCallback(ctx context.Context, b *tgbot.Bot, update *models.Update, db *gorm.DB) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}

	answerAndLog := func(text string) {
		if _, err := b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID,
			Text:            text,
		}); err != nil {
			log.Printf("[bot.cancel] AnswerCallbackQuery error: %v", err)
		}
	}

	idStr := strings.TrimPrefix(cb.Data, tgutil.CancelCallbackPrefix)
	subID, err := uuid.Parse(idStr)
	if err != nil {
		answerAndLog("invalid subscription id")
		return
	}

	// Single JOINed read — see renew callback above (audit A7).
	var sub model.Subscription
	if err := db.WithContext(ctx).Preload("User").First(&sub, "id = ?", subID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			answerAndLog("already deleted")
		} else {
			log.Printf("[bot.cancel] lookup error: %v", err)
			answerAndLog("lookup failed")
		}
		return
	}
	owner := &sub.User
	if owner == nil || owner.ID == 0 {
		log.Printf("[bot.cancel] orphan sub %s — owner missing", sub.ID)
		answerAndLog("forbidden")
		return
	}
	if owner.TelegramID != cb.From.ID {
		answerAndLog("not your subscription")
		return
	}

	// Hard delete — Subscription has no DeletedAt column and we'd otherwise
	// need the auth path to filter on it. Matches the existing
	// DELETE /api/v1/subscriptions/:id repository call.
	if err := db.WithContext(ctx).
		Where("id = ? AND user_id = ?", sub.ID, owner.ID).
		Delete(&model.Subscription{}).Error; err != nil {
		log.Printf("[bot.cancel] delete error: %v", err)
		answerAndLog("delete failed")
		return
	}

	answerAndLog(cancelToastLabel(owner.Locale))

	if cb.Message.Message != nil {
		var newText string
		if sub.IsTrial {
			newText = cancelConfirmationTextForTrial(&sub, owner.Locale)
		} else {
			newText = cancelConfirmationText(&sub, owner.Locale)
		}
		if _, err := b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    cb.Message.Message.Chat.ID,
			MessageID: cb.Message.Message.ID,
			Text:      newText,
			ParseMode: "Markdown",
		}); err != nil {
			log.Printf("[bot.cancel] edit message error: %v", err)
		}
	}
}

// advancePayment moves a payment date forward by one billing period.
// Defaults to monthly for unknown / missing period strings.
func advancePayment(from time.Time, period string) time.Time {
	switch period {
	case "yearly":
		return from.AddDate(1, 0, 0)
	case "weekly":
		return from.AddDate(0, 0, 7)
	default: // monthly + fallback
		return from.AddDate(0, 1, 0)
	}
}

func renewToastLabel(locale string) string {
	if locale == "ru" {
		return "✅ Дата обновлена"
	}
	return "✅ Renewed"
}

func cancelToastLabel(locale string) string {
	if locale == "ru" {
		return "🗑 Подписка удалена"
	}
	return "🗑 Subscription removed"
}

func cancelConfirmationText(sub *model.Subscription, locale string) string {
	name := escapeMarkdownLite(sub.Name)
	if locale == "ru" {
		return fmt.Sprintf("🗑 Подписка *%s* удалена из вашего списка.", name)
	}
	return fmt.Sprintf("🗑 Subscription *%s* removed from your list.", name)
}

func renewConfirmationText(sub *model.Subscription, locale string) string {
	dateFmt := "2006-01-02"
	if locale == "ru" {
		dateFmt = "02.01.2006"
	}
	dateStr := sub.NextPaymentAt.Format(dateFmt)

	if locale == "ru" {
		return fmt.Sprintf("✅ Подписка *%s* оплачена. Следующее списание: %s.",
			escapeMarkdownLite(sub.Name), dateStr)
	}
	return fmt.Sprintf("✅ Subscription *%s* paid. Next payment: %s.",
		escapeMarkdownLite(sub.Name), dateStr)
}

func cancelConfirmationTextForTrial(sub *model.Subscription, locale string) string {
	name := escapeMarkdownLite(sub.Name)
	if locale == "ru" {
		return fmt.Sprintf("🗑 Пробный период *%s* отменён.", name)
	}
	return fmt.Sprintf("🗑 Trial for *%s* cancelled.", name)
}

// escapeMarkdownLite delegates to the shared tgutil package.
func escapeMarkdownLite(s string) string {
	return tgutil.EscapeMarkdown(s)
}

// handleForceNotify is an admin-only test command that forces the notification
// worker to run for the calling admin user, ignoring NotificationTime and
// the notified_at dedup window. This lets the admin test push delivery any
// time of day, as many times as needed.
func handleForceNotify(ctx context.Context, b *tgbot.Bot, update *models.Update, cfg *config.Config, db *gorm.DB, notifWorker *worker.NotificationWorker) {
	if update.Message == nil {
		return
	}

	chatID := update.Message.Chat.ID
	tgUser := update.Message.From

	// Admin gate — only admin Telegram IDs can use this command.
	if !cfg.IsAdmin(tgUser.ID) {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "⛔ This command is admin-only.",
		})
		return
	}

	// Find the admin's internal user record.
	var user model.User
	if err := db.WithContext(ctx).Where("telegram_id = ?", tgUser.ID).First(&user).Error; err != nil {
		log.Printf("[bot.force_notify] user lookup error: %v", err)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Failed to look up your user record.",
		})
		return
	}

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "🔧 Running test notification pass…",
	})

	sent, total, err := notifWorker.ForceNotifyUser(ctx, user.ID)
	if err != nil {
		log.Printf("[bot.force_notify] error: %v", err)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("❌ Error: %v", err),
		})
		return
	}

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   fmt.Sprintf("🔧 Тестовый прогон воркера завершен.\nПодписок на завтра: %d\nОтправлено: %d", total, sent),
	})
}

// SetWebhook registers the webhook URL with Telegram.
//
// AllowedUpdates is set explicitly even though Telegram's default already
// includes `my_chat_member`. Two reasons:
//
//  1. Intent visibility — anyone reading this code sees which update
//     types we depend on, no spelunking through Telegram docs needed.
//  2. Defence against silent API drift — Telegram has previously narrowed
//     defaults; making the list explicit means our churn tracking won't
//     mysteriously stop working if they do it again.
//
// `chat_member` (without `my_`) is deliberately omitted — it covers
// membership changes of OTHER users in groups, which we don't need and
// would 10× the webhook traffic on group bots.
func SetWebhook(b *tgbot.Bot, cfg *config.Config) error {
	webhookURL := cfg.BaseURL + "/webhook"
	log.Printf("[bot] setting webhook: %s", webhookURL)

	_, err := b.SetWebhook(context.Background(), &tgbot.SetWebhookParams{
		URL:         webhookURL,
		SecretToken: cfg.WebhookSecret,
		AllowedUpdates: []string{
			"message",
			"callback_query",
			"my_chat_member",
		},
	})
	return err
}
