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
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/workerutil"
)

// ── FSM state for broadcast ───────────────────────────
const stateAwaitBroadcastMsg = "await_broadcast_msg"

// broadcastHandler holds all broadcast-related logic. It reuses the
// adminPanel's FSM helpers (setState, getData, etc.) and repository.
type broadcastHandler struct {
	panel *adminPanel
}

func newBroadcastHandler(panel *adminPanel) *broadcastHandler {
	return &broadcastHandler{panel: panel}
}

// handleBroadcastStart shows the audience selection keyboard (FSM Step 1).
func (bh *broadcastHandler) handleBroadcastStart(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	text := "📢 *Настройка рассылки*\nВыберите аудиторию:"
	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "🇷🇺 Только RU", CallbackData: "broadcast_lang_ru"},
				{Text: "🇬🇧 Только EN", CallbackData: "broadcast_lang_en"},
			},
			{
				{Text: "🌍 Все", CallbackData: "broadcast_lang_all"},
				{Text: "❌ Отмена", CallbackData: "admin_back"},
			},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

// handleBroadcastLang processes the language choice callback (FSM Step 1→2).
// Saves the chosen language in FSM data and moves to stateAwaitBroadcastMsg.
func (bh *broadcastHandler) handleBroadcastLang(ctx context.Context, b *tgbot.Bot, tgID int64, data string, chatID int64, msgID int) {
	// data = "broadcast_lang_ru" / "broadcast_lang_en" / "broadcast_lang_all"
	lang := strings.TrimPrefix(data, "broadcast_lang_")
	if lang != "ru" && lang != "en" && lang != "all" {
		return
	}

	bh.panel.setData(ctx, tgID, "broadcast:"+lang)
	bh.panel.setState(ctx, tgID, stateAwaitBroadcastMsg)

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "📨 Отправьте мне сообщение для рассылки.\n\nВы можете использовать *текст*, *фото* или *видео*.\nБот скопирует его один-в-один.\n\n_Для отмены введите /cancel_",
		ParseMode: "Markdown",
	})
}

// handleBroadcastContent processes the content message from admin (FSM Step 2→confirmation).
// Works for ANY message type: text, photo, video, document, audio, sticker, etc.
// Saves the message coordinates (chat_id:message_id) and shows the confirmation prompt.
func (bh *broadcastHandler) handleBroadcastContent(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}

	tgID := msg.From.ID
	chatID := msg.Chat.ID

	// Extract the saved language from FSM data.
	raw := bh.panel.getData(ctx, tgID)
	if !strings.HasPrefix(raw, "broadcast:") {
		bh.panel.clearState(ctx, tgID)
		return
	}
	lang := strings.TrimPrefix(raw, "broadcast:")

	// Save message coordinates: "broadcast:lang:chatID:messageID"
	bh.panel.setData(ctx, tgID, fmt.Sprintf("broadcast:%s:%d:%d", lang, chatID, msg.ID))
	bh.panel.setState(ctx, tgID, stateNone) // done with text FSM; confirm via inline button

	// Count eligible recipients.
	count, err := bh.panel.repo.CountBroadcastRecipients(lang)
	if err != nil {
		log.Printf("[broadcast] count error: %v", err)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Ошибка при подсчёте получателей.",
		})
		bh.panel.clearState(ctx, tgID)
		return
	}

	langLabel := map[string]string{"ru": "🇷🇺 RU", "en": "🇬🇧 EN", "all": "🌍 Все"}[lang]

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text: fmt.Sprintf("📊 *Предпросмотр рассылки*\n\n🎯 Аудитория: %s\n👥 Получателей: *%d*\n\nНачинаем?",
			langLabel, count),
		ParseMode: "Markdown",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: "✅ Начать", CallbackData: "broadcast_confirm"},
					{Text: "❌ Отмена", CallbackData: "admin_back"},
				},
			},
		},
	})
}

// handleBroadcastConfirm processes the "Start" button (FSM Step 3).
// Immediately responds to admin, clears FSM, and launches the async worker.
func (bh *broadcastHandler) handleBroadcastConfirm(ctx context.Context, b *tgbot.Bot, tgID int64, chatID int64, msgID int) {
	raw := bh.panel.getData(ctx, tgID)
	bh.panel.clearState(ctx, tgID)

	// Parse "broadcast:lang:fromChatID:messageID"
	parts := strings.SplitN(raw, ":", 4)
	if len(parts) < 4 || parts[0] != "broadcast" {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      "❌ Данные рассылки потеряны. Начните заново через /admin.",
		})
		return
	}

	lang := parts[1]
	var fromChatID int64
	var messageID int
	fmt.Sscanf(parts[2], "%d", &fromChatID)
	fmt.Sscanf(parts[3], "%d", &messageID)

	if fromChatID == 0 || messageID == 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      "❌ Некорректные данные сообщения. Начните заново через /admin.",
		})
		return
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "🚀 Рассылка запущена в фоне.\nОтчёт будет отправлен по завершении.",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "🔙 Главное меню", CallbackData: "admin_back"}},
			},
		},
	})

	// Launch the background worker. Wrapped in Supervise for panic safety.
	go workerutil.Supervise("broadcast-copymsg", func() {
		bh.runBroadcast(b, lang, fromChatID, messageID, tgID)
	})
}

// runBroadcast iterates users in batches and copies the admin's message via
// CopyMessage API. Respects the app lifecycle context, uses FindInBatches
// for memory efficiency, and handles Telegram 429 rate limits gracefully.
func (bh *broadcastHandler) runBroadcast(b *tgbot.Bot, lang string, fromChatID int64, messageID int, adminTgID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	var sent, failed int

	// Build the base query with language + active-user filters.
	q := bh.panel.db.WithContext(ctx).
		Model(&model.User{}).
		Where("is_banned = false AND deleted_at IS NULL")

	switch lang {
	case "ru":
		q = q.Where("LOWER(locale) = 'ru'")
	case "en":
		q = q.Where("LOWER(locale) = 'en' OR locale IS NULL OR locale = ''")
	}

	// Stream users in chunks of 500 — never load the entire table into RAM.
	err := q.FindInBatches(&[]model.User{}, 500, func(tx *gorm.DB, _ int) error {
		users, ok := tx.Statement.Dest.(*[]model.User)
		if !ok {
			return errors.New("broadcast: unexpected batch dest type")
		}
		for _, u := range *users {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if bh.copyOne(ctx, b, fromChatID, messageID, u.TelegramID) {
				sent++
			} else {
				failed++
			}

			// Throttle: ~20 msg/s to stay under Telegram's rate limits.
			time.Sleep(50 * time.Millisecond)
		}
		return nil
	}).Error

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("[broadcast] batch iteration error: %v", err)
	}

	log.Printf("[broadcast] finished: %d sent, %d failed", sent, failed)

	// Send completion report to admin.
	report := fmt.Sprintf("✅ *Рассылка завершена!*\n\n📤 Успешно: *%d*\n❌ Ошибок: *%d*", sent, failed)
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    adminTgID,
		Text:      report,
		ParseMode: "Markdown",
	})
}

// copyOne performs a single CopyMessage call with retry-after-aware back-off.
// Returns true on success, false on permanent failure. Does not break the
// broadcast loop on "bot was blocked by the user" errors.
func (bh *broadcastHandler) copyOne(parent context.Context, b *tgbot.Bot, fromChatID int64, messageID int, toChatID int64) bool {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		sendCtx, cancel := context.WithTimeout(parent, 10*time.Second)
		_, err := b.CopyMessage(sendCtx, &tgbot.CopyMessageParams{
			ChatID:     toChatID,
			FromChatID: fromChatID,
			MessageID:  messageID,
		})
		cancel()

		if err == nil {
			return true
		}

		errLower := strings.ToLower(err.Error())

		// Permanent failures — skip this user, don't retry.
		if strings.Contains(errLower, "blocked") ||
			strings.Contains(errLower, "forbidden") ||
			strings.Contains(errLower, "deactivated") ||
			strings.Contains(errLower, "not found") {
			log.Printf("[broadcast] skip chat %d: %v", toChatID, err)
			return false
		}

		// 429 rate limit — respect Retry-After header, then retry.
		if workerutil.IsRateLimit(err) && attempt < maxAttempts-1 {
			delay, ok := workerutil.ParseRetryAfter(err)
			if !ok {
				delay = time.Second
			}
			log.Printf("[broadcast] 429 for chat %d, sleeping %s (attempt %d/%d)",
				toChatID, delay, attempt+1, maxAttempts)
			select {
			case <-parent.Done():
				return false
			case <-time.After(delay):
			}
			continue
		}

		log.Printf("[broadcast] failed to copy to %d (attempt %d): %v",
			toChatID, attempt+1, err)
		return false
	}
	return false
}
