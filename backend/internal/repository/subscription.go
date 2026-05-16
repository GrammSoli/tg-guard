package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
)

// SubscriptionRepo handles CRUD operations for subscriptions.
type SubscriptionRepo struct {
	db *gorm.DB
}

func NewSubscriptionRepo(db *gorm.DB) *SubscriptionRepo {
	return &SubscriptionRepo{db: db}
}

func (r *SubscriptionRepo) ListByUser(userID uint) ([]model.Subscription, error) {
	var subs []model.Subscription
	err := r.db.Where("user_id = ?", userID).Order("next_payment_at ASC").Find(&subs).Error
	return subs, err
}

func (r *SubscriptionRepo) GetByID(id uuid.UUID, userID uint) (*model.Subscription, error) {
	var sub model.Subscription
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *SubscriptionRepo) Create(sub *model.Subscription) error {
	return r.db.Create(sub).Error
}

// Update writes the user-editable fields of a subscription. When the
// caller has just changed NextPaymentAt or Period, pass `clearNotified
// = true` so the worker's dedup flag (notified_at) is reset and a fresh
// reminder fires on the new date. Without this, editing a sub from
// "tomorrow" to "next month" would block the reminder for ~20h after
// the new date arrives.
//
// Single UPDATE (one round-trip + atomic). The previous implementation
// did two sequential UPDATEs — field set, then `notified_at = NULL` —
// outside a transaction; the notification worker could read the
// already-updated row mid-pair, see a stale `notified_at` against the
// new `next_payment_at`, and either fire a duplicate reminder or
// suppress one entirely depending on the interleave. Now both writes
// land in the same statement so the worker sees a consistent row.
// Audit Tier-1 #3.
func (r *SubscriptionRepo) Update(sub *model.Subscription, clearNotified bool) error {
	cols := []string{
		"Name", "Brand", "Tag", "Note", "IconName", "IconColor",
		"Amount", "Currency", "Period", "NextPaymentAt",
		"IsTrial", "IsAutoPay",
	}
	if clearNotified {
		// Setting the pointer to nil before .Updates(struct) makes
		// GORM emit `notified_at = NULL` inline with the other fields.
		sub.NotifiedAt = nil
		cols = append(cols, "NotifiedAt")
	}
	return r.db.Model(sub).Select(cols).Updates(sub).Error
}

// Delete removes the subscription that matches BOTH id AND userID.
// Returns rowsAffected so the handler can distinguish "deleted" (≥1)
// from "row didn't exist or wasn't yours" (0) — that boundary becomes
// the 200 / 404 split at the API. Without it, the API used to confirm
// "deleted":true for any UUID the client sent (including UUIDs of
// other users' rows that the WHERE clause silently filtered out),
// which masked IDOR probes as successful operations.
func (r *SubscriptionRepo) Delete(id uuid.UUID, userID uint) (int64, error) {
	res := r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Subscription{})
	return res.RowsAffected, res.Error
}
