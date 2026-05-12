package handler

import (
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
)

// UserHandler handles user profile endpoints.
type UserHandler struct {
	cfg *config.Config
	db  *gorm.DB
}

func NewUserHandler(cfg *config.Config, db ...* gorm.DB) *UserHandler {
	h := &UserHandler{cfg: cfg}
	if len(db) > 0 {
		h.db = db[0]
	}
	return h
}

// GetMe returns the authenticated user's profile.
// GET /api/v1/me
func (h *UserHandler) GetMe(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	return c.JSON(fiber.Map{
		"id":                    user.ID,
		"telegram_id":           user.TelegramID,
		"first_name":            user.FirstName,
		"last_name":             user.LastName,
		"username":              user.Username,
		"photo_url":             user.PhotoURL,
		"locale":                user.Locale,
		"timezone":              user.Timezone,
		"base_currency":         user.BaseCurrency,
		"is_donator":            user.IsDonator,
		"is_admin":              h.cfg.IsAdmin(user.TelegramID),
		"notifications_enabled": user.NotificationsEnabled,
	})
}

// UpdateMe patches user settings.
// PATCH /api/v1/me
func (h *UserHandler) UpdateMe(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var body struct {
		Locale               *string `json:"locale"`
		Timezone             *string `json:"timezone"`
		BaseCurrency         *string `json:"base_currency"`
		NotificationsEnabled *bool   `json:"notifications_enabled"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	updates := map[string]interface{}{}
	if body.Locale != nil {
		updates["locale"] = *body.Locale
	}
	if body.Timezone != nil {
		updates["timezone"] = *body.Timezone
	}
	if body.BaseCurrency != nil {
		updates["base_currency"] = *body.BaseCurrency
	}
	if body.NotificationsEnabled != nil {
		updates["notifications_enabled"] = *body.NotificationsEnabled
	}

	if len(updates) > 0 && h.db != nil {
		if err := h.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "update failed"})
		}
	}

	return c.JSON(fiber.Map{"status": "ok"})
}
