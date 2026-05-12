package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
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
