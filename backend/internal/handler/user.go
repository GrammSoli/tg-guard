package handler

import (
	"log"
	"regexp"

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

func NewUserHandler(cfg *config.Config, db ...*gorm.DB) *UserHandler {
	h := &UserHandler{cfg: cfg}
	if len(db) > 0 {
		h.db = db[0]
	}
	return h
}

// notificationTimePattern enforces an "HH:MM" 24h format on the
// notification_time field so we never push junk into worker parsing.
var notificationTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)$`)

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
		"notification_time":     user.NotificationTime,
	})
}

// UpdateMe patches user settings.
// PATCH /api/v1/me
//
// Pointer-typed fields let us tell "field was omitted from the JSON body"
// from "field was explicitly set to its zero value" (e.g. false / ""). We
// build a map[string]interface{} of just the present fields and hand it to
// GORM's .Updates(); that path writes zero-values too, unlike a struct-based
// .Updates() which would silently drop notifications_enabled=false.
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
		NotificationTime     *string `json:"notification_time"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}

	updates := map[string]interface{}{}
	if body.Locale != nil {
		updates["locale"] = *body.Locale
	}
	if body.Timezone != nil {
		// Trust the IANA tz name the client (Intl API) supplies. Length cap
		// matches the column size; further validation lives in the worker
		// (LoadLocation falls back to UTC on bad input).
		if len(*body.Timezone) > 64 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "timezone too long"})
		}
		updates["timezone"] = *body.Timezone
	}
	if body.BaseCurrency != nil {
		updates["base_currency"] = *body.BaseCurrency
	}
	if body.NotificationsEnabled != nil {
		updates["notifications_enabled"] = *body.NotificationsEnabled
	}
	if body.NotificationTime != nil {
		if !notificationTimePattern.MatchString(*body.NotificationTime) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "notification_time must be HH:MM (00:00-23:59)",
			})
		}
		updates["notification_time"] = *body.NotificationTime
	}

	if len(updates) > 0 && h.db != nil {
		if err := h.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			// Log the underlying DB error so missing columns / type errors
			// are visible in logs instead of being masked by a generic 500.
			log.Printf("[user.UpdateMe] user=%d update error: %v", user.ID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "update failed: " + err.Error(),
			})
		}
	}

	return c.JSON(fiber.Map{"status": "ok"})
}
