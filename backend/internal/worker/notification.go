package worker

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// notificationTickInterval is the worker's check cadence. Used as the
// half-window when deciding whether the user's preferred notification time
// has just passed.
const notificationTickInterval = 30 * time.Minute

// Start launches the notification check loop every 30 minutes.
func (w *NotificationWorker) Start(ctx context.Context) {
	log.Println("[notification-worker] starting")

	ticker := time.NewTicker(notificationTickInterval)
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
	// Widen the candidate window to ~±2h around 24h-out so that the user's
	// preferred notification_time, evaluated in their own timezone, has a
	// chance to land inside it regardless of where they live.
	windowStart := now.Add(22 * time.Hour)
	windowEnd := now.Add(26 * time.Hour)

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
		if !sub.User.NotificationsEnabled {
			continue
		}
		if !shouldSendNow(&sub.User, now) {
			continue
		}

		text := buildReminderText(&sub, sub.User.Locale)

		if err := w.notifier.SendMessage(ctx, sub.User.TelegramID, text); err != nil {
			log.Printf("[notification-worker] send to %d error: %v", sub.User.TelegramID, err)
			continue
		}

		w.db.WithContext(ctx).Model(&sub).Update("notified_at", now)
	}
}

// shouldSendNow returns true when `now`, projected into the user's timezone,
// has just passed their preferred notification time within the worker's
// tick interval. We send on the FIRST tick that's at-or-after the preferred
// moment; the existing notified_at dedupe filter prevents double-sends on
// subsequent ticks the same day.
func shouldSendNow(u *model.User, now time.Time) bool {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	hh, mm := parseHHMM(u.NotificationTime)
	nowLocal := now.In(loc)
	preferred := time.Date(
		nowLocal.Year(), nowLocal.Month(), nowLocal.Day(),
		hh, mm, 0, 0, loc,
	)

	delta := nowLocal.Sub(preferred)
	// `delta` in [0, tickInterval] means the preferred moment is in the
	// just-elapsed tick window. Negative = too early; >tick = too late
	// (will catch on next day or already sent and deduped via notified_at).
	return delta >= 0 && delta < notificationTickInterval
}

// buildReminderText assembles the renewal-reminder Telegram message,
// localized to the user's locale and folding the user's optional Note into
// the line when present.
//
//	No note (ru): "💳 Напоминание: завтра спишется оплата за подписку *Netflix* — 15.49 USD."
//	With note   : "💳 Напоминание: завтра спишется оплата за подписку *Netflix* (Для Кристины) — 15.49 USD."
//
// The note is escaped against the four characters legacy Markdown
// (ParseMode: "Markdown") interprets, so a user typing "for *mom*" can't
// break the bold formatting of the subscription name.
func buildReminderText(sub *model.Subscription, locale string) string {
	notePart := ""
	if n := strings.TrimSpace(sub.Note); n != "" {
		notePart = " (" + escapeTelegramMarkdown(n) + ")"
	}

	switch locale {
	case "ru":
		return fmt.Sprintf(
			"💳 Напоминание: завтра спишется оплата за подписку *%s*%s — %.2f %s.",
			sub.Name, notePart, sub.Amount, sub.Currency,
		)
	default:
		return fmt.Sprintf(
			"💳 Reminder: your *%s*%s subscription renews tomorrow — %.2f %s.",
			sub.Name, notePart, sub.Amount, sub.Currency,
		)
	}
}

// escapeTelegramMarkdown escapes the characters that Telegram's legacy
// Markdown parse mode interprets: * _ ` [ . Strict enough to keep the
// subscription's bold name intact even if the user's note contains those.
var telegramMarkdownReplacer = strings.NewReplacer(
	"*", `\*`,
	"_", `\_`,
	"`", "\\`",
	"[", `\[`,
)

func escapeTelegramMarkdown(s string) string {
	return telegramMarkdownReplacer.Replace(s)
}

// parseHHMM parses an "HH:MM" string. Falls back to 10:00 on malformed input
// — the handler validates input on write, so this is a defence in depth for
// rows predating the new field.
func parseHHMM(s string) (int, int) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 10, 0
	}
	hh, err1 := strconv.Atoi(parts[0])
	mm, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 10, 0
	}
	return hh, mm
}
