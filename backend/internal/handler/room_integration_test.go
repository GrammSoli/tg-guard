//go:build integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/handler"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/testhelper"
)

// newRoomApp wires RoomHandler under a test middleware that injects
// the given user. Notifier is nil — Remind / DM-sending paths are not
// exercised in this file.
func newRoomApp(t *testing.T, u *model.User) (*fiber.App, *gorm.DB) {
	t.Helper()
	db := testhelper.NewPostgres(t)
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	cfg := &config.Config{}
	h := handler.NewRoomHandler(db, cfg, nil)
	app := fiber.New()
	app.Use(injectUser(u))
	app.Post("/rooms", h.Create)
	app.Get("/rooms/:id", h.GetDetail)
	app.Patch("/rooms/:id", h.UpdateRoom)
	app.Delete("/rooms/:id", h.DeleteRoom)
	app.Post("/rooms/:id/services", h.AddService)
	app.Post("/rooms/join/:invite", h.Join)
	return app, db
}

func getReq(t *testing.T, app *fiber.App, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("app.Test GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func postRoom(t *testing.T, app *fiber.App, path string, body any) (int, map[string]any) {
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

// TestRoomCreate_CopiesOwnerTimezone confirms the Phase 1 contract:
// SharedRoom.Timezone snapshots from User.Timezone at creation time,
// not the worker-default "UTC". This is the data point the billing-
// reset worker uses to decide WHEN this room's local midnight is.
func TestRoomCreate_CopiesOwnerTimezone(t *testing.T) {
	owner := &model.User{
		TelegramID: 10,
		FirstName:  "Owner",
		Timezone:   "Europe/Moscow",
		IsDonator:  true,
	}
	app, db := newRoomApp(t, owner)

	status, body := postRoom(t, app, "/rooms", map[string]any{
		"name":     "Family Netflix",
		"currency": "USD",
	})
	// Room.Create returns 201 Created — sub.Create does too; tests pin
	// the actual handler contract.
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("status = %d, want 200 or 201; body=%v", status, body)
	}

	var room model.SharedRoom
	if err := db.Where("owner_id = ?", owner.ID).First(&room).Error; err != nil {
		t.Fatalf("reload room: %v", err)
	}
	if room.Timezone != "Europe/Moscow" {
		t.Errorf("Timezone = %q, want Europe/Moscow (owner's TZ)", room.Timezone)
	}
}

// TestRoomCreate_LengthCap_Rejects covers the Phase 1 input-length
// guard: handler must reject before PG silently truncates the 50-char
// Name column.
func TestRoomCreate_LengthCap_Rejects(t *testing.T) {
	owner := &model.User{TelegramID: 11, FirstName: "U", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, owner)

	status, _ := postRoom(t, app, "/rooms", map[string]any{
		"name": strings.Repeat("R", 51),
	})
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	var n int64
	db.Model(&model.SharedRoom{}).Count(&n)
	if n != 0 {
		t.Errorf("room created despite oversized name (count=%d)", n)
	}
}

// TestRoomGetDetail_NonMember_403 covers the IDOR boundary: a user
// who is neither owner nor member of a room must not be able to read
// its details. Compare with subscriptions which return 404 for the
// same scenario — rooms have a public invite-code surface (the join
// link), so existence isn't sensitive in the same way; 403 is
// correct here.
func TestRoomGetDetail_NonMember_403(t *testing.T) {
	caller := &model.User{TelegramID: 12, FirstName: "Caller", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, caller)

	// Seed a room owned by someone else.
	otherOwner := model.User{TelegramID: 13, FirstName: "Other", IsDonator: true}
	if err := db.Create(&otherOwner).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	room := model.SharedRoom{
		ID:         uuid.New(),
		Name:       "Private Room",
		OwnerID:    otherOwner.ID,
		InviteCode: "private01",
		Currency:   "USD",
		BillingDay: 1,
		Timezone:   "UTC",
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatalf("seed room: %v", err)
	}

	status, body := getReq(t, app, "/rooms/"+room.ID.String())
	if status != http.StatusForbidden {
		t.Errorf("status = %d, want 403; body=%v", status, body)
	}
}

func TestRoomUpdate_NonOwner_403(t *testing.T) {
	caller := &model.User{TelegramID: 14, FirstName: "Caller", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, caller)

	otherOwner := model.User{TelegramID: 15, FirstName: "Other", IsDonator: true}
	db.Create(&otherOwner)
	room := model.SharedRoom{
		ID:         uuid.New(),
		Name:       "Theirs",
		OwnerID:    otherOwner.ID,
		InviteCode: "theirs01",
		Currency:   "USD",
		BillingDay: 1,
		Timezone:   "UTC",
	}
	db.Create(&room)

	// Even though billing_day=15 is itself valid, the request must be
	// rejected before reaching the repo Update because of ownership.
	raw, _ := json.Marshal(map[string]any{"billing_day": 15})
	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+room.ID.String(), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}

	// And the stored row must be unchanged.
	var reloaded model.SharedRoom
	db.First(&reloaded, "id = ?", room.ID)
	if reloaded.BillingDay != 1 {
		t.Errorf("BillingDay = %d, want unchanged 1 (IDOR PATCH succeeded)", reloaded.BillingDay)
	}
}

func TestRoomAddService_LengthCap_Rejects(t *testing.T) {
	owner := &model.User{TelegramID: 16, FirstName: "Owner", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, owner)

	room := model.SharedRoom{
		ID:         uuid.New(),
		Name:       "R",
		OwnerID:    owner.ID,
		InviteCode: "addsvc01",
		Currency:   "USD",
		BillingDay: 1,
		Timezone:   "UTC",
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatalf("seed room: %v", err)
	}

	// RoomService.Note has gorm:"size:128". 129-char note must reject.
	status, _ := postRoom(t, app, "/rooms/"+room.ID.String()+"/services", map[string]any{
		"brand":    "spotify",
		"name":     "Spotify",
		"amount":   9.99,
		"currency": "USD",
		"note":     strings.Repeat("n", 129),
	})
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	var n int64
	db.Model(&model.RoomService{}).Count(&n)
	if n != 0 {
		t.Errorf("service created with oversized note (count=%d)", n)
	}
}

// TestRoomJoin_Idempotent covers audit Tier-1 #5: the previous
// "IsMember? AddMember" pair had a TOCTOU window where two concurrent
// Join calls (same invite link tapped twice from a flaky network)
// both passed the EXISTS check and then raced on the composite PK.
// The current ON CONFLICT DO NOTHING path makes a repeat Join a
// silent no-op. We test serially here — the race is hard to
// reproduce reliably with Fiber's in-process Test, but the ON
// CONFLICT semantics are the actual guarantee.
func TestRoomJoin_Idempotent(t *testing.T) {
	joiner := &model.User{TelegramID: 17, FirstName: "Joiner", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, joiner)

	otherOwner := model.User{TelegramID: 18, FirstName: "Other", IsDonator: true}
	db.Create(&otherOwner)
	room := model.SharedRoom{
		ID:         uuid.New(),
		Name:       "Joinable",
		OwnerID:    otherOwner.ID,
		InviteCode: "joincode01",
		Currency:   "USD",
		BillingDay: 1,
		Timezone:   "UTC",
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatalf("seed room: %v", err)
	}

	// First Join: 200 + member row.
	if status, _ := postRoom(t, app, "/rooms/join/"+room.InviteCode, nil); status != http.StatusOK {
		t.Fatalf("first Join status = %d, want 200", status)
	}

	// Second Join with the same user must also 200 (idempotent) and
	// produce no duplicate member row.
	if status, _ := postRoom(t, app, "/rooms/join/"+room.InviteCode, nil); status != http.StatusOK {
		t.Fatalf("second Join status = %d, want 200 (idempotent)", status)
	}

	var count int64
	db.Model(&model.RoomMember{}).Where("room_id = ? AND user_id = ?", room.ID, joiner.ID).Count(&count)
	if count != 1 {
		t.Errorf("member rows = %d, want 1 (Join should be idempotent)", count)
	}
}

func TestRoomDelete_NonOwner_403(t *testing.T) {
	caller := &model.User{TelegramID: 19, FirstName: "Caller", Timezone: "UTC", IsDonator: true}
	app, db := newRoomApp(t, caller)

	otherOwner := model.User{TelegramID: 20, FirstName: "Other", IsDonator: true}
	db.Create(&otherOwner)
	room := model.SharedRoom{
		ID:         uuid.New(),
		Name:       "Theirs",
		OwnerID:    otherOwner.ID,
		InviteCode: "delcode01",
		Currency:   "USD",
		BillingDay: 1,
		Timezone:   "UTC",
	}
	db.Create(&room)

	if status := deleteReq(t, app, "/rooms/"+room.ID.String()); status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", status)
	}
	var count int64
	db.Model(&model.SharedRoom{}).Where("id = ?", room.ID).Count(&count)
	if count != 1 {
		t.Errorf("room deleted by non-owner — IDOR DELETE succeeded")
	}
}
