package handler

import (
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
