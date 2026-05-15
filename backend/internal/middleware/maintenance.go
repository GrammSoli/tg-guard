package middleware

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
)

// maintenanceCacheTTL bounds how stale the cached maintenance flag may
// be. The switch is flipped from the in-bot admin panel; a ~15s
// propagation delay is irrelevant for a maintenance window and saves a
// DB round-trip on every authenticated request.
const maintenanceCacheTTL = 15 * time.Second

// MaintenanceGuard returns middleware that answers 503 for every request
// while AppSettings.maintenance_mode is true.
//
// Two exceptions pass through:
//   - Admin API routes (path contains "/admin/") — always, so a misfire
//     can't lock the operator out.
//   - Requests whose initData carries an admin Telegram id — so admins
//     can keep using the WebApp during a window. Because this guard runs
//     BEFORE AuthMiddleware, it does a lightweight UNVALIDATED peek at
//     the id (see peekTelegramID); a forged admin id is harmless — the
//     request just reaches AuthMiddleware, which rejects it 401 on the
//     bad HMAC. The guard never grants access, it only declines to 503.
//
// The kill-switch itself is flipped from the Telegram bot (the /webhook
// route, not /api), which this guard never touches.
//
// The flag is read from a single-row table; we cache it for
// maintenanceCacheTTL so a burst of traffic doesn't hammer the DB. The
// cache fails OPEN: any DB error leaves the cached value unchanged (or
// false on first read), so a database hiccup never accidentally takes
// the whole API down.
func MaintenanceGuard(db *gorm.DB, cfg *config.Config) fiber.Handler {
	var (
		mu        sync.RWMutex
		cachedOn  bool
		fetchedAt time.Time
	)

	isOn := func() bool {
		mu.RLock()
		on, fresh := cachedOn, time.Since(fetchedAt) < maintenanceCacheTTL
		mu.RUnlock()
		if fresh {
			return on
		}
		var s model.AppSettings
		if err := db.Select("maintenance_mode").First(&s, 1).Error; err != nil {
			// Fail open — keep serving on a DB error rather than 503-ing
			// the whole API. Refresh the timestamp so we don't spin on a
			// down database every single request.
			mu.Lock()
			fetchedAt = time.Now()
			on = cachedOn
			mu.Unlock()
			return on
		}
		mu.Lock()
		cachedOn, fetchedAt = s.MaintenanceMode, time.Now()
		mu.Unlock()
		return s.MaintenanceMode
	}

	return func(c fiber.Ctx) error {
		// Admin routes bypass the gate unconditionally.
		if strings.Contains(c.Path(), "/admin/") {
			return c.Next()
		}
		if isOn() {
			// Admin bypass: peek (unvalidated) at the initData for an
			// admin Telegram id. Safe — see the func doc above.
			if tgID, ok := peekTelegramID(c.Get("X-Telegram-Init-Data")); ok && cfg.IsAdmin(tgID) {
				return c.Next()
			}
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "maintenance_mode",
				"message": "Ведутся технические работы",
			})
		}
		return c.Next()
	}
}

// peekTelegramID does a lightweight, UNVALIDATED extraction of the
// Telegram user id from a raw initData query string (the value of the
// X-Telegram-Init-Data header — this project doesn't use the
// "Authorization: tma …" form). It deliberately does NOT verify the
// HMAC signature: that's AuthMiddleware's job, and it runs right after
// this guard.
//
// The only caller is MaintenanceGuard's admin bypass. A forged admin id
// here merely lets the request reach AuthMiddleware, which then rejects
// it 401 on the invalid signature — so an unsigned peek grants nothing.
//
// Returns ok=false on a missing header, malformed query, missing/blank
// user field, bad JSON, or a zero id.
func peekTelegramID(initData string) (int64, bool) {
	if initData == "" {
		return 0, false
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, false
	}
	userJSON := values.Get("user")
	if userJSON == "" {
		return 0, false
	}
	var tgUser TelegramUser // reuse the struct from auth.go (same package)
	if err := json.Unmarshal([]byte(userJSON), &tgUser); err != nil || tgUser.ID == 0 {
		return 0, false
	}
	return tgUser.ID, true
}
