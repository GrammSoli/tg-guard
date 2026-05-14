package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
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
func Setup(cfg *config.Config, db *gorm.DB, notifWorker *worker.NotificationWorker, rdb *redis.Client) (*tgbot.Bot, error) {
	adminRepo := repository.NewAdminRepo(db)
	panel := newAdminPanel(cfg, db, rdb)

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			// Route admin FSM text input before dropping unknown updates.
			if update.Message != nil && update.Message.Text != "" && cfg.IsAdmin(update.Message.From.ID) {
				state := panel.getState(ctx, update.Message.From.ID)
				if state != stateNone {
					panel.handleText(ctx, b, update)
					return
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

	return b, nil
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
	result := db.Unscoped().Where("telegram_id = ?", tgUser.ID).First(&user)
	if result.Error == gorm.ErrRecordNotFound {
		isNewUser = true
		user = model.User{
			TelegramID: tgUser.ID,
			FirstName:  tgUser.FirstName,
			LastName:   tgUser.LastName,
			Username:   tgUser.Username,
		}
		db.Create(&user)
	} else if user.DeletedAt.Valid {
		// Restore soft-deleted user on /start
		db.Unscoped().Model(&user).Update("deleted_at", nil)
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
				adminRepo.IncrementCampaign(tag, "auths")
			}
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

	var sub model.Subscription
	if err := db.WithContext(ctx).First(&sub, "id = ?", subID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			answerAndLog("subscription not found")
		} else {
			log.Printf("[bot.renew] lookup error: %v", err)
			answerAndLog("lookup failed")
		}
		return
	}

	// Authorize the click — only the owner can advance the date.
	var owner model.User
	if err := db.WithContext(ctx).First(&owner, sub.UserID).Error; err != nil {
		log.Printf("[bot.renew] owner lookup error: %v", err)
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

	var sub model.Subscription
	if err := db.WithContext(ctx).First(&sub, "id = ?", subID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			answerAndLog("already deleted")
		} else {
			log.Printf("[bot.cancel] lookup error: %v", err)
			answerAndLog("lookup failed")
		}
		return
	}

	var owner model.User
	if err := db.WithContext(ctx).First(&owner, sub.UserID).Error; err != nil {
		log.Printf("[bot.cancel] owner lookup error: %v", err)
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
func SetWebhook(b *tgbot.Bot, cfg *config.Config) error {
	webhookURL := cfg.BaseURL + "/webhook"
	log.Printf("[bot] setting webhook: %s", webhookURL)

	_, err := b.SetWebhook(context.Background(), &tgbot.SetWebhookParams{
		URL:         webhookURL,
		SecretToken: cfg.WebhookSecret,
	})
	return err
}
