//go:build integration

package middleware_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/testhelper"
)

const intgBotToken = "1234567890:INTEGRATION_TEST_TOKEN"

// newAuthApp wires AuthMiddleware in front of a tiny /me echo handler so
// each test asserts on the user record the middleware loaded into the
// Fiber context. Real Postgres via testcontainers.
func newAuthApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()
	db := testhelper.NewPostgres(t)

	app := fiber.New()
	app.Use(middleware.AuthMiddleware(intgBotToken, db))
	app.Get("/me", func(c fiber.Ctx) error {
		u := middleware.UserFromCtx(c)
		return c.JSON(u)
	})
	return app, db
}

func validInitData(tgID int64) string {
	userJSON := fmt.Sprintf(`{"id":%d,"first_name":"Test","last_name":"User","username":"alice","language_code":"en"}`, tgID)
	return testhelper.SignInitData(intgBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      userJSON,
	})
}

// doRequest fires GET /me with the given initData header and returns the
// status code and decoded body.
func doRequest(t *testing.T, app *fiber.App, initData string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	if initData != "" {
		req.Header.Set("X-Telegram-Init-Data", initData)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestAuthIntegration_MissingHeader_Returns401(t *testing.T) {
	app, _ := newAuthApp(t)
	status, body := doRequest(t, app, "")
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
	if !strings.Contains(fmt.Sprintf("%v", body["error"]), "missing") {
		t.Errorf("body.error = %v, want a 'missing' message", body["error"])
	}
}

func TestAuthIntegration_NewUser_InsertsRow(t *testing.T) {
	app, db := newAuthApp(t)
	status, body := doRequest(t, app, validInitData(11111))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	// telegram_id round-trips as a JSON number.
	if got, ok := body["telegram_id"].(float64); !ok || int64(got) != 11111 {
		t.Errorf("body.telegram_id = %v, want 11111", body["telegram_id"])
	}

	var stored model.User
	if err := db.Where("telegram_id = ?", 11111).First(&stored).Error; err != nil {
		t.Fatalf("user not persisted: %v", err)
	}
	if stored.Username != "alice" {
		t.Errorf("Username = %q, want alice", stored.Username)
	}
	// Defaults set by GORM should be present on a fresh insert.
	if stored.Timezone == "" {
		t.Error("Timezone empty — default UTC should have been applied")
	}
	if stored.NotificationTime == "" {
		t.Error("NotificationTime empty — default 10:00 should have been applied")
	}
}

// TestAuthIntegration_ExistingUser_PreservesSettings is the direct
// regression test for fd591ef. Before the upsert-with-RETURNING fix,
// the second /me hit returned a struct populated only from the INSERT
// values, so saved fields (premium_expires_at, timezone, base_currency,
// notification_*) came back as Go zero-values — handler/user.GetMe
// then served a paying user as free with the saved settings dropped.
func TestAuthIntegration_ExistingUser_PreservesSettings(t *testing.T) {
	app, db := newAuthApp(t)

	// Seed: pretend this user logged in once already AND set non-default
	// preferences via PATCH /me.
	preexisting := model.User{
		TelegramID:           22222,
		FirstName:            "Old",
		Username:             "oldname",
		Timezone:             "Europe/Moscow",
		Locale:               "ru",
		BaseCurrency:         "EUR",
		NotificationTime:     "08:30",
		NotificationsEnabled: false,
		IsDonator:            true,
	}
	if err := db.Create(&preexisting).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// New initData arrives with refreshed profile fields (different
	// first_name, language_code).
	userJSON := `{"id":22222,"first_name":"NEW_FIRST","last_name":"NEW_LAST","username":"alice_renamed","language_code":"en"}`
	initData := testhelper.SignInitData(intgBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      userJSON,
	})

	status, body := doRequest(t, app, initData)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}

	// The middleware returned the FULL row (RETURNING *), so non-profile
	// preferences must survive untouched.
	if got, _ := body["timezone"].(string); got != "Europe/Moscow" {
		t.Errorf("timezone = %q, want Europe/Moscow (must not reset to default UTC)", got)
	}
	if got, _ := body["base_currency"].(string); got != "EUR" {
		t.Errorf("base_currency = %q, want EUR", got)
	}
	if got, _ := body["notification_time"].(string); got != "08:30" {
		t.Errorf("notification_time = %q, want 08:30", got)
	}
	if got, ok := body["notifications_enabled"].(bool); !ok || got {
		t.Errorf("notifications_enabled = %v, want false", body["notifications_enabled"])
	}
	if got, ok := body["is_donator"].(bool); !ok || !got {
		t.Errorf("is_donator = %v, want true (paid user must not be served as free)", body["is_donator"])
	}

	// Locale is INSERT-only by design — once set, /me should not flip it
	// back to the Telegram-client default. The user picked ru via PATCH,
	// and the incoming language_code=en must not override that.
	if got, _ := body["locale"].(string); got != "ru" {
		t.Errorf("locale = %q, want ru (must not revert to Telegram default)", got)
	}

	// Profile fields should be updated to the fresh values.
	if got, _ := body["first_name"].(string); got != "NEW_FIRST" {
		t.Errorf("first_name = %q, want NEW_FIRST", got)
	}
}

// TestAuthIntegration_EmptyProfileField_DoesNotClobber covers the
// COALESCE(NULLIF(...)) branch: Telegram occasionally drops last_name
// or photo_url for users with privacy locks, and an upsert that
// straight-overwrote with the empty string would wipe what we have on
// file. Defensive of audit comment in middleware/auth.go.
func TestAuthIntegration_EmptyProfileField_DoesNotClobber(t *testing.T) {
	app, db := newAuthApp(t)

	// First hit: full profile.
	full := testhelper.SignInitData(intgBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      `{"id":33333,"first_name":"Has","last_name":"Lastname","username":"u","photo_url":"https://example/p.jpg"}`,
	})
	if status, _ := doRequest(t, app, full); status != http.StatusOK {
		t.Fatalf("first hit status = %d", status)
	}

	// Second hit: same user, but Telegram now drops last_name and
	// photo_url. The middleware must keep the previously-stored values.
	partial := testhelper.SignInitData(intgBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      `{"id":33333,"first_name":"Has","username":"u"}`,
	})
	if status, _ := doRequest(t, app, partial); status != http.StatusOK {
		t.Fatalf("second hit status = %d", status)
	}

	var stored model.User
	if err := db.Where("telegram_id = ?", 33333).First(&stored).Error; err != nil {
		t.Fatalf("user lookup: %v", err)
	}
	if stored.LastName != "Lastname" {
		t.Errorf("LastName = %q, want Lastname (must not be clobbered by empty)", stored.LastName)
	}
	if stored.PhotoURL != "https://example/p.jpg" {
		t.Errorf("PhotoURL = %q, want the original URL", stored.PhotoURL)
	}
}

func TestAuthIntegration_BannedUser_Returns403(t *testing.T) {
	app, db := newAuthApp(t)

	// Pre-seed a banned user.
	if err := db.Create(&model.User{
		TelegramID: 44444,
		FirstName:  "Ban",
		Username:   "ban",
		IsBanned:   true,
	}).Error; err != nil {
		t.Fatalf("seed banned: %v", err)
	}

	status, body := doRequest(t, app, validInitData(44444))
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", status)
	}
	if !strings.Contains(fmt.Sprintf("%v", body["error"]), "banned") {
		t.Errorf("body.error = %v, want 'banned'", body["error"])
	}
}

// TestAuthIntegration_SoftDeletedUser_Revives covers the
// deleted_at = NULL DoUpdate branch: a soft-deleted account that
// re-authenticates must come back as a normal user, not stay deleted.
func TestAuthIntegration_SoftDeletedUser_Revives(t *testing.T) {
	app, db := newAuthApp(t)

	// Create then soft-delete.
	if err := db.Create(&model.User{
		TelegramID: 55555,
		FirstName:  "Was",
		Username:   "ghost",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Where("telegram_id = ?", 55555).Delete(&model.User{}).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	status, _ := doRequest(t, app, validInitData(55555))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (soft-deleted user must be revived)", status)
	}

	// Unscoped because the soft-delete filter would otherwise hide a row
	// where deleted_at hasn't been cleared.
	var stored model.User
	if err := db.Unscoped().Where("telegram_id = ?", 55555).First(&stored).Error; err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if stored.DeletedAt.Valid {
		t.Errorf("DeletedAt still set after re-auth — revive branch broken")
	}
}

func TestAuthIntegration_InvalidSignature_Returns401(t *testing.T) {
	app, _ := newAuthApp(t)

	// Sign with a DIFFERENT token — middleware must reject.
	bad := testhelper.SignInitData("DIFFERENT_TOKEN", map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      `{"id":66666,"first_name":"X"}`,
	})
	status, _ := doRequest(t, app, bad)
	if status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", status)
	}
}
