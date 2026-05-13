package notifier

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Notifier abstracts Telegram message sending so we can swap real and mock
// implementations (e.g. for E2E tests).
type Notifier interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
	// SendMessageWithMarkup sends a Markdown message with an attached reply
	// markup (typically an inline keyboard). Used by the renewal-reminder
	// flow so the user can extend a subscription right from the chat.
	SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) error
	// SendDocument uploads an in-memory file as a Telegram document with a
	// caption. Used for one-shot deliveries (data exports etc.) where the
	// receiving client is a mini-app that can't reliably download blobs.
	SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string) error
}

// ─── Telegram (production) ─────────────────────────────────────

// TelegramNotifier sends real messages via the Telegram Bot API.
type TelegramNotifier struct {
	bot *bot.Bot
}

func NewTelegramNotifier(b *bot.Bot) *TelegramNotifier {
	return &TelegramNotifier{bot: b}
}

func (n *TelegramNotifier) SendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := n.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "Markdown",
	})
	return err
}

func (n *TelegramNotifier) SendMessageWithMarkup(ctx context.Context, chatID int64, text string, markup models.ReplyMarkup) error {
	_, err := n.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: markup,
	})
	return err
}

func (n *TelegramNotifier) SendDocument(ctx context.Context, chatID int64, filename string, content []byte, caption string) error {
	_, err := n.bot.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(content),
		},
		Caption: caption,
	})
	return err
}

// ─── Mock (test mode) ──────────────────────────────────────────

// MockNotifier logs messages to stdout instead of sending them.
type MockNotifier struct{}

func NewMockNotifier() *MockNotifier {
	return &MockNotifier{}
}

func (n *MockNotifier) SendMessage(_ context.Context, chatID int64, text string) error {
	log.Printf("[mock-notifier] would send to %d: %s", chatID, text)
	fmt.Printf("[mock-notifier] chatID=%d text=%q\n", chatID, text)
	return nil
}

func (n *MockNotifier) SendMessageWithMarkup(_ context.Context, chatID int64, text string, _ models.ReplyMarkup) error {
	log.Printf("[mock-notifier] would send to %d (with markup): %s", chatID, text)
	return nil
}

func (n *MockNotifier) SendDocument(_ context.Context, chatID int64, filename string, content []byte, caption string) error {
	log.Printf("[mock-notifier] would send document to %d: %s (%d bytes) caption=%q", chatID, filename, len(content), caption)
	return nil
}
