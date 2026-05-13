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
	"github.com/subguard/backend/internal/tgutil"
)

// NotificationWorker checks upcoming payments and sends Telegram reminders.
type NotificationWorker struct {
	db       *gorm.DB
	notifier notifier.Notifier
}

func NewNotificationWorker(db *gorm.DB, n notifier.Notifier) *NotificationWorker {
	return &NotificationWorker{db: db, notifier: n}
}

// SetNotifier replaces the notifier implementation. Used during startup to
// break the circular dependency: bot.Setup needs the worker, but the real
// TelegramNotifier needs the bot instance that Setup returns.
func (w *NotificationWorker) SetNotifier(n notifier.Notifier) {
	w.notifier = n
}

// notificationTickInterval is the worker's check cadence.
const notificationTickInterval = 30 * time.Minute

// notificationDedupWindow stops us re-notifying the same subscription
// over and over: once a reminder went out we won't fire again until this
// window has elapsed. 20h chosen so a daily user gets at most one ping
// per renewal date even if the worker scans every 30 minutes.
const notificationDedupWindow = 20 * time.Hour

// Callback prefixes imported from tgutil — shared with bot package.

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
	log.Printf("[notification-worker] ── tick ── UTC now: %s", now.Format(time.RFC3339))

	// Pre-filter at the DB level: anything that could possibly land on
	// "tomorrow" in some user's timezone is at most ~26h out from now in
	// UTC. Widen to 20-30h to keep the query small while still admitting
	// every plausible candidate. The precise per-user logic lives in
	// shouldSendNow below.
	windowStart := now.Add(20 * time.Hour)
	windowEnd := now.Add(30 * time.Hour)

	var subs []model.Subscription
	err := w.db.WithContext(ctx).
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, telegram_id, username, notifications_enabled, timezone, notification_time, locale")
		}).
		Where("next_payment_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("notified_at IS NULL OR notified_at < ?", now.Add(-notificationDedupWindow)).
		Find(&subs).Error
	if err != nil {
		log.Printf("[notification-worker] query error: %v", err)
		return
	}

	if len(subs) == 0 {
		log.Println("[notification-worker] no candidate subscriptions found")
		return
	}

	log.Printf("[notification-worker] found %d candidate subscriptions", len(subs))

	for _, sub := range subs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		userLabel := fmt.Sprintf("user %d (@%s)", sub.User.TelegramID, sub.User.Username)

		if sub.User.TelegramID == 0 {
			log.Printf("[notification-worker] skip %s sub %q: telegram_id=0", userLabel, sub.Name)
			continue
		}
		if !sub.User.NotificationsEnabled {
			log.Printf("[notification-worker] skip %s sub %q: notifications disabled", userLabel, sub.Name)
			continue
		}
		if !shouldSendNow(&sub, &sub.User, now) {
			continue // reason already logged inside shouldSendNow
		}

		text := buildReminderText(&sub, sub.User.Locale)
		markup := renewKeyboard(&sub, sub.User.Locale)

		if err := w.notifier.SendMessageWithMarkup(ctx, sub.User.TelegramID, text, markup); err != nil {
			log.Printf("[notification-worker] SEND FAILED %s sub %q: %v", userLabel, sub.Name, err)
			continue
		}

		log.Printf("[notification-worker] ✓ notification sent to %s for sub %q (%.2f %s)",
			userLabel, sub.Name, sub.Amount, sub.Currency)
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
	userLabel := fmt.Sprintf("user %d (@%s)", u.TelegramID, u.Username)

	loc, err := time.LoadLocation(u.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)

	preferredHHMM := u.NotificationTime
	if !isValidHHMM(preferredHHMM) {
		preferredHHMM = "10:00"
	}

	log.Printf("[notification-worker] checking %s sub %q: local_time=%s, preferred=%s, next_payment=%s",
		userLabel, sub.Name,
		localNow.Format("2006-01-02 15:04"),
		preferredHHMM,
		sub.NextPaymentAt.In(loc).Format("2006-01-02 15:04"),
	)

	if localNow.Format("15:04") < preferredHHMM {
		log.Printf("[notification-worker] skip %s sub %q: too early (now %s < preferred %s)",
			userLabel, sub.Name, localNow.Format("15:04"), preferredHHMM)
		return false
	}

	next := sub.NextPaymentAt.In(loc)
	tomorrow := localNow.AddDate(0, 0, 1)
	if next.Year() != tomorrow.Year() ||
		next.Month() != tomorrow.Month() ||
		next.Day() != tomorrow.Day() {
		log.Printf("[notification-worker] skip %s sub %q: payment date %s is not tomorrow (%s)",
			userLabel, sub.Name,
			next.Format("2006-01-02"),
			tomorrow.Format("2006-01-02"),
		)
		return false
	}

	if sub.NotifiedAt != nil && now.Sub(*sub.NotifiedAt) < notificationDedupWindow {
		log.Printf("[notification-worker] skip %s sub %q: already notified at %s (dedup window %s)",
			userLabel, sub.Name,
			sub.NotifiedAt.Format(time.RFC3339),
			notificationDedupWindow,
		)
		return false
	}

	return true
}

// renewKeyboard builds the two-button inline keyboard that ships with
// every renewal reminder. Clicking either button fires a CallbackQuery
// handled by internal/bot/bot.go:
//
//	"Paid (Renew)"  → advance NextPaymentAt by Period + clear notified_at
//	"Cancelled"     → delete the subscription row
//
// Both buttons live on the same row so the user picks one in a single
// glance instead of scrolling through a vertical menu.
func renewKeyboard(sub *model.Subscription, locale string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{
				Text:         renewButtonLabel(locale),
				CallbackData: tgutil.RenewCallbackPrefix + sub.ID.String(),
			},
			{
				Text:         cancelButtonLabel(locale),
				CallbackData: tgutil.CancelCallbackPrefix + sub.ID.String(),
			},
		}},
	}
}

func renewButtonLabel(locale string) string {
	if locale == "ru" {
		return "✅ Оплачено"
	}
	return "✅ Paid"
}

func cancelButtonLabel(locale string) string {
	if locale == "ru" {
		return "❌ Отменил"
	}
	return "❌ Cancelled"
}

// buildReminderText assembles the renewal-reminder Telegram message,
// localized to the user's locale. The message is structured as a
// multi-line card with clear visual hierarchy:
//
//	🔔 *Завтра списание по подписке*
//
//	🏷 *Netflix* _(Для Кристины)_
//	💳 К оплате: *15.49 USD*
//
//	Выберите действие ниже:
//
// The note line renders in italic only when sub.Note is non-empty.
// All user-supplied text is escaped for legacy Markdown safety.
func buildReminderText(sub *model.Subscription, locale string) string {
	escapedName := escapeTelegramMarkdown(sub.Name)

	// Note: italic in legacy Markdown is _text_
	notePart := ""
	if n := strings.TrimSpace(sub.Note); n != "" {
		notePart = " _(" + escapeTelegramMarkdown(n) + ")_"
	}

	amountStr := fmt.Sprintf("%.2f %s", sub.Amount, sub.Currency)

	switch locale {
	case "ru":
		return fmt.Sprintf(
			"🔔 *Завтра списание по подписке*\n\n"+
				"🏷 *%s*%s\n"+
				"💳 К оплате: *%s*\n\n"+
				"Выберите действие ниже:",
			escapedName, notePart, amountStr,
		)
	default:
		return fmt.Sprintf(
			"🔔 *Subscription payment tomorrow*\n\n"+
				"🏷 *%s*%s\n"+
				"💳 Amount due: *%s*\n\n"+
				"Choose an action below:",
			escapedName, notePart, amountStr,
		)
	}
}

// escapeTelegramMarkdown delegates to the shared tgutil package.
func escapeTelegramMarkdown(s string) string {
	return tgutil.EscapeMarkdown(s)
}

// isValidHHMM verifies that a stored notification_time is in the strict
// "HH:MM" 24-hour format we depend on for lexical comparison. The PATCH /me
// handler enforces this on write via regexp, so this guard is defence in
// depth — protects against rows predating the field or any direct DB edits.
// ForceNotifyUser runs the notification logic for a specific user, ignoring
// time-of-day checks and the notified_at dedup flag. Designed for admin
// testing via the /force_notify bot command.
func (w *NotificationWorker) ForceNotifyUser(ctx context.Context, userID uint) (sent int, total int, err error) {
	now := time.Now().UTC()

	var user model.User
	if err := w.db.WithContext(ctx).First(&user, userID).Error; err != nil {
		return 0, 0, fmt.Errorf("user lookup: %w", err)
	}

	loc, locErr := time.LoadLocation(user.Timezone)
	if locErr != nil || loc == nil {
		loc = time.UTC
	}

	// Find ALL subscriptions for this user — no time/dedup filtering.
	var subs []model.Subscription
	if err := w.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&subs).Error; err != nil {
		return 0, 0, fmt.Errorf("subscriptions query: %w", err)
	}

	// Filter to those whose next_payment lands "tomorrow" in the user's
	// local timezone.
	var candidates []model.Subscription
	localNow := now.In(loc)
	localTomorrow := localNow.AddDate(0, 0, 1)
	for _, sub := range subs {
		next := sub.NextPaymentAt.In(loc)
		if next.Year() == localTomorrow.Year() &&
			next.Month() == localTomorrow.Month() &&
			next.Day() == localTomorrow.Day() {
			candidates = append(candidates, sub)
		}
	}

	total = len(candidates)
	log.Printf("[force-notify] user %d: %d subs total, %d due tomorrow (%s)",
		user.TelegramID, len(subs), total, localTomorrow.Format("2006-01-02"))

	for _, sub := range candidates {
		text := buildReminderText(&sub, user.Locale)
		markup := renewKeyboard(&sub, user.Locale)

		if sendErr := w.notifier.SendMessageWithMarkup(ctx, user.TelegramID, text, markup); sendErr != nil {
			log.Printf("[force-notify] send failed for sub %q: %v", sub.Name, sendErr)
			continue
		}
		log.Printf("[force-notify] ✓ sent sub %q (%.2f %s) to user %d",
			sub.Name, sub.Amount, sub.Currency, user.TelegramID)
		sent++
	}

	return sent, total, nil
}

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
