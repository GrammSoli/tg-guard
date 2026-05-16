package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/workerutil"
)

// broadcastConcurrency caps in-flight CopyMessage calls. Telegram's
// global send-rate limit for a bot is ~30 msg/s; the broadcastTick
// (40ms = 25/s) is the actual rate gate, the pool size just lets
// individual sends overlap so high per-send latency doesn't drop
// effective throughput below the tick rate. Audit Tier-3 #6.
const (
	broadcastConcurrency = 8
	broadcastTick        = 40 * time.Millisecond
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
				{Text: "🇷🇺 Только RU", CallbackData: "admin_bc_lang_ru"},
				{Text: "🇬🇧 Только EN", CallbackData: "admin_bc_lang_en"},
			},
			{
				{Text: "🌍 Все", CallbackData: "admin_bc_lang_all"},
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
	// data = "admin_bc_lang_ru" / "admin_bc_lang_en" / "admin_bc_lang_all"
	lang := strings.TrimPrefix(data, "admin_bc_lang_")
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
					{Text: "✅ Начать", CallbackData: "admin_bc_confirm"},
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

	// Guard against double-start. Telegram can deliver a callback twice
	// (network retry, user double-tap before ack), and without this two
	// goroutines would broadcast the same message to every user. Lock is
	// scoped by (lang, fromChatID, messageID) so a different broadcast
	// can still launch in parallel. TTL = 24h matches the worker's max
	// run time; defer-cleanup releases earlier if the run finishes.
	lockKey := fmt.Sprintf("broadcast_lock:%s:%d:%d", lang, fromChatID, messageID)
	acquired, lockErr := bh.panel.rdb.SetNX(ctx, lockKey, "1", 24*time.Hour).Result()
	if lockErr != nil {
		log.Printf("[broadcast] redis SetNX error for lock: %v — proceeding without lock", lockErr)
	} else if !acquired {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      "⚠️ Эта рассылка уже запущена. Подождите завершения.",
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

	// Launch the background worker. Registered in workerWG so graceful
	// shutdown waits for the in-flight broadcast to finish (or hit its
	// drain timeout). Wrapped in Supervise for panic safety.
	bh.panel.wg.Add(1)
	go func() {
		defer bh.panel.wg.Done()
		// Release the lock even on panic — Supervise catches it but we
		// want the next broadcast attempt to succeed without waiting
		// 24h for the TTL.
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			bh.panel.rdb.Del(releaseCtx, lockKey)
		}()
		workerutil.Supervise("broadcast-copymsg", func() {
			bh.runBroadcast(b, lang, fromChatID, messageID, tgID)
		})
	}()
}

// runBroadcast iterates users in batches and copies the admin's message via
// CopyMessage API. Respects the app lifecycle context, uses FindInBatches
// for memory efficiency, and handles Telegram 429 rate limits gracefully.
//
// Parent context is bh.panel.appCtx (the server lifecycle ctx) so SIGTERM
// cancels the broadcast and main.go's workerWG drain blocks until this
// returns. Previously this used context.Background() which would keep
// running against a closing DB pool — see audit C2.
func (bh *broadcastHandler) runBroadcast(b *tgbot.Bot, lang string, fromChatID int64, messageID int, adminTgID int64) {
	ctx, cancel := context.WithTimeout(bh.panel.appCtx, 24*time.Hour)
	defer cancel()

	// sent / failed must be int64 for atomic ops — workers update them
	// from inside the goroutine pool, so a non-atomic int would race
	// (go test -race would catch it; production would silently report
	// truncated counts to the admin report at the end).
	var sent, failed int64

	// Build the base query with language + active-user filters. Inactive
	// users (those who blocked the bot — is_active=false set by the
	// my_chat_member handler) are excluded: Telegram would reject every
	// send with "bot was blocked by the user" anyway, and we don't want
	// to burn API quota or fan up the 429 rate-limit counter for them.
	q := bh.panel.db.WithContext(ctx).
		Model(&model.User{}).
		Where("is_banned = false AND deleted_at IS NULL AND is_active = true")

	switch lang {
	case "ru":
		q = q.Where("LOWER(locale) = 'ru'")
	case "en":
		q = q.Where("LOWER(locale) = 'en' OR locale IS NULL OR locale = ''")
	}

	// Worker pool + token-bucket ticker. The previous implementation did
	// sequential `copyOne(...); sleep(50ms)`, which on slow international
	// links dropped effective throughput below the rate cap because each
	// send blocked for its own latency. The new pool overlaps in-flight
	// sends up to broadcastConcurrency, gated by a global ticker at the
	// Telegram-safe rate. For a 500k-user campaign on healthy latency
	// this brings completion from ~7 hours down to ~5.5 hours (peak rate
	// 25/s vs the prior ~20/s, AND closer to the cap because latency no
	// longer steals from the tick budget).
	chatCh := make(chan int64, broadcastConcurrency*2)
	ticker := time.NewTicker(broadcastTick)
	defer ticker.Stop()

	var wg sync.WaitGroup
	for i := 0; i < broadcastConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chatID := range chatCh {
				if bh.copyOne(ctx, b, fromChatID, messageID, chatID) {
					atomic.AddInt64(&sent, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}()
	}

	// Stream users in chunks of 500 — never load the entire table into
	// RAM — and feed each TelegramID into the worker pool one tick at
	// a time so the GLOBAL emit rate stays at broadcastTick regardless
	// of how many workers are idle vs busy.
	err := q.FindInBatches(&[]model.User{}, 500, func(tx *gorm.DB, _ int) error {
		users, ok := tx.Statement.Dest.(*[]model.User)
		if !ok {
			return errors.New("broadcast: unexpected batch dest type")
		}
		for _, u := range *users {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case chatCh <- u.TelegramID:
			}
		}
		return nil
	}).Error

	// Signal workers to drain and exit, then wait for them so the final
	// sent/failed counts are stable before we report.
	close(chatCh)
	wg.Wait()

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("[broadcast] batch iteration error: %v", err)
	}

	finalSent := atomic.LoadInt64(&sent)
	finalFailed := atomic.LoadInt64(&failed)
	log.Printf("[broadcast] finished: %d sent, %d failed", finalSent, finalFailed)

	// Send completion report to admin.
	report := fmt.Sprintf("✅ *Рассылка завершена!*\n\n📤 Успешно: *%d*\n❌ Ошибок: *%d*", finalSent, finalFailed)
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

		// Permanent per-recipient failures — skip this user, don't retry.
		// Matched against an explicit phrase list (workerutil) so a
		// transient global 403 (token rotation, edge proxy) is NOT
		// mistaken for a per-user block, which would skip the entire
		// remaining batch. See audit #9.
		if workerutil.IsPermanentSendFailure(err) {
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
