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
func (r *SubscriptionRepo) Update(sub *model.Subscription, clearNotified bool) error {
	tx := r.db.Model(sub).Select(
		"Name", "Brand", "Tag", "Note", "IconName", "IconColor",
		"Amount", "Currency", "Period", "NextPaymentAt",
		"IsTrial", "IsAutoPay",
	)
	if err := tx.Updates(sub).Error; err != nil {
		return err
	}
	if clearNotified {
		return r.db.Model(&model.Subscription{}).
			Where("id = ?", sub.ID).
			Update("notified_at", nil).Error
	}
	return nil
}

func (r *SubscriptionRepo) Delete(id uuid.UUID, userID uint) error {
	return r.db.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Subscription{}).Error
}
