package handler

import (
	"fmt"
	"log"
	"regexp"
	"time"

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

// ExportMe returns a GDPR-style data dump for the authenticated user as a
// JSON attachment: profile + personal subscriptions + every room they touch
// (with their role per room). The frontend wraps this in a Blob and forces a
// download.
// GET /api/v1/me/export
func (h *UserHandler) ExportMe(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "db unavailable"})
	}

	var subs []model.Subscription
	if err := h.db.Where("user_id = ?", user.ID).Find(&subs).Error; err != nil {
		log.Printf("[user.ExportMe] user=%d sub list error: %v", user.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "export failed: " + err.Error()})
	}

	// Rooms — both owned and joined. Preload services + members so the dump
	// is self-contained; reviewer of the export shouldn't need a second
	// query against another table.
	var rooms []model.SharedRoom
	if err := h.db.
		Preload("Services").
		Preload("Members").
		Joins("JOIN room_members ON room_members.room_id = shared_rooms.id").
		Where("room_members.user_id = ? OR shared_rooms.owner_id = ?", user.ID, user.ID).
		Group("shared_rooms.id").
		Find(&rooms).Error; err != nil {
		log.Printf("[user.ExportMe] user=%d room list error: %v", user.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "export failed: " + err.Error()})
	}

	roomDumps := make([]fiber.Map, 0, len(rooms))
	for _, r := range rooms {
		role := "member"
		if r.OwnerID == user.ID {
			role = "owner"
		}
		roomDumps = append(roomDumps, fiber.Map{
			"id":          r.ID,
			"name":        r.Name,
			"role":        role,
			"currency":    r.Currency,
			"billing_day": r.BillingDay,
			"invite_code": r.InviteCode,
			"created_at":  r.CreatedAt,
			"services":    r.Services,
			"members":     r.Members,
		})
	}

	dump := fiber.Map{
		"generated_at": time.Now().UTC(),
		"profile": fiber.Map{
			"id":                    user.ID,
			"telegram_id":           user.TelegramID,
			"first_name":            user.FirstName,
			"last_name":             user.LastName,
			"username":              user.Username,
			"locale":                user.Locale,
			"timezone":              user.Timezone,
			"base_currency":         user.BaseCurrency,
			"is_donator":            user.IsDonator,
			"notifications_enabled": user.NotificationsEnabled,
			"notification_time":     user.NotificationTime,
			"created_at":            user.CreatedAt,
		},
		"subscriptions": subs,
		"rooms":         roomDumps,
	}

	filename := fmt.Sprintf(`attachment; filename="subguard-export-%d.json"`, user.TelegramID)
	c.Set(fiber.HeaderContentDisposition, filename)
	return c.Status(fiber.StatusOK).JSON(dump)
}

// DeleteMe hard-deletes the authenticated user and every owned artifact:
//   - subscriptions
//   - shared rooms they OWN (cascades to room_services + room_members of
//     those rooms via the existing ForeignKey constraints on the model)
//   - their own room_members rows in rooms they merely JOINED
//   - the user row itself
//
// Wrapped in a single transaction so partial deletes never leave the system
// in a broken state.
// DELETE /api/v1/me
func (h *UserHandler) DeleteMe(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.db == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "db unavailable"})
	}

	uid := user.ID
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// 1) personal subscriptions
		if err := tx.Where("user_id = ?", uid).Delete(&model.Subscription{}).Error; err != nil {
			return fmt.Errorf("delete subscriptions: %w", err)
		}

		// 2) rooms the user OWNS — fetch ids, then nuke their services,
		//    members, and the rooms themselves.
		var ownedRoomIDs []string
		if err := tx.Model(&model.SharedRoom{}).
			Where("owner_id = ?", uid).
			Pluck("id::text", &ownedRoomIDs).Error; err != nil {
			return fmt.Errorf("list owned rooms: %w", err)
		}
		if len(ownedRoomIDs) > 0 {
			if err := tx.Where("room_id IN ?", ownedRoomIDs).Delete(&model.RoomService{}).Error; err != nil {
				return fmt.Errorf("delete owned room services: %w", err)
			}
			if err := tx.Where("room_id IN ?", ownedRoomIDs).Delete(&model.RoomMember{}).Error; err != nil {
				return fmt.Errorf("delete owned room members: %w", err)
			}
			if err := tx.Where("id IN ?", ownedRoomIDs).Delete(&model.SharedRoom{}).Error; err != nil {
				return fmt.Errorf("delete owned rooms: %w", err)
			}
		}

		// 3) memberships in other rooms (where user is a member but not the
		//    owner). The owned-room members were already wiped above; this
		//    cleans up the rest.
		if err := tx.Where("user_id = ?", uid).Delete(&model.RoomMember{}).Error; err != nil {
			return fmt.Errorf("delete memberships: %w", err)
		}

		// 4) donations log — keep audit trail OR drop. We DROP because the
		//    user is exercising their right to be forgotten.
		if err := tx.Where("user_id = ?", uid).Delete(&model.Donation{}).Error; err != nil {
			return fmt.Errorf("delete donations: %w", err)
		}

		// 5) the user row itself
		if err := tx.Delete(&model.User{}, uid).Error; err != nil {
			return fmt.Errorf("delete user: %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("[user.DeleteMe] user=%d delete error: %v", uid, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "delete failed: " + err.Error(),
		})
	}

	log.Printf("[user.DeleteMe] user=%d deleted (telegram_id=%d)", uid, user.TelegramID)
	return c.JSON(fiber.Map{"deleted": true})
}
