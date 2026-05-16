//go:build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/handler"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/testhelper"
)

// injectUser is a test-only middleware that drops a fully-loaded
// model.User into c.Locals under the same "user" key the real
// AuthMiddleware uses. Decouples handler tests from initData / HMAC
// machinery — those paths are exercised separately in
// middleware/auth_integration_test.go, so this file can focus on the
// handler's own business logic.
func injectUser(u *model.User) fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Locals("user", u)
		return c.Next()
	}
}

// newSubApp wires SubscriptionHandler under a test middleware that
// injects the given user. Returns the app + db so tests can seed
// fixtures and assert on rows.
func newSubApp(t *testing.T, u *model.User) (*fiber.App, *gorm.DB) {
	t.Helper()
	db := testhelper.NewPostgres(t)

	// Persist the user so repo.GetByID(user.ID) lookups inside the
	// handler resolve. The handler reads user.ID off the context user,
	// not the DB row, so this is mostly for foreign-key consistency
	// when subscriptions get created.
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	h := handler.NewSubscriptionHandler(db)
	app := fiber.New()
	app.Use(injectUser(u))
	app.Post("/subscriptions", h.Create)
	app.Patch("/subscriptions/:id", h.Update)
	app.Delete("/subscriptions/:id", h.Delete)
	return app, db
}

// postJSON fires POST with a JSON body. Returns status + parsed body.
func postJSON(t *testing.T, app *fiber.App, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp.StatusCode, parsed
}

func patchJSON(t *testing.T, app *fiber.App, path string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test PATCH %s: %v", path, err)
	}
	defer resp.Body.Close()
	var parsed map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp.StatusCode, parsed
}

func deleteReq(t *testing.T, app *fiber.App, path string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test DELETE %s: %v", path, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func tomorrow() string {
	return time.Now().AddDate(0, 0, 1).Format("2006-01-02")
}

func TestSubscriptionCreate_HappyPath(t *testing.T) {
	user := &model.User{
		TelegramID:   1,
		FirstName:    "U",
		Locale:       "en",
		Timezone:     "UTC",
		IsDonator:    true, // skip paywall in default app_settings (disabled anyway)
	}
	app, db := newSubApp(t, user)

	status, body := postJSON(t, app, "/subscriptions", map[string]any{
		"name":            "Netflix",
		"amount":          12.99,
		"currency":        "USD",
		"period":          "monthly",
		"next_payment_at": tomorrow(),
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%v", status, body)
	}

	var count int64
	db.Model(&model.Subscription{}).Where("user_id = ?", user.ID).Count(&count)
	if count != 1 {
		t.Errorf("subscription count = %d, want 1", count)
	}
}

func TestSubscriptionCreate_LengthCap_Rejects(t *testing.T) {
	user := &model.User{TelegramID: 2, FirstName: "U", Timezone: "UTC", IsDonator: true}
	app, db := newSubApp(t, user)

	// Subscription.Name has gorm:"size:100". 101 'X' chars must reject
	// at the handler boundary, NOT silently truncate at the DB.
	status, body := postJSON(t, app, "/subscriptions", map[string]any{
		"name":            strings.Repeat("X", 101),
		"amount":          1.0,
		"period":          "monthly",
		"next_payment_at": tomorrow(),
	})
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%v", status, body)
	}
	var n int64
	db.Model(&model.Subscription{}).Count(&n)
	if n != 0 {
		t.Errorf("subscription created despite oversized name (count=%d)", n)
	}
}

// TestSubscriptionCreate_PaywallEnforcedConcurrently is the regression
// test for audit Tier-1 #4: two concurrent POST /subscriptions for the
// same free-tier user would both pass the `count < limit` SELECT and
// then both INSERT, overshooting the cap. The FOR UPDATE lock on the
// user row serializes the count-then-insert pair per user. Free-tier
// limit set to 2 here; we fire 5 concurrent requests and assert that
// the row count never exceeds 2.
func TestSubscriptionCreate_PaywallEnforcedConcurrently(t *testing.T) {
	user := &model.User{
		TelegramID: 3,
		FirstName:  "Free",
		Timezone:   "UTC",
		IsDonator:  false, // paywall applies
	}
	app, db := newSubApp(t, user)

	// Enable paywall with limit=2.
	settings := model.AppSettings{
		ID:             1,
		PaywallEnabled: true,
		FreeSubsLimit:  2,
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	const concurrentRequests = 5
	var wg sync.WaitGroup
	statuses := make([]int, concurrentRequests)
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := map[string]any{
				"name":            fmt.Sprintf("Sub %d", idx),
				"amount":          1.0,
				"period":          "monthly",
				"next_payment_at": tomorrow(),
			}
			status, _ := postJSON(t, app, "/subscriptions", body)
			statuses[idx] = status
		}(i)
	}
	wg.Wait()

	// Exactly 2 should have succeeded, the rest should have 403'd.
	var ok, forbidden int
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			ok++
		case http.StatusForbidden:
			forbidden++
		}
	}
	if ok != 2 {
		t.Errorf("201 responses = %d, want 2 (the free-tier cap)", ok)
	}
	if forbidden != concurrentRequests-2 {
		t.Errorf("403 responses = %d, want %d", forbidden, concurrentRequests-2)
	}

	var dbCount int64
	db.Model(&model.Subscription{}).Where("user_id = ?", user.ID).Count(&dbCount)
	if dbCount != 2 {
		t.Errorf("subscription rows = %d, want 2 (paywall overshot under concurrency)", dbCount)
	}
}

// TestSubscriptionUpdate_OtherUsersSub_404 covers the IDOR boundary:
// user B must not be able to mutate user A's subscription. The
// repository's GetByID(id, userID) does the ownership check, returning
// gorm.ErrRecordNotFound when the row exists but belongs to someone
// else — the handler maps that to 404 (not 403) on purpose, so the
// existence of the row leaks no information.
func TestSubscriptionUpdate_OtherUsersSub_404(t *testing.T) {
	owner := &model.User{TelegramID: 100, FirstName: "Owner", Timezone: "UTC", IsDonator: true}
	app, db := newSubApp(t, owner)

	// Seed a sub owned by a DIFFERENT user. We construct a stranger row
	// directly in DB — they don't need to log in for this test.
	stranger := model.User{TelegramID: 200, FirstName: "Stranger", IsDonator: true}
	if err := db.Create(&stranger).Error; err != nil {
		t.Fatalf("seed stranger: %v", err)
	}
	strangerSub := model.Subscription{
		ID:            uuid.New(),
		UserID:        stranger.ID,
		Name:          "stranger sub",
		Amount:        5,
		Currency:      "USD",
		Period:        "monthly",
		NextPaymentAt: time.Now().Add(48 * time.Hour),
	}
	if err := db.Create(&strangerSub).Error; err != nil {
		t.Fatalf("seed stranger sub: %v", err)
	}

	status, _ := patchJSON(t, app, "/subscriptions/"+strangerSub.ID.String(), map[string]any{
		"name": "PWNED",
	})
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (IDOR rejected as not-found)", status)
	}

	// The stranger's row must be unchanged.
	var reloaded model.Subscription
	if err := db.First(&reloaded, "id = ?", strangerSub.ID).Error; err != nil {
		t.Fatalf("reload stranger sub: %v", err)
	}
	if reloaded.Name != "stranger sub" {
		t.Errorf("Name = %q, want unchanged 'stranger sub'", reloaded.Name)
	}
}

func TestSubscriptionDelete_OtherUsersSub_404(t *testing.T) {
	owner := &model.User{TelegramID: 101, FirstName: "Owner", Timezone: "UTC", IsDonator: true}
	app, db := newSubApp(t, owner)

	stranger := model.User{TelegramID: 201, FirstName: "Stranger", IsDonator: true}
	db.Create(&stranger)
	strangerSub := model.Subscription{
		ID:            uuid.New(),
		UserID:        stranger.ID,
		Name:          "their sub",
		Amount:        5,
		Currency:      "USD",
		Period:        "monthly",
		NextPaymentAt: time.Now().Add(48 * time.Hour),
	}
	db.Create(&strangerSub)

	if status := deleteReq(t, app, "/subscriptions/"+strangerSub.ID.String()); status != http.StatusNotFound {
		t.Errorf("DELETE status = %d, want 404", status)
	}

	var count int64
	db.Model(&model.Subscription{}).Where("id = ?", strangerSub.ID).Count(&count)
	if count != 1 {
		t.Errorf("stranger's sub disappeared — IDOR delete succeeded")
	}
}

// TestSubscriptionUpdate_PeriodChange_ClearsNotifiedAt covers audit
// note A4: a period change must reset notified_at so the worker will
// re-remind the user at the new schedule. Without this, a yearly →
// monthly switch leaves an old "reminded" stamp lingering and the
// next renewal silently misses its notification.
func TestSubscriptionUpdate_PeriodChange_ClearsNotifiedAt(t *testing.T) {
	user := &model.User{TelegramID: 102, FirstName: "U", Timezone: "UTC", IsDonator: true}
	app, db := newSubApp(t, user)

	now := time.Now().UTC()
	sub := model.Subscription{
		ID:            uuid.New(),
		UserID:        user.ID,
		Name:          "Sub",
		Amount:        10,
		Currency:      "USD",
		Period:        "yearly",
		NextPaymentAt: now.Add(48 * time.Hour),
		NotifiedAt:    &now, // pretend the worker pinged this already
	}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatalf("seed sub: %v", err)
	}

	// PATCH period: yearly → monthly. Handler should clear notified_at.
	status, _ := patchJSON(t, app, "/subscriptions/"+sub.ID.String(), map[string]any{
		"period": "monthly",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	var reloaded model.Subscription
	if err := db.First(&reloaded, "id = ?", sub.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Period != "monthly" {
		t.Errorf("Period = %q, want monthly", reloaded.Period)
	}
	if reloaded.NotifiedAt != nil {
		t.Errorf("NotifiedAt = %v, want nil after period change", reloaded.NotifiedAt)
	}
}

// Quiet ioutil import to keep go vet/lint happy if the JSON helpers
// ever need raw body access in a future test.
var _ = io.ReadAll
