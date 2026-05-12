package handler

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
)

// SubscriptionHandler handles subscription CRUD.
type SubscriptionHandler struct {
	repo *repository.SubscriptionRepo
	db   *gorm.DB
}

func NewSubscriptionHandler(db *gorm.DB) *SubscriptionHandler {
	return &SubscriptionHandler{
		repo: repository.NewSubscriptionRepo(db),
		db:   db,
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
		Name          string     `json:"name"`
		Brand         string     `json:"brand"`
		Tag           string     `json:"tag"`
		Amount        float64    `json:"amount"`
		Currency      string     `json:"currency"`
		Period        string     `json:"period"`
		NextPaymentAt string     `json:"next_payment_at"`
		IsTrial       bool       `json:"is_trial"`
		TrialEndsAt   *string    `json:"trial_ends_at"`
		IsAutoPay     bool       `json:"is_auto_pay"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	if body.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	nextPayment, err := time.Parse(time.RFC3339, body.NextPaymentAt)
	if err != nil {
		nextPayment, err = time.Parse("2006-01-02", body.NextPaymentAt)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid next_payment_at date"})
		}
	}

	sub := model.Subscription{
		UserID:        user.ID,
		Name:          body.Name,
		Brand:         defaultStr(body.Brand, "default"),
		Tag:           body.Tag,
		Amount:        body.Amount,
		Currency:      defaultStr(body.Currency, "USD"),
		Period:        defaultStr(body.Period, "monthly"),
		NextPaymentAt: nextPayment,
		IsTrial:       body.IsTrial,
		IsAutoPay:     body.IsAutoPay,
	}

	if body.IsTrial && body.TrialEndsAt != nil {
		t, err := time.Parse(time.RFC3339, *body.TrialEndsAt)
		if err != nil {
			t, _ = time.Parse("2006-01-02", *body.TrialEndsAt)
		}
		sub.TrialEndsAt = &t
	}

	if err := h.repo.Create(&sub); err != nil {
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

	var body map[string]interface{}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	// Apply allowed updates
	if v, ok := body["name"].(string); ok {
		sub.Name = v
	}
	if v, ok := body["brand"].(string); ok {
		sub.Brand = v
	}
	if v, ok := body["tag"].(string); ok {
		sub.Tag = v
	}
	if v, ok := body["amount"].(float64); ok {
		sub.Amount = v
	}
	if v, ok := body["currency"].(string); ok {
		sub.Currency = v
	}
	if v, ok := body["period"].(string); ok {
		sub.Period = v
	}
	if v, ok := body["is_trial"].(bool); ok {
		sub.IsTrial = v
	}
	if v, ok := body["is_auto_pay"].(bool); ok {
		sub.IsAutoPay = v
	}
	if v, ok := body["next_payment_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sub.NextPaymentAt = t
		}
	}

	if err := h.repo.Update(sub); err != nil {
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
