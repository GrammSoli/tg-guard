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
	"github.com/subguard/backend/internal/observability"
	"github.com/subguard/backend/internal/tgutil"
)

// RoomReminderWorker DMs every member of a shared room the day before its
// monthly billing_day, so nobody is caught off guard when the billing-reset
// worker clears the paid flags. It is the automatic counterpart to the
// owner-initiated manual reminder in handler/room.go.
type RoomReminderWorker struct {
	db           *gorm.DB
	notifier     notifier.Notifier
	baseURL      string
	reminderHour int // UTC hour at which the once-daily reminder pass runs
}

func NewRoomReminderWorker(db *gorm.DB, n notifier.Notifier, baseURL string, reminderHour int) *RoomReminderWorker {
	return &RoomReminderWorker{db: db, notifier: n, baseURL: baseURL, reminderHour: reminderHour}
}

// Start runs the reminder loop: once on boot, then hourly. The actual work
// only happens on the tick that lands in reminderHour (UTC).
func (w *RoomReminderWorker) Start(ctx context.Context) {
	log.Println("[room-reminder] starting")

	w.check(ctx)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[room-reminder] stopped")
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// billsTomorrow reports whether a room with the given billing_day has its
// next monthly charge on the day after `now` (UTC). A billing_day past the
// end of a short month is clamped to that month's last day, mirroring
// BillingResetWorker so the reminder and the reset agree on the date.
func billsTomorrow(billingDay int, now time.Time) bool {
	tomorrow := now.AddDate(0, 0, 1)
	y, m, _ := tomorrow.Date()
	daysInMonth := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	effective := billingDay
	if effective > daysInMonth {
		effective = daysInMonth
	}
	return tomorrow.Day() == effective
}

func (w *RoomReminderWorker) check(ctx context.Context) {
	now := time.Now().UTC()
	// One pass per day, at the configured hour. Other ticks return early.
	if now.Hour() != w.reminderHour {
		return
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// last_billing_reminder_at is the per-room "already reminded today"
	// stamp — a second tick within the same hour (e.g. a mid-window
	// restart) finds nothing and is a silent no-op instead of
	// double-messaging every member.
	var rooms []model.SharedRoom
	err := w.db.WithContext(ctx).
		Preload("Members.User").
		Where("last_billing_reminder_at IS NULL OR last_billing_reminder_at < ?", todayStart).
		Find(&rooms).Error
	if err != nil {
		log.Printf("[room-reminder] query error: %v", err)
		observability.CaptureException(err)
		return
	}

	for i := range rooms {
		select {
		case <-ctx.Done():
			return
		default:
		}
		room := &rooms[i]
		if !billsTomorrow(room.BillingDay, now) {
			continue
		}
		w.remindRoom(ctx, room, now)
	}
}

// remindRoom messages every eligible member of one room and then stamps
// last_billing_reminder_at so the room is skipped for the rest of the day.
func (w *RoomReminderWorker) remindRoom(ctx context.Context, room *model.SharedRoom, now time.Time) {
	// Room name is user-supplied and the notifier sends with Markdown
	// ParseMode — escape once so a name like "*x*" can't break parsing.
	escapedName := tgutil.EscapeMarkdown(room.Name)
	url := fmt.Sprintf("%s/?room=%s", strings.TrimRight(w.baseURL, "/"), room.ID)

	sent := 0
	for j := range room.Members {
		select {
		case <-ctx.Done():
			return
		default:
		}
		u := room.Members[j].User
		if u == nil || u.TelegramID == 0 || u.IsBanned || !u.NotificationsEnabled {
			continue
		}
		text, btnText := roomReminderStrings(escapedName, u.Locale)
		kb := &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{{
				Text:   btnText,
				WebApp: &models.WebAppInfo{URL: url},
			}}},
		}
		if err := w.notifier.SendMessageWithMarkup(ctx, u.TelegramID, text, kb); err != nil {
			log.Printf("[room-reminder] send to %d failed (room %s): %v", u.TelegramID, room.ID, err)
			continue
		}
		sent++
	}

	// Stamp regardless of per-member failures: this room's pass for today
	// is done, and re-running would re-message everyone who already got it.
	// Fresh background context so a shutdown mid-pass still records it.
	stampCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.db.WithContext(stampCtx).Model(&model.SharedRoom{}).
		Where("id = ?", room.ID).
		Update("last_billing_reminder_at", now).Error; err != nil {
		log.Printf("[room-reminder] stamp room %s failed: %v", room.ID, err)
		observability.CaptureException(err)
	}
	log.Printf("[room-reminder] room %q (%s): reminded %d/%d members",
		room.Name, room.ID, sent, len(room.Members))
}

// roomReminderStrings returns the localized message body and button label
// for the day-before billing reminder.
func roomReminderStrings(escapedRoomName, locale string) (text, button string) {
	if strings.HasPrefix(locale, "ru") {
		return fmt.Sprintf("🔔 Завтра день оплаты в комнате «%s». Не забудьте про свою долю.", escapedRoomName),
			"Перейти в комнату"
	}
	return fmt.Sprintf("🔔 Tomorrow is billing day for the «%s» room. Don't forget your share.", escapedRoomName),
		"Open room"
}
