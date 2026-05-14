package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/repository"
)

// RecommendationsHandler serves sponsored offers to authenticated users.
type RecommendationsHandler struct {
	repo *repository.AdminRepo
}

func NewRecommendationsHandler(db *gorm.DB) *RecommendationsHandler {
	return &RecommendationsHandler{repo: repository.NewAdminRepo(db)}
}

// List returns active sponsored offers filtered by the requesting user's
// locale. If recommendations are globally disabled, returns an empty array.
func (h *RecommendationsHandler) List(c fiber.Ctx) error {
	settings, err := h.repo.GetSettings()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "settings lookup failed"})
	}
	if !settings.RecommendationsEnabled {
		return c.JSON([]any{})
	}

	lang := "en"
	if user := middleware.UserFromCtx(c); user != nil && user.Locale != "" {
		lang = user.Locale
	}

	offers, err := h.repo.ListActiveOffers(lang)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "offers lookup failed"})
	}
	return c.JSON(offers)
}

// TrackView increments the view counter for a batch of offer IDs.
// POST /api/v1/recommendations/track/view  { "ids": [1, 2, 3] }
func (h *RecommendationsHandler) TrackView(c fiber.Ctx) error {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.Bind().JSON(&body); err != nil || len(body.IDs) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "ids required"})
	}
	// Cap batch size to prevent abuse.
	if len(body.IDs) > 50 {
		body.IDs = body.IDs[:50]
	}
	h.repo.IncrementViews(body.IDs)
	return c.JSON(fiber.Map{"ok": true})
}

// TrackClick increments the click counter for a single offer.
// POST /api/v1/recommendations/:id/track/click
func (h *RecommendationsHandler) TrackClick(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	h.repo.IncrementClick(uint(id))
	return c.JSON(fiber.Map{"ok": true})
}
