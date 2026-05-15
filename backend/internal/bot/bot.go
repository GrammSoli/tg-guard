package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
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
			// ── Stars payment webhooks ────────────────────────
			// PreCheckoutQuery MUST be answered within 10 seconds or
			// the payment button spins indefinitely in the user's app.
			if update.PreCheckoutQuery != nil {
				handlePreCheckoutQuery(ctx, b, update)
				return
			}
			// SuccessfulPayment arrives inside a Message update.
			if update.Message != nil && update.Message.SuccessfulPayment != nil {
				handleSuccessfulPayment(ctx, b, update, db)
				return
			}

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

	// Language selection callback from the /start onboarding flow.
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "lang:", tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleLangCallback(ctx, b, update, cfg, db)
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
	// All admin_ callbacks routed through the panel.
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "admin_", tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			panel.handleCallback(ctx, b, update)
		},
	)
	// Pricing-menu callbacks use the short "pr_" prefix (pr_st_*, pr_cr_*)
	// — they are NOT admin_-prefixed, so they need their own registration
	// or the dispatcher never delivers them to handleCallback and the
	// button spins forever. Same panel.handleCallback target; its
	// internal switch already routes "pr_" to handlePriceCallback.
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, "pr_", tgbot.MatchTypePrefix,
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

	// Dirty-check the value at the SQL level so a no-op transition
	// (user already kicked, status=kicked re-fired — Telegram does this
	// occasionally on webhook retry) doesn't bump users.updated_at and
	// keep churning WAL. Audit O1. RowsAffected==0 now means "no state
	// change required"; we still log so the operator can see the event
	// arrived.
	res := db.WithContext(ctx).Model(&model.User{}).
		Where("telegram_id = ? AND is_active != ?", tgID, isActive).
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

	// Maintenance gate — keep the bot's text replies in sync with the
	// API's MaintenanceGuard and the WebApp stub. Admins bypass it so
	// they can still reach /start and /admin to flip the switch back
	// off. Placed before deep-link parsing so a room-invite or ad-link
	// /start during a window also gets the stub, not the join flow.
	if !cfg.IsAdmin(tgUser.ID) {
		if s, settErr := adminRepo.GetSettings(); settErr == nil && s.MaintenanceMode {
			// Prefer the user's stored locale (their explicit choice);
			// fall back to the Telegram client language for users who
			// haven't been through onboarding yet.
			locale := user.Locale
			if locale == "" {
				locale = tgUser.LanguageCode
			}
			b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID:    chatID,
				Text:      maintenanceMessage(locale),
				ParseMode: "HTML",
			})
			return
		}
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
			// Escape: inviteCode comes from a t.me/?start= deep-link param
			// — an attacker controls the string and could embed a backtick
			// to break out of the code-span below and inject Markdown.
			// Legitimate invite codes pass through unchanged (alphanumeric,
			// no special characters per the generator's alphabet).
			b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID:    chatID,
				Text:      fmt.Sprintf("🔗 Join room: open the app and enter code `%s`", tgutil.EscapeMarkdown(inviteCode)),
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

	// Step 1 of onboarding: language picker. The actual welcome message
	// is sent by handleLangCallback after the user picks a language.
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "🌍 Укажите ваш язык / Please choose your language:",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "🇷🇺 Русский", CallbackData: "lang:ru"},
				{Text: "🇬🇧 English", CallbackData: "lang:en"},
			}},
		},
	})
}

// maintenanceMessage is the localized "under maintenance" reply the bot
// sends non-admin users for /start while AppSettings.maintenance_mode is
// on — keeps the bot's text surface in sync with the API MaintenanceGuard
// and the WebApp's MaintenanceScreen. HTML parse mode (the <b> tag).
//
// English only for an explicit "en*" locale (covers "en", "en-US");
// everything else — "ru", any other code, or empty — falls back to
// Russian, the project's primary audience.
func maintenanceMessage(locale string) string {
	if strings.HasPrefix(locale, "en") {
		return "🛠 <b>Maintenance Break</b>\n\nWe are adding new features, we'll be right back ☕️"
	}
	return "🛠 <b>Технические работы</b>\n\nПрикручиваем новые фичи, скоро вернёмся ☕️"
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

// handleLangCallback processes the inline language-selection buttons from
// the /start onboarding flow. It persists the chosen locale to the DB,
// then edits the original message into a localized welcome with a WebApp
// button whose URL carries the ?lang= parameter so the React front-end
// can initialize i18n before the first render.
func handleLangCallback(ctx context.Context, b *tgbot.Bot, update *models.Update, cfg *config.Config, db *gorm.DB) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}

	// Ack the callback to dismiss the spinner.
	if _, err := b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
	}); err != nil {
		log.Printf("[bot.lang] AnswerCallbackQuery error: %v", err)
	}

	lang := strings.TrimPrefix(cb.Data, "lang:")
	if lang != "ru" && lang != "en" {
		lang = "en"
	}

	// Persist the language choice.
	if err := db.WithContext(ctx).Model(&model.User{}).
		Where("telegram_id = ?", cb.From.ID).
		Update("locale", lang).Error; err != nil {
		log.Printf("[bot.lang] DB update locale tg=%d lang=%s: %v", cb.From.ID, lang, err)
	}

	// Build WebApp URL with language parameter.
	webAppURL := cfg.BaseURL + "?lang=" + lang

	// Localized welcome text and button label.
	var text, btnLabel string
	if lang == "ru" {
		text = "🛡 <b>Добро пожаловать в SubGuard!</b>\n\n" +
			"Ваш умный трекер подписок. Мы поможем навести порядок в регулярных платежах и сэкономить деньги.\n\n" +
			"✨ <b>Что мы умеем:</b>\n" +
			"• Напоминать о списаниях до того, как они произойдут\n" +
			"• Делить стоимость подписок с друзьями и семьей\n" +
			"• Контролировать все расходы в одной удобной панели\n\n" +
			"Жмите кнопку ниже, чтобы запустить приложение и взять подписки под контроль 👇"
		btnLabel = "🚀 Открыть приложение"
	} else {
		text = "🛡 <b>Welcome to SubGuard!</b>\n\n" +
			"Your smart subscription tracker. We'll help you organize your recurring payments and save money.\n\n" +
			"✨ <b>What you can do:</b>\n" +
			"• Get notified before you get charged\n" +
			"• Split subscription costs with friends and family\n" +
			"• Track all expenses in one clean dashboard\n\n" +
			"Tap the button below to launch the app and take control of your subscriptions 👇"
		btnLabel = "🚀 Open App"
	}

	// Edit the original language-picker message into the welcome.
	if cb.Message.Message != nil {
		if _, err := b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    cb.Message.Message.Chat.ID,
			MessageID: cb.Message.Message.ID,
			Text:      text,
			ParseMode: "HTML",
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: btnLabel, WebApp: &models.WebAppInfo{URL: webAppURL}},
				}},
			},
		}); err != nil {
			log.Printf("[bot.lang] edit message error: %v", err)
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
			"pre_checkout_query",
		},
	})
	return err
}

// ── Stars Payment Handlers ──────────────────────────────────────────

// handlePreCheckoutQuery auto-confirms every pre-checkout query from
// Telegram. For Telegram Stars (XTR) there is no shipping or provider
// validation to perform — the bot MUST answer within 10 seconds or the
// payment button hangs indefinitely in the user's client.
func handlePreCheckoutQuery(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	pcq := update.PreCheckoutQuery
	if pcq == nil {
		return
	}
	log.Printf("[bot/pre_checkout] query=%s payload=%q from_tg=%d amount=%d %s",
		pcq.ID, pcq.InvoicePayload, pcq.From.ID, pcq.TotalAmount, pcq.Currency)

	if _, err := b.AnswerPreCheckoutQuery(ctx, &tgbot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: pcq.ID,
		OK:                 true,
	}); err != nil {
		log.Printf("[bot/pre_checkout] AnswerPreCheckoutQuery error: %v", err)
	}
}

// handleSuccessfulPayment processes a completed Stars payment. It:
//  1. Parses the invoice payload to extract the internal user ID.
//  2. Deduplicates via TelegramPaymentChargeID (Telegram retries on
//     non-200 / slow responses, so we'd otherwise create duplicate
//     Donation rows and spam the user with "thank you" messages).
//  3. Activates premium (is_donator = true) and logs a Donation row.
//  4. Sends a localized congratulation message to the user.
func handleSuccessfulPayment(ctx context.Context, b *tgbot.Bot, update *models.Update, db *gorm.DB) {
	msg := update.Message
	if msg == nil || msg.SuccessfulPayment == nil {
		return
	}
	payment := msg.SuccessfulPayment

	// ── 📥 Entry point ──────────────────────────────────────────
	log.Printf("📥 [bot/payment] Received SuccessfulPayment webhook. Payload: %q, ChargeID: %s, Amount: %d %s, From TG: %d",
		payment.InvoicePayload, payment.TelegramPaymentChargeID,
		payment.TotalAmount, payment.Currency, msg.From.ID)

	// ── Step 1: Safe payload parsing ──────────────────────────
	// Format: "premium_stars_<plan>_<userID>" (4 parts). The legacy
	// 3-part "premium_stars_<userID>" is still accepted as a lifetime
	// grant so invoices created before this deploy still resolve.
	if !strings.HasPrefix(payment.InvoicePayload, "premium_stars_") {
		log.Printf("❌ [bot/payment] Unknown payload prefix=%q, ignoring", payment.InvoicePayload)
		return
	}
	parts := strings.Split(payment.InvoicePayload, "_")
	var plan, uidStr string
	switch len(parts) {
	case 4: // premium / stars / plan / uid
		plan, uidStr = parts[2], parts[3]
		if plan != "month" {
			plan = "lifetime"
		}
	case 3: // legacy premium / stars / uid
		plan, uidStr = "lifetime", parts[2]
	default:
		log.Printf("❌ [bot/payment] Malformed payload=%q (got %d parts)", payment.InvoicePayload, len(parts))
		return
	}
	userID, err := strconv.ParseUint(uidStr, 10, 64)
	if err != nil {
		log.Printf("❌ [bot/payment] Failed to parse user ID from %q: %v", uidStr, err)
		return
	}
	log.Printf("✅ [bot/payment] Parsed plan=%s UserID=%d", plan, userID)

	// ── Step 2: Idempotency — dedup by TelegramPaymentChargeID ──
	chargeID := payment.TelegramPaymentChargeID
	if chargeID == "" {
		log.Printf("❌ [bot/payment] Empty charge ID, ignoring")
		return
	}
	var existingDonation model.Donation
	if err := db.WithContext(ctx).
		Where("telegram_payment_charge_id = ?", chargeID).
		First(&existingDonation).Error; err == nil {
		log.Printf("⚠️ [bot/payment] Duplicate charge=%s for user=%d, already processed — skipping", chargeID, userID)
		return
	}
	log.Printf("🆕 [bot/payment] Charge %s is new, proceeding", chargeID)

	// ── Step 3: Find user and activate premium ──────────────────
	var user model.User
	if err := db.WithContext(ctx).First(&user, "id = ?", uint(userID)).Error; err != nil {
		log.Printf("❌ [bot/payment] User with ID=%d (uint=%d) not found in DB: %v", userID, uint(userID), err)
		return
	}
	log.Printf("👤 [bot/payment] Found user: ID=%d, TelegramID=%d, IsDonator=%v, Locale=%q",
		user.ID, user.TelegramID, user.IsDonator, user.Locale)

	// ── Step 4: DB update ───────────────────────────────────────
	log.Printf("💽 [bot/payment] Attempting to update user %d to is_donator = true and create donation", user.ID)

	// month → expires in 1 month; lifetime → NULL (never expires).
	var expiresAt *time.Time
	if plan == "month" {
		t := time.Now().UTC().AddDate(0, 1, 0)
		expiresAt = &t
	}
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).
			Where("id = ?", user.ID).
			Updates(map[string]interface{}{
				"is_donator":         true,
				"premium_expires_at": expiresAt,
			})
		if result.Error != nil {
			return fmt.Errorf("set premium: %w", result.Error)
		}
		log.Printf("💽 [bot/payment] UPDATE users SET is_donator=true, premium_expires_at=%v WHERE id=%d — rows_affected=%d",
			expiresAt, user.ID, result.RowsAffected)

		donation := model.Donation{
			UserID:                  user.ID,
			TelegramID:              user.TelegramID,
			TelegramPaymentChargeID: chargeID,
			Amount:                  payment.TotalAmount,
		}
		if err := tx.Create(&donation).Error; err != nil {
			return fmt.Errorf("create donation: %w", err)
		}
		log.Printf("💽 [bot/payment] INSERT donation: id=%d, charge=%s", donation.ID, chargeID)
		return nil
	})
	if txErr != nil {
		log.Printf("❌ [bot/payment] Transaction FAILED for user=%d charge=%s: %v", user.ID, chargeID, txErr)
		return
	}
	log.Printf("🟢 [bot/payment] DB update successful. Premium activated for user=%d, charge=%s, amount=%d",
		user.ID, chargeID, payment.TotalAmount)

	// ── Step 5: Localized congratulation ────────────────────────
	locale := user.Locale
	if locale == "" && msg.From != nil {
		locale = msg.From.LanguageCode
	}

	var text string
	if strings.HasPrefix(locale, "ru") {
		text = "🎉 <b>Спасибо за покупку!</b>\n\nPremium успешно активирован. Вернитесь в приложение, чтобы пользоваться всеми функциями!"
	} else {
		text = "🎉 <b>Thank you for your purchase!</b>\n\nPremium is activated. Return to the app to enjoy all features!"
	}

	log.Printf("✉️ [bot/payment] Sending success message to chat=%d (locale=%q)", msg.Chat.ID, locale)
	if _, err := b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    msg.Chat.ID,
		Text:      text,
		ParseMode: "HTML",
	}); err != nil {
		log.Printf("❌ [bot/payment] Congratulation send error for user=%d: %v", user.ID, err)
	} else {
		log.Printf("✅ [bot/payment] Congratulation sent successfully to user=%d", user.ID)
	}
}
