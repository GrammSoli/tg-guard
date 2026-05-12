package notifier

import (
	"context"
	"fmt"
	"log"

	"github.com/go-telegram/bot"
)

// Notifier abstracts Telegram message sending so we can swap real and mock
// implementations (e.g. for E2E tests).
type Notifier interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
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
