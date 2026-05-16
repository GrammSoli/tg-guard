package handler

import (
	"log"
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

// callerLocale returns the authenticated user's locale, defaulting to
// "en" when the user has no locale set yet. Used by Track* handlers to
// scope counter increments to offers the caller could legitimately see.
func callerLocale(c fiber.Ctx) string {
	if user := middleware.UserFromCtx(c); user != nil && user.Locale != "" {
		return user.Locale
	}
	return "en"
}

// TrackView increments the view counter for a batch of offer IDs.
// POST /api/v1/recommendations/track/view  { "ids": [1, 2, 3] }
//
// The counter UPDATE is filtered by the caller's locale + is_active
// inside the repo so an EN user firing TrackView with RU offer IDs
// (which they'd never see in List anyway) inflates nothing. Audit
// Tier-1 #7.
func (h *RecommendationsHandler) TrackView(c fiber.Ctx) error {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.Bind().JSON(&body); err != nil || len(body.IDs) == 0 {
		log.Printf("[track/view] bad payload: err=%v ids=%v", err, body.IDs)
		return c.Status(400).JSON(fiber.Map{"error": "ids required"})
	}
	// Cap batch size to prevent abuse.
	if len(body.IDs) > 50 {
		body.IDs = body.IDs[:50]
	}
	log.Printf("[track/view] incrementing views for IDs: %v", body.IDs)
	h.repo.IncrementViews(body.IDs, callerLocale(c))
	return c.JSON(fiber.Map{"ok": true})
}

// TrackClick increments the click counter for a single offer. Same
// locale + is_active guard as TrackView.
// POST /api/v1/recommendations/:id/track/click
func (h *RecommendationsHandler) TrackClick(c fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	log.Printf("[track/click] incrementing click for ID: %d", id)
	h.repo.IncrementClick(uint(id), callerLocale(c))
	return c.JSON(fiber.Map{"ok": true})
}
