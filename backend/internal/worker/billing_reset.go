package worker

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/observability"
)

// BillingResetWorker resets payment statuses on the billing day for each room.
type BillingResetWorker struct {
	db *gorm.DB
}

func NewBillingResetWorker(db *gorm.DB) *BillingResetWorker {
	return &BillingResetWorker{db: db}
}

// Start launches the billing reset check loop. Runs once per hour,
// but only acts on rooms whose billing_day matches the current day.
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
// Idempotency: rooms.last_billing_reset_at is the per-room "we already
// reset today" stamp. The eligibility query filters on it (NULL or
// strictly older than today's UTC midnight), and each successful reset
// transactionally writes it. A second tick within the same UTC day —
// whether from a server restart in the 00:00–01:00 window or from the
// hour-gate being relaxed in the future — picks up zero rows and is a
// silent no-op. Replaces the previous "emergent" idempotency that relied
// on the WHERE has_paid=true clause finding nothing on the second pass:
// that protection broke as soon as any member re-paid between the two
// ticks.
//
// Timezone semantics (audit Tier-4 #5): billing_day is interpreted in
// UTC, NOT in the user's stored timezone. Net effect: a room created
// by a Sydney user (UTC+10/+11) with billing_day=15 resets at
// 14 Sept 14:00 Sydney time = 15 Sept 00:00 UTC, not 15 Sept Sydney
// midnight. This is intentional for now — per-room TZ would require
// a `timezone` column on shared_rooms, a separate reset cursor per
// timezone slice, and a UI for the owner to pick. For the current
// audience (primarily RU/EN single-timezone groups), accepting the
// "all rooms reset at one global moment" model is simpler and the
// off-by-up-to-12h shift in the owner's local view of "billing day"
// is small relative to a monthly cycle. Re-evaluate when shared-
// rooms grows international.
func (w *BillingResetWorker) check(ctx context.Context) {
	now := time.Now().UTC()
	today := now.Day()
	daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Cheap optimisation: only scan in the 00:00–01:00 UTC window. The
	// column-based guard below is the real safety net; this just spares
	// the DB a query 22 times a day. If a deploy goes late and the
	// worker first ticks at 03:00, we'll miss today — accept that as the
	// existing contract, since loosening it would change reset timing
	// semantics that downstream UX (calendar view, reminders) depends on.
	if now.Hour() > 1 {
		return
	}

	// Match rooms whose billing_day == today, PLUS rooms whose stored
	// billing_day is greater than days-in-this-month and today is the last
	// day. Without that branch, a room with billing_day=31 would silently
	// skip February (28/29 days), April (30), June (30), September (30),
	// November (30).
	var rooms []model.SharedRoom
	q := w.db.WithContext(ctx).
		Where("last_billing_reset_at IS NULL OR last_billing_reset_at < ?", todayStart)
	if today == daysInMonth {
		q = q.Where("billing_day = ? OR billing_day > ?", today, daysInMonth)
	} else {
		q = q.Where("billing_day = ?", today)
	}
	err := q.Find(&rooms).Error
	if err != nil {
		log.Printf("[billing-reset] query error: %v", err)
		observability.CaptureException(err)
		return
	}

	if len(rooms) == 0 {
		return
	}

	log.Printf("[billing-reset] found %d rooms eligible for reset on day %d (month length=%d), resetting payment statuses",
		len(rooms), today, daysInMonth)

	for _, room := range rooms {
		select {
		case <-ctx.Done():
			return
		default:
		}

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
				log.Printf("[billing-reset] reset %d members for room %q (ID: %s)",
					result.RowsAffected, room.Name, room.ID)
			}
			return tx.Model(&model.SharedRoom{}).
				Where("id = ?", room.ID).
				Update("last_billing_reset_at", now).Error
		})
		if txErr != nil {
			log.Printf("[billing-reset] error processing room %s: %v", room.ID, txErr)
			// Surface to Sentry — silent DB-write failures here mean
			// members stay reset / unreset incorrectly, which is hard
			// to spot without log mining (audit observability hook
			// from PR #86).
			observability.CaptureException(txErr)
			continue
		}
	}
}
