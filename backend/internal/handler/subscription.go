package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
	"github.com/subguard/backend/internal/timezone"
)

// errPaywallLimit is the in-tx sentinel for "subscription create denied
// by paywall." Surfaced out of Create's transaction closure via errors.Is
// so the surrounding handler can format the 403 response without
// reaching back into the closure for the count/limit values.
var errPaywallLimit = errors.New("paywall_limit")

// parseUserDate normalises an incoming next_payment_at / trial_ends_at
// string to a wall-clock anchored at NOON in the user's stored
// timezone. This is the single source of truth for "what calendar day
// did the user pick" — the notification worker only needs the day in
// the user's local zone to match, so any anchor between 00:00 and
// 24:00 local works; noon gives ±12h tolerance against worker
// scheduling drift and DST shifts.
//
// Two input shapes are accepted:
//
//  1. "yyyy-MM-dd" — the canonical form the (post-fix) frontend sends.
//     Parsed in the user's location, then bumped to noon.
//
//  2. RFC3339 — accepted for forward-compat with any future caller
//     that genuinely needs hour-precision. However a UTC-midnight
//     timestamp ("2026-05-15T00:00:00Z") is reinterpreted as "user
//     picked May 15" because that's exactly what the LEGACY frontend
//     produced by calling `new Date(dateOnly).toISOString()` — without
//     this reinterpretation, an old frontend pinned at midnight UTC
//     would fire reminders one day early for every user west of UTC.
//
// tz is the user's IANA name from users.timezone. Unknown / empty
// falls back to UTC so we degrade to today's pre-fix behaviour rather
// than crashing.
func parseUserDate(raw, tz string) (time.Time, error) {
	loc := timezone.LoadOrUTC(tz)

	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		// Legacy-shape detection: exactly UTC-midnight on this calendar
		// day → treat as date-only (see comment above).
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 &&
			t.Nanosecond() == 0 && t.Location() == time.UTC {
			return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, loc), nil
		}
		return t, nil
	}

	t, err := time.ParseInLocation("2006-01-02", raw, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: %w", raw, err)
	}
	return t.Add(12 * time.Hour), nil
}

// SubscriptionHandler handles subscription CRUD.
type SubscriptionHandler struct {
	repo      *repository.SubscriptionRepo
	adminRepo *repository.AdminRepo
	db        *gorm.DB
}

func NewSubscriptionHandler(db *gorm.DB) *SubscriptionHandler {
	return &SubscriptionHandler{
		repo:      repository.NewSubscriptionRepo(db),
		adminRepo: repository.NewAdminRepo(db),
		db:        db,
	}
}

// List returns all subscriptions for the authenticated user.
// GET /api/v1/subscriptions
func (h *SubscriptionHandler) List(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	subs, err := h.repo.ListByUser(user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch subscriptions"})
	}

	return c.JSON(subs)
}

// Create adds a new subscription.
// POST /api/v1/subscriptions
func (h *SubscriptionHandler) Create(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Name          string  `json:"name"`
		Brand         string  `json:"brand"`
		Tag           string  `json:"tag"`
		Note          string  `json:"note"`
		IconName      string  `json:"icon_name"`
		IconColor     string  `json:"icon_color"`
		Amount        float64 `json:"amount"`
		Currency      string  `json:"currency"`
		Period        string  `json:"period"`
		NextPaymentAt string  `json:"next_payment_at"`
		IsTrial       bool    `json:"is_trial"`
		TrialEndsAt   *string `json:"trial_ends_at"`
		IsAutoPay     bool    `json:"is_auto_pay"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	if body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	nextPayment, err := parseUserDate(body.NextPaymentAt, user.Timezone)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid next_payment_at date"})
	}

	sub := model.Subscription{
		UserID:        user.ID,
		Name:          body.Name,
		Brand:         defaultStr(body.Brand, "default"),
		Tag:           body.Tag,
		Note:          body.Note,
		IconName:      body.IconName,
		IconColor:     body.IconColor,
		Amount:        body.Amount,
		Currency:      defaultStr(body.Currency, "USD"),
		Period:        defaultStr(body.Period, "monthly"),
		NextPaymentAt: nextPayment,
		IsTrial:       body.IsTrial,
		IsAutoPay:     body.IsAutoPay,
	}

	if body.IsTrial && body.TrialEndsAt != nil {
		if t, err := parseUserDate(*body.TrialEndsAt, user.Timezone); err == nil {
			sub.TrialEndsAt = &t
		}
	}

	// Paywall + Create wrapped in a tx with SELECT … FOR UPDATE on the
	// user row. Without the lock two concurrent POST /subscriptions for
	// the same user could BOTH pass `count < limit` and BOTH insert,
	// overshooting the free-tier cap. Locking the user row makes the
	// COUNT-then-INSERT pair serialized per user (other users are
	// unaffected), and rolling back the tx on `errPaywallLimit` keeps
	// the count check authoritative even under contention.
	// Audit Tier-1 #4.
	var paywallCount, paywallLimit int64
	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		var locked model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", user.ID).First(&locked).Error; err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if !locked.IsDonator {
			settings, sErr := h.adminRepo.GetSettings()
			if sErr == nil && settings.PaywallEnabled {
				var count int64
				if err := tx.Model(&model.Subscription{}).
					Where("user_id = ?", user.ID).
					Count(&count).Error; err != nil {
					return fmt.Errorf("count subs: %w", err)
				}
				if count >= int64(settings.FreeSubsLimit) {
					paywallCount = count
					paywallLimit = int64(settings.FreeSubsLimit)
					return errPaywallLimit
				}
			}
		}
		return tx.Create(&sub).Error
	})
	if errors.Is(txErr, errPaywallLimit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "paywall_limit",
			"limit": paywallLimit,
			"count": paywallCount,
		})
	}
	if txErr != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create subscription"})
	}

	return c.Status(fiber.StatusCreated).JSON(sub)
}

// Update patches an existing subscription.
// PATCH /api/v1/subscriptions/:id
func (h *SubscriptionHandler) Update(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid subscription id"})
	}

	sub, err := h.repo.GetByID(id, user.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "subscription not found"})
	}

	// Pointer-typed fields so we can tell "key omitted" from "key present
	// with zero value" — same pattern as handler/user.go UpdateMe. Audit
	// A6: replaced the previous map[string]interface{} parsing which lost
	// type info (e.g. amount sent as int silently dropped, period change
	// detection broke when the frontend round-tripped the full object).
	var body struct {
		Name          *string  `json:"name"`
		Brand         *string  `json:"brand"`
		Tag           *string  `json:"tag"`
		Note          *string  `json:"note"`
		IconName      *string  `json:"icon_name"`
		IconColor    *string  `json:"icon_color"`
		Amount        *float64 `json:"amount"`
		Currency      *string  `json:"currency"`
		Period        *string  `json:"period"`
		IsTrial       *bool    `json:"is_trial"`
		IsAutoPay     *bool    `json:"is_auto_pay"`
		NextPaymentAt *string  `json:"next_payment_at"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	clearNotified := false
	if body.Name != nil {
		sub.Name = *body.Name
	}
	if body.Brand != nil {
		sub.Brand = *body.Brand
	}
	if body.Tag != nil {
		sub.Tag = *body.Tag
	}
	if body.Note != nil {
		sub.Note = *body.Note
	}
	if body.IconName != nil {
		sub.IconName = *body.IconName
	}
	if body.IconColor != nil {
		sub.IconColor = *body.IconColor
	}
	if body.Amount != nil {
		sub.Amount = *body.Amount
	}
	if body.Currency != nil {
		sub.Currency = *body.Currency
	}
	// Period: only flip clearNotified on actual value change (audit A4).
	if body.Period != nil && *body.Period != sub.Period {
		sub.Period = *body.Period
		clearNotified = true
	}
	if body.IsTrial != nil {
		sub.IsTrial = *body.IsTrial
	}
	if body.IsAutoPay != nil {
		sub.IsAutoPay = *body.IsAutoPay
	}
	if body.NextPaymentAt != nil {
		if t, err := parseUserDate(*body.NextPaymentAt, user.Timezone); err == nil {
			if !t.Equal(sub.NextPaymentAt) {
				clearNotified = true
			}
			sub.NextPaymentAt = t
		}
	}

	if err := h.repo.Update(sub, clearNotified); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update"})
	}

	return c.JSON(sub)
}

// Delete removes a subscription.
// DELETE /api/v1/subscriptions/:id
func (h *SubscriptionHandler) Delete(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid subscription id"})
	}

	if err := h.repo.Delete(id, user.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete"})
	}

	return c.JSON(fiber.Map{"deleted": true})
}

func defaultStr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
