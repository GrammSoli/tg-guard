package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
)

// NotificationWorker checks upcoming payments and sends Telegram reminders.
type NotificationWorker struct {
	db       *gorm.DB
	notifier notifier.Notifier
}

func NewNotificationWorker(db *gorm.DB, n notifier.Notifier) *NotificationWorker {
	return &NotificationWorker{db: db, notifier: n}
}

// Start launches the notification check loop every 30 minutes.
func (w *NotificationWorker) Start(ctx context.Context) {
	log.Println("[notification-worker] starting")

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[notification-worker] stopped")
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

func (w *NotificationWorker) check(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	now := time.Now().UTC()
	windowStart := now.Add(23 * time.Hour)
	windowEnd := now.Add(25 * time.Hour)

	var subs []model.Subscription
	err := w.db.WithContext(ctx).
		Preload("User").
		Where("next_payment_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("notified_at IS NULL OR notified_at < ?", now.Add(-23*time.Hour)).
		Find(&subs).Error
	if err != nil {
		log.Printf("[notification-worker] query error: %v", err)
		return
	}

	if len(subs) == 0 {
		return
	}

	log.Printf("[notification-worker] found %d subscriptions due soon", len(subs))

	for _, sub := range subs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if sub.User.TelegramID == 0 {
			continue
		}

		text := fmt.Sprintf(
			"💳 Reminder: your *%s* subscription (%.2f %s) renews tomorrow.",
			sub.Name, sub.Amount, sub.Currency,
		)

		if err := w.notifier.SendMessage(ctx, sub.User.TelegramID, text); err != nil {
			log.Printf("[notification-worker] send to %d error: %v", sub.User.TelegramID, err)
			continue
		}

		w.db.WithContext(ctx).Model(&sub).Update("notified_at", now)
	}
}
