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
	w.check()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[billing-reset] stopped")
			return
		case <-ticker.C:
			w.check()
		}
	}
}

func (w *BillingResetWorker) check() {
	now := time.Now().UTC()
	today := now.Day()

	// Only run the reset between 00:00–01:00 UTC to avoid duplicate resets
	if now.Hour() > 1 {
		return
	}

	var rooms []model.SharedRoom
	err := w.db.Where("billing_day = ?", today).Find(&rooms).Error
	if err != nil {
		log.Printf("[billing-reset] query error: %v", err)
		return
	}

	if len(rooms) == 0 {
		return
	}

	log.Printf("[billing-reset] found %d rooms with billing_day=%d, resetting payment statuses", len(rooms), today)

	for _, room := range rooms {
		result := w.db.Model(&model.RoomMember{}).
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
