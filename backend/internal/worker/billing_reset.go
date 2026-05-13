package worker

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
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
func (w *BillingResetWorker) check(ctx context.Context) {
	now := time.Now().UTC()
	today := now.Day()
	daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()

	// Only run the reset between 00:00–01:00 UTC to avoid duplicate resets
	if now.Hour() > 1 {
		return
	}

	// Match rooms whose billing_day == today, PLUS rooms whose stored
	// billing_day is greater than days-in-this-month and today is the last
	// day. Without that branch, a room with billing_day=31 would silently
	// skip February (28/29 days), April (30), June (30), September (30),
	// November (30).
	var rooms []model.SharedRoom
	q := w.db.WithContext(ctx).Where("billing_day = ?", today)
	if today == daysInMonth {
		q = w.db.WithContext(ctx).Where("billing_day = ? OR billing_day > ?", today, daysInMonth)
	}
	err := q.Find(&rooms).Error
	if err != nil {
		log.Printf("[billing-reset] query error: %v", err)
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
		result := w.db.WithContext(ctx).Model(&model.RoomMember{}).
			Where("room_id = ? AND has_paid = true", room.ID).
			Updates(map[string]interface{}{"has_paid": false, "paid_at": nil})
		if result.Error != nil {
			log.Printf("[billing-reset] error resetting room %s: %v", room.ID, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			log.Printf("[billing-reset] reset %d members for room %q (ID: %s)", result.RowsAffected, room.Name, room.ID)
		}
	}
}
