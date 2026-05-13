package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
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

// notificationTickInterval is the worker's check cadence.
const notificationTickInterval = 30 * time.Minute

// notificationDedupWindow stops us re-notifying the same subscription
// over and over: once a reminder went out we won't fire again until this
// window has elapsed. 20h chosen so a daily user gets at most one ping
// per renewal date even if the worker scans every 30 minutes.
const notificationDedupWindow = 20 * time.Hour

// renewCallbackPrefix is the CallbackData prefix written into the inline
// "Renew" button. The bot dispatches on this prefix. Format is
// "renew_sub_<uuid>", 46 bytes — well under Telegram's 64-byte limit.
const renewCallbackPrefix = "renew_sub_"

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
	// Pre-filter at the DB level: anything that could possibly land on
	// "tomorrow" in some user's timezone is at most ~26h out from now in
	// UTC. Widen to 20-30h to keep the query small while still admitting
	// every plausible candidate. The precise per-user logic lives in
	// shouldSendNow below.
	windowStart := now.Add(20 * time.Hour)
	windowEnd := now.Add(30 * time.Hour)

	var subs []model.Subscription
	err := w.db.WithContext(ctx).
		Preload("User").
		Where("next_payment_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("notified_at IS NULL OR notified_at < ?", now.Add(-notificationDedupWindow)).
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
		if !shouldSendNow(&sub, &sub.User, now) {
			continue
		}

		text := buildReminderText(&sub, sub.User.Locale)
		markup := renewKeyboard(&sub, sub.User.Locale)

		if err := w.notifier.SendMessageWithMarkup(ctx, sub.User.TelegramID, text, markup); err != nil {
			log.Printf("[notification-worker] send to %d error: %v", sub.User.TelegramID, err)
			continue
		}

		w.db.WithContext(ctx).Model(&sub).Update("notified_at", now)
	}
}

// shouldSendNow decides whether a candidate subscription deserves a
// reminder right now. Three gates, evaluated in the user's own timezone:
//
//  1. The user's preferred HH:MM has already passed today (string compare
//     on zero-padded "15:04" — works because both values are HH:MM 24h).
//  2. The next payment falls on "tomorrow" relative to the user's
//     localNow.AddDate(0, 0, 1) — covers users whose UTC day boundary
//     doesn't match the server's.
//  3. We haven't fired a notification for this row inside the dedup
//     window. The DB query already filters on this, but checking again
//     here keeps the in-memory loop honest if the data races with another
//     writer (e.g. callback handler).
//
// All three must hold. Time-of-day failure means "too early today" —
// we'll catch it on a later tick. Tomorrow-mismatch means the row is in
// the candidate window but its local date doesn't actually land tomorrow
// for this particular user.
func shouldSendNow(sub *model.Subscription, u *model.User, now time.Time) bool {
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)

	preferredHHMM := u.NotificationTime
	if !isValidHHMM(preferredHHMM) {
		preferredHHMM = "10:00"
	}
	if localNow.Format("15:04") < preferredHHMM {
		return false
	}

	next := sub.NextPaymentAt.In(loc)
	tomorrow := localNow.AddDate(0, 0, 1)
	if next.Year() != tomorrow.Year() ||
		next.Month() != tomorrow.Month() ||
		next.Day() != tomorrow.Day() {
		return false
	}

	if sub.NotifiedAt != nil && now.Sub(*sub.NotifiedAt) < notificationDedupWindow {
		return false
	}

	return true
}

// renewKeyboard builds the single-button inline keyboard that ships with
// every renewal reminder. Clicking it fires a CallbackQuery handled by
// internal/bot/bot.go, which advances NextPaymentAt by the subscription's
// period and clears notified_at.
func renewKeyboard(sub *model.Subscription, locale string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{
				Text:         renewButtonLabel(locale),
				CallbackData: renewCallbackPrefix + sub.ID.String(),
			},
		}},
	}
}

func renewButtonLabel(locale string) string {
	if locale == "ru" {
		return "✅ Оплачено (Продлить)"
	}
	return "✅ Paid (Renew)"
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

// isValidHHMM verifies that a stored notification_time is in the strict
// "HH:MM" 24-hour format we depend on for lexical comparison. The PATCH /me
// handler enforces this on write via regexp, so this guard is defence in
// depth — protects against rows predating the field or any direct DB edits.
func isValidHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h1, h2, m1, m2 := s[0], s[1], s[3], s[4]
	if h1 < '0' || h1 > '2' || h2 < '0' || h2 > '9' {
		return false
	}
	if h1 == '2' && h2 > '3' {
		return false
	}
	if m1 < '0' || m1 > '5' || m2 < '0' || m2 > '9' {
		return false
	}
	return true
}
