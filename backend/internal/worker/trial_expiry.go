package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/observability"
)

// trialCheckInterval is how often elapsed trials are swept. Conversion is
// not time-critical to the minute — the card already reads trial_ends_at
// live — so hourly keeps the query load trivial.
const trialCheckInterval = 1 * time.Hour

// TrialExpiryWorker converts trial subscriptions whose trial_ends_at has
// passed into regular subscriptions (is_trial=false, trial_ends_at=NULL).
// This mirrors what the bot's "Renew" callback does on conversion, but for
// trials the user simply let lapse without acting on the reminder — without
// this sweep is_trial would stay true forever.
type TrialExpiryWorker struct {
	db *gorm.DB
}

func NewTrialExpiryWorker(db *gorm.DB) *TrialExpiryWorker {
	return &TrialExpiryWorker{db: db}
}

// Start runs the sweep once on boot, then every trialCheckInterval.
func (w *TrialExpiryWorker) Start(ctx context.Context) {
	log.Println("[trial-expiry] starting")

	w.check(ctx)

	ticker := time.NewTicker(trialCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[trial-expiry] stopped")
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// check flips every elapsed trial to a regular subscription in a single
// statement. next_payment_at is intentionally left untouched: a lapsed
// trial was not actively renewed, so its existing due date stands — the
// card then shows Overdue if that date is already past. Idempotent — a
// converted row no longer matches is_trial = true.
func (w *TrialExpiryWorker) check(ctx context.Context) {
	now := time.Now().UTC()
	res := w.db.WithContext(ctx).Model(&model.Subscription{}).
		Where("is_trial = ? AND trial_ends_at IS NOT NULL AND trial_ends_at < ?", true, now).
		Updates(map[string]interface{}{"is_trial": false, "trial_ends_at": nil})
	if res.Error != nil {
		if errors.Is(res.Error, context.Canceled) || errors.Is(res.Error, context.DeadlineExceeded) {
			return
		}
		log.Printf("[trial-expiry] update error: %v", res.Error)
		observability.CaptureException(res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[trial-expiry] converted %d expired trial(s) to regular subscriptions", res.RowsAffected)
	}
}
