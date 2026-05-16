package worker

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/observability"
	"github.com/subguard/backend/internal/timezone"
)

// BillingResetWorker resets payment statuses on the billing day for each room.
type BillingResetWorker struct {
	db *gorm.DB
}

func NewBillingResetWorker(db *gorm.DB) *BillingResetWorker {
	return &BillingResetWorker{db: db}
}

// Start launches the billing reset check loop. Ticks every hour because the
// "is local midnight" check is now per-room: a tick at any UTC hour can be
// the right moment for some room somewhere on Earth.
func (w *BillingResetWorker) Start(ctx context.Context) {
	log.Println("[billing-reset] starting")

	// Check immediately on boot, then every hour
	w.check(ctx)

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[billing-reset] stopped")
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// check accepts ctx so its DB queries respect graceful shutdown. Without
// WithContext the queries would continue against a potentially-closing
// connection pool and raise "database is closed" errors mid-tick.
//
// Per-room timezone semantics: a room with Timezone = "Australia/Sydney"
// and BillingDay = 15 resets when its OWN local clock is at 00:00–01:59
// on the 15th — not when 15 Sept 00:00 UTC arrives. We pre-filter at
// the DB layer (rooms not reset in the last ~23h) and then evaluate
// roomDueForResetNow() per row, because different rooms in different
// zones can never share a single SQL day-of-month predicate.
//
// Idempotency stays on last_billing_reset_at: a second tick within the
// same local day picks up zero rows (the 23h-ago window has already
// moved past the stamp), so a server restart in the 00:00–01:59 window
// is a no-op instead of double-clobbering payments members made in
// between.
func (w *BillingResetWorker) check(ctx context.Context) {
	nowUTC := time.Now().UTC()
	// Anywhere on Earth, the local "today" can only have started within
	// the last ~23 hours. Anything stamped more recently than that has
	// already been handled for its current local day.
	twentyThreeHoursAgo := nowUTC.Add(-23 * time.Hour)

	var rooms []model.SharedRoom
	err := w.db.WithContext(ctx).
		Where("last_billing_reset_at IS NULL OR last_billing_reset_at < ?", twentyThreeHoursAgo).
		Find(&rooms).Error
	if err != nil {
		log.Printf("[billing-reset] query error: %v", err)
		observability.CaptureException(err)
		return
	}

	if len(rooms) == 0 {
		return
	}

	eligible := 0
	for i := range rooms {
		select {
		case <-ctx.Done():
			return
		default:
		}
		room := &rooms[i]
		if !roomDueForResetNow(room, nowUTC) {
			continue
		}
		eligible++

		// Atomic reset + stamp. If either fails the whole pair rolls
		// back so the room remains "not reset today" for the next tick.
		txErr := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&model.RoomMember{}).
				Where("room_id = ? AND has_paid = true", room.ID).
				Updates(map[string]interface{}{"has_paid": false, "paid_at": nil})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				log.Printf("[billing-reset] reset %d members for room %q (ID: %s, tz: %s)",
					result.RowsAffected, room.Name, room.ID, room.Timezone)
			}
			return tx.Model(&model.SharedRoom{}).
				Where("id = ?", room.ID).
				Update("last_billing_reset_at", nowUTC).Error
		})
		if txErr != nil {
			log.Printf("[billing-reset] error processing room %s: %v", room.ID, txErr)
			observability.CaptureException(txErr)
			continue
		}
	}

	if eligible > 0 {
		log.Printf("[billing-reset] %d rooms eligible this tick (of %d candidates)",
			eligible, len(rooms))
	}
}

// roomDueForResetNow reports whether, in the room's own timezone,
// localNow is in the 00:00–01:59 window AND today is the room's
// billing day (with last-day-of-short-month clamping for billing_day
// values > the current month's length).
//
// The 2-hour window is the safety net for a worker that misses one
// tick (deploy / restart). Idempotency stays on last_billing_reset_at,
// so a second tick in the same window is filtered out at the DB layer.
func roomDueForResetNow(room *model.SharedRoom, nowUTC time.Time) bool {
	loc := timezone.LoadOrUTC(room.Timezone)
	localNow := nowUTC.In(loc)
	if localNow.Hour() > 1 {
		return false
	}
	today := localNow.Day()
	// daysInMonth needs to be computed in the SAME Location as localNow.
	// A DST midnight transition can otherwise give an off-by-one month
	// length.
	daysInMonth := time.Date(localNow.Year(), localNow.Month()+1, 0, 0, 0, 0, 0, loc).Day()
	if today == daysInMonth {
		return room.BillingDay == today || room.BillingDay > daysInMonth
	}
	return room.BillingDay == today
}
