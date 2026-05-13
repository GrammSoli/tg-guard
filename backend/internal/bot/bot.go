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
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
)

// renewCallbackPrefix / cancelCallbackPrefix are the CallbackData prefixes
// written by the notification worker into the two-button inline keyboard
// attached to every reminder. Mirrored from internal/worker/notification.go
// — kept in sync by code review (small enough that introducing a shared
// constants package would be more boilerplate than benefit).
const (
	renewCallbackPrefix  = "renew_sub_"
	cancelCallbackPrefix = "cancel_sub_"
)

// Setup initializes the Telegram bot with handlers and returns the bot instance.
func Setup(cfg *config.Config, db *gorm.DB) (*tgbot.Bot, error) {
	adminRepo := repository.NewAdminRepo(db)

	opts := []tgbot.Option{
		tgbot.WithDefaultHandler(func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			// Fallback handler — ignore unknown updates
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
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, renewCallbackPrefix, tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleRenewCallback(ctx, b, update, db)
		},
	)
	b.RegisterHandler(tgbot.HandlerTypeCallbackQueryData, cancelCallbackPrefix, tgbot.MatchTypePrefix,
		func(ctx context.Context, b *tgbot.Bot, update *models.Update) {
			handleCancelCallback(ctx, b, update, db)
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

	// Upsert user
	var user model.User
	result := db.Where("telegram_id = ?", tgUser.ID).First(&user)
	if result.Error == gorm.ErrRecordNotFound {
		user = model.User{
			TelegramID: tgUser.ID,
			FirstName:  tgUser.FirstName,
			LastName:   tgUser.LastName,
			Username:   tgUser.Username,
		}
		db.Create(&user)
	}

	// Parse deep link parameters: /start room_ABC123 or /start ad_campaign_tag
	parts := strings.SplitN(update.Message.Text, " ", 2)
	if len(parts) == 2 {
		param := parts[1]

		// Track traffic campaign
		if strings.HasPrefix(param, "ad_") {
			tag := param
			adminRepo.IncrementCampaign(tag, "bot_starts")
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

	idStr := strings.TrimPrefix(cb.Data, renewCallbackPrefix)
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

	// Persist: new payment date + clear notified_at so the worker can
	// fire again before the *next* renewal. Both in one Updates() so we
	// don't half-update on failure.
	if err := db.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ?", sub.ID).
		Updates(map[string]interface{}{
			"next_payment_at": next,
			"notified_at":     nil,
		}).Error; err != nil {
		log.Printf("[bot.renew] update error: %v", err)
		answerAndLog("save failed")
		return
	}
	sub.NextPaymentAt = next

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

	idStr := strings.TrimPrefix(cb.Data, cancelCallbackPrefix)
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
		newText := cancelConfirmationText(&sub, owner.Locale)
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

// escapeMarkdownLite escapes the four characters legacy Markdown parse mode
// interprets. Kept local to bot.go so worker and bot don't reach into each
// other's internals.
var markdownLiteReplacer = strings.NewReplacer(
	"*", `\*`,
	"_", `\_`,
	"`", "\\`",
	"[", `\[`,
)

func escapeMarkdownLite(s string) string {
	return markdownLiteReplacer.Replace(s)
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
