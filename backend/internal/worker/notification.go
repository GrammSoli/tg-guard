package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
	"github.com/subguard/backend/internal/observability"
	"github.com/subguard/backend/internal/tgutil"
	"github.com/subguard/backend/internal/timezone"
	"github.com/subguard/backend/internal/workerutil"
)

// notificationBatchSize streams candidate subscriptions in chunks so the
// worker can't OOM if a single tick has tens of thousands of due rows.
const notificationBatchSize = 500

// persistChunkSize caps how many IDs we cram into a single bulk-update
// IN (…) clause. PostgreSQL's extended-protocol parameter limit is
// 65535; we leave plenty of headroom so a backlogged tick (e.g. worker
// caught up after a long outage) doesn't trip the limit and leave
// reminders un-marked. Audit Tier-3 #3.
const persistChunkSize = 1000

// maxSendRetries caps retry_after-driven retries per subscription on a
// 429 from Telegram. Two retries plus the original attempt is enough for
// the typical Bot API back-off; on a third failure we give up for this
// tick — the dedup window keeps us from hammering on the next tick.
const maxSendRetries = 2

// errBatchCancelled is the sentinel error FindInBatches returns when we
// abort iteration because the parent context was cancelled. Treated as
// success in the surrounding logic — we already wrote what we could.
var errBatchCancelled = errors.New("batch cancelled by ctx")

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

// defaultNotifyMinutes is the fallback "10:00" preference, pre-parsed.
// Used when a stored NotificationTime fails isValidHHMM (legacy rows /
// hand-edited DB rows). Matches the default the PATCH /me handler
// writes via notificationTimePattern.
const defaultNotifyMinutes = 10 * 60

// parseHHMMToMinutes turns an "HH:MM" string into minutes from midnight.
// Assumes the caller has already validated the format via isValidHHMM —
// no allocation, branch-free arithmetic on the four digit bytes.
func parseHHMMToMinutes(s string) int {
	return int(s[0]-'0')*600 +
		int(s[1]-'0')*60 +
		int(s[3]-'0')*10 +
		int(s[4]-'0')
}

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
	defer observability.TimeWorkerTick("notification")()
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Notification kill-switch. When the admin has flipped
	// pause_notifications on, skip the whole tick — nothing is sent and
	// nothing is marked notified, so once the switch is flipped back
	// every due reminder fires on the next tick exactly as normal.
	var appSettings model.AppSettings
	if err := w.db.WithContext(ctx).Select("pause_notifications").First(&appSettings, 1).Error; err != nil {
		// Fail open: a settings-read error shouldn't silently stop
		// reminders. Log and proceed with the normal tick.
		log.Printf("[notification-worker] settings read failed, proceeding: %v", err)
	} else if appSettings.PauseNotifications {
		log.Println("[notification-worker] ── tick skipped ── pause_notifications is ON")
		return
	}

	now := time.Now().UTC()
	log.Printf("[notification-worker] ── tick ── UTC now: %s", now.Format(time.RFC3339))

	// Coarse DB pre-filter — see notificationWindow. The precise per-user
	// "is it tomorrow + has their preferred time arrived" logic lives in
	// shouldSendNow below.
	windowStart, windowEnd := notificationWindow(now)

	// Track successfully-sent IDs so we can bulk-update notified_at in a
	// single statement at the end. Stream via FindInBatches so a tick
	// with 100k due rows doesn't load them all into memory.
	sentIDs := make([]uuid.UUID, 0, notificationBatchSize)
	var seen, sentCount int

	dest := &[]model.Subscription{}
	err := w.db.WithContext(ctx).
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, telegram_id, username, notifications_enabled, timezone, notification_time, locale, is_banned")
		}).
		Where("next_payment_at BETWEEN ? AND ?", windowStart, windowEnd).
		Where("notified_at IS NULL OR notified_at < ?", now.Add(-notificationDedupWindow)).
		Order("next_payment_at ASC, id ASC").
		FindInBatches(dest, notificationBatchSize, func(tx *gorm.DB, batchNum int) error {
			batchSubs, ok := tx.Statement.Dest.(*[]model.Subscription)
			if !ok {
				return errors.New("unexpected batch dest type")
			}
			seen += len(*batchSubs)

			for i := range *batchSubs {
				select {
				case <-ctx.Done():
					return errBatchCancelled
				default:
				}
				sub := &(*batchSubs)[i]
				if w.tryProcessOne(ctx, sub, now) {
					sentIDs = append(sentIDs, sub.ID)
					sentCount++
				}
			}
			return nil
		}).Error

	if err != nil && !errors.Is(err, errBatchCancelled) && !errors.Is(err, context.Canceled) {
		log.Printf("[notification-worker] batch iteration error: %v", err)
		observability.CaptureException(err)
	}

	if seen == 0 {
		log.Println("[notification-worker] no candidate subscriptions found")
	} else {
		log.Printf("[notification-worker] processed %d candidates, sent %d reminders", seen, sentCount)
	}

	// Bulk-persist notified_at in chunks. Use a fresh short context per
	// chunk so that even on SIGTERM mid-tick we still record what already
	// went out — otherwise the next worker start would re-send the same
	// reminders. The parent ctx may be cancelled at this point, so we
	// deliberately start from context.Background() with a per-chunk
	// budget.
	//
	// Chunking by persistChunkSize is required because PostgreSQL caps
	// statement parameters at 65535. A single `WHERE id IN (?, ?, …)`
	// for, say, 30k subs in a backlogged tick would silently fail with
	// "extended protocol limited to 65535 parameters" and leave all
	// those subs un-marked → duplicate reminders on the next tick.
	// Audit Tier-3 #3.
	for start := 0; start < len(sentIDs); start += persistChunkSize {
		end := start + persistChunkSize
		if end > len(sentIDs) {
			end = len(sentIDs)
		}
		chunk := sentIDs[start:end]
		persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := w.db.WithContext(persistCtx).
			Model(&model.Subscription{}).
			Where("id IN ?", chunk).
			Update("notified_at", now).Error; err != nil {
			log.Printf("[notification-worker] bulk notified_at update failed for chunk %d-%d of %d: %v",
				start, end, len(sentIDs), err)
			observability.CaptureException(err)
		}
		persistCancel()
	}
}

// notificationWindow is the coarse [start, end] range the DB pre-filter
// uses to pick reminder candidates. It must be a safe SUPERSET of every
// subscription shouldSendNow could pass: a payment that is "tomorrow" in
// some user's timezone has next_payment_at anywhere from ~now (easternmost
// tz, UTC+14) to ~now+48h (westernmost tz, UTC-12, late in their day).
//
// Returning `now` as the lower bound — not the old now+20h — is what lets a
// subscription created the same day it falls due still receive a reminder.
// With +20h, a sub created less than ~20h before payment was never observed
// by a tick while it sat 20-30h out, so it got no reminder at all.
func notificationWindow(now time.Time) (start, end time.Time) {
	return now, now.Add(48 * time.Hour)
}

// tryProcessOne is the per-subscription gate + send loop. Returns true iff
// the reminder was delivered (so the caller should mark notified_at).
// All skip reasons are logged inside the helper or shouldSendNow.
func (w *NotificationWorker) tryProcessOne(ctx context.Context, sub *model.Subscription, now time.Time) bool {
	userLabel := fmt.Sprintf("user %d (@%s)", sub.User.TelegramID, sub.User.Username)

	if sub.User.TelegramID == 0 {
		log.Printf("[notification-worker] skip %s sub %q: telegram_id=0", userLabel, sub.Name)
		return false
	}
	if !sub.User.NotificationsEnabled {
		log.Printf("[notification-worker] skip %s sub %q: notifications disabled", userLabel, sub.Name)
		return false
	}
	if sub.User.IsBanned {
		log.Printf("[notification-worker] skip %s sub %q: user is banned", userLabel, sub.Name)
		return false
	}
	if !shouldSendNow(sub, &sub.User, now) {
		return false // reason already logged inside shouldSendNow
	}

	text := buildReminderText(sub, sub.User.Locale)
	markup := renewKeyboard(sub, sub.User.Locale)

	if !w.sendWithRetry(ctx, sub.User.TelegramID, text, markup, userLabel, sub.Name) {
		return false
	}
	log.Printf("[notification-worker] ✓ notification sent to %s for sub %q (%.2f %s)",
		userLabel, sub.Name, sub.Amount, sub.Currency)
	return true
}

// sendWithRetry calls the notifier and re-tries on Telegram 429 using the
// retry_after hint embedded in the error string. Caps at maxSendRetries
// retries so a misbehaving API can't trap the worker for a full hour.
// Returns true on success, false if all retries failed or ctx was
// cancelled mid-back-off.
func (w *NotificationWorker) sendWithRetry(
	ctx context.Context,
	chatID int64,
	text string,
	markup *models.InlineKeyboardMarkup,
	userLabel, subName string,
) bool {
	for attempt := 0; attempt <= maxSendRetries; attempt++ {
		err := w.notifier.SendMessageWithMarkup(ctx, chatID, text, markup)
		if err == nil {
			observability.NotificationsSentTotal.WithLabelValues("sent").Inc()
			return true
		}

		delay, isRateLimit := workerutil.ParseRetryAfter(err)
		if isRateLimit && attempt < maxSendRetries {
			log.Printf("[notification-worker] 429 for %s sub %q — sleeping %s (attempt %d/%d)",
				userLabel, subName, delay, attempt+1, maxSendRetries)
			select {
			case <-ctx.Done():
				observability.NotificationsSentTotal.WithLabelValues("aborted").Inc()
				return false
			case <-time.After(delay):
			}
			continue
		}

		log.Printf("[notification-worker] SEND FAILED %s sub %q (attempt %d): %v",
			userLabel, subName, attempt+1, err)
		// Bucket the failure: permanent (blocked/deactivated user) vs
		// transient (network / 5xx). Alerting can ignore the permanent
		// bucket — it's normal background churn that doesn't indicate
		// a system problem.
		if workerutil.IsPermanentSendFailure(err) {
			observability.NotificationsSentTotal.WithLabelValues("permanent_failure").Inc()
		} else if isRateLimit {
			observability.NotificationsSentTotal.WithLabelValues("rate_limited").Inc()
		} else {
			observability.NotificationsSentTotal.WithLabelValues("failed").Inc()
		}
		return false
	}
	return false
}

// shouldSendNow decides whether a candidate subscription deserves a
// reminder right now. Three gates, evaluated in the user's own timezone:
//
//  1. The user's preferred HH:MM has already passed today. Compared as
//     minutes-from-midnight (not lexical string compare!) so the day
//     boundary works correctly — a previous bug had "00:10" < "23:30"
//     evaluate true, blocking reminders for users whose preferred time
//     was late-evening once midnight rolled over.
//  2. The next payment falls on "tomorrow" relative to the user's
//     localNow.AddDate(0, 0, 1) — covers users whose UTC day boundary
//     doesn't match the server's.
//  3. We haven't fired a notification for this row inside the dedup
//     window. The DB query already filters on this, but checking again
//     here keeps the in-memory loop honest if the data races with another
//     writer (e.g. callback handler).
//
// All three must hold. Time-of-day failure means "too early today" —
// we'll catch it on a later tick AND the next-day gate will then reject
// the same row, so a "preferred=23:30 + tick=23:00" sub correctly fires
// at the 23:30 tick and then gets dedup-blocked by notified_at.
//
// LoadLocation hits the package-level sync.Map cache in internal/timezone,
// so calling this once per subscription is cheap — no per-tick map plumbing
// required.
func shouldSendNow(sub *model.Subscription, u *model.User, now time.Time) bool {
	userLabel := fmt.Sprintf("user %d (@%s)", u.TelegramID, u.Username)

	loc := timezone.LoadOrUTC(u.Timezone)
	prefMin := defaultNotifyMinutes
	if isValidHHMM(u.NotificationTime) {
		prefMin = parseHHMMToMinutes(u.NotificationTime)
	}

	localNow := now.In(loc)
	nowMin := localNow.Hour()*60 + localNow.Minute()

	log.Printf("[notification-worker] checking %s sub %q: local_time=%s, preferred_min=%d, next_payment=%s",
		userLabel, sub.Name,
		localNow.Format("2006-01-02 15:04"),
		prefMin,
		sub.NextPaymentAt.In(loc).Format("2006-01-02 15:04"),
	)

	if nowMin < prefMin {
		log.Printf("[notification-worker] skip %s sub %q: too early (now %dm < preferred %dm)",
			userLabel, sub.Name, nowMin, prefMin)
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
				Text:         renewButtonLabel(locale, sub.IsTrial),
				CallbackData: tgutil.RenewCallbackPrefix + sub.ID.String(),
			},
			{
				Text:         cancelButtonLabel(locale, sub.IsTrial),
				CallbackData: tgutil.CancelCallbackPrefix + sub.ID.String(),
			},
		}},
	}
}

func renewButtonLabel(locale string, isTrial bool) string {
	if isTrial {
		if locale == "ru" {
			return "✅ Оставить (Платная)"
		}
		return "✅ Keep (Paid)"
	}
	if locale == "ru" {
		return "✅ Оплачено"
	}
	return "✅ Paid"
}

func cancelButtonLabel(locale string, isTrial bool) string {
	if isTrial {
		if locale == "ru" {
			return "❌ Отменил триал"
		}
		return "❌ Cancelled trial"
	}
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

	if sub.IsTrial {
		return buildTrialReminderText(escapedName, notePart, amountStr, locale)
	}

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

func buildTrialReminderText(name, notePart, amountStr, locale string) string {
	switch locale {
	case "ru":
		return fmt.Sprintf(
			"❗ *Завтра заканчивается пробный период!*\n\n"+
				"🏷 *%s*%s\n"+
				"💳 Будет списано: *%s*\n\n"+
				"Выберите действие ниже:",
			name, notePart, amountStr,
		)
	default:
		return fmt.Sprintf(
			"❗ *Trial period ends tomorrow!*\n\n"+
				"🏷 *%s*%s\n"+
				"💳 Will be charged: *%s*\n\n"+
				"Choose an action below:",
			name, notePart, amountStr,
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

	loc := timezone.LoadOrUTC(user.Timezone)

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
