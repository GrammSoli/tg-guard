package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
	"github.com/subguard/backend/internal/repository"
)

type RoomHandler struct {
	repo     *repository.RoomRepo
	notifier notifier.Notifier
}

func NewRoomHandler(db *gorm.DB, n notifier.Notifier) *RoomHandler {
	return &RoomHandler{repo: repository.NewRoomRepo(db), notifier: n}
}

func (h *RoomHandler) List(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	rooms, err := h.repo.ListByUser(user.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "fetch failed"})
	}
	out := make([]fiber.Map, len(rooms))
	for i, r := range rooms {
		out[i] = roomSummary(&r)
	}
	return c.JSON(out)
}

func (h *RoomHandler) GetDetail(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad id"})
	}
	room, err := h.repo.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID && !h.repo.IsMember(room.ID, user.ID) {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	return c.JSON(buildRoomDetailResponse(room, user.ID))
}

// buildRoomDetailResponse formats an already-loaded SharedRoom (with
// Services + Members.User preloaded) into the GetDetail JSON shape.
// Extracted so mutation handlers can return fresh state without
// re-running the auth path they just passed (audit O5). The owner-only
// mutations call h.repo.GetByID once after the write and feed the result
// here — saves an IsMember EXISTS query per mutation, plus the redundant
// ownership check.
func buildRoomDetailResponse(room *model.SharedRoom, callerID uint) fiber.Map {
	// Members.User was preloaded in a single JOIN by GetByID (audit A3).
	// Project the fresh first_name/username/photo_url onto the flattened
	// RoomMember fields so the JSON response keeps the same shape the
	// frontend expects. The cached Name/Username/Avatar are kept as
	// fallback when the joined User is nil (soft-deleted user still
	// listed as member).
	for i := range room.Members {
		if u := room.Members[i].User; u != nil {
			room.Members[i].Name = u.FirstName
			room.Members[i].Username = u.Username
			room.Members[i].Avatar = u.PhotoURL
		}
	}
	m := roomSummary(room)
	m["owner_id"] = room.OwnerID
	m["invite_code"] = room.InviteCode
	m["services"] = room.Services
	m["members"] = room.Members
	m["is_owner"] = room.OwnerID == callerID
	m["billing_day"] = room.BillingDay
	m["created_at"] = room.CreatedAt
	m["last_reminded_at"] = room.LastRemindedAt
	return m
}

// refreshAndReturn re-reads the room and returns its detail JSON. Used
// by owner-only mutations that already passed the ownership check and
// must return updated state; skips the IsMember EXISTS roundtrip the
// full GetDetail does.
func (h *RoomHandler) refreshAndReturn(c fiber.Ctx, roomID uuid.UUID, callerID uint) error {
	fresh, err := h.repo.GetByID(roomID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(buildRoomDetailResponse(fresh, callerID))
}

func (h *RoomHandler) Create(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	var body struct {
		Name     string `json:"name"`
		Currency string `json:"currency"`
		Services []struct {
			Brand     string  `json:"brand"`
			Name      string  `json:"name"`
			Amount    float64 `json:"amount"`
			Currency  string  `json:"currency"`
			Note      string  `json:"note"`
			IconName  string  `json:"icon_name"`
			IconColor string  `json:"icon_color"`
		} `json:"services"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name required"})
	}
	now := time.Now()
	room := model.SharedRoom{
		Name: body.Name, OwnerID: user.ID,
		Currency:   defaultStr(body.Currency, "USD"),
		BillingDay: now.Day(),
		Members:    []model.RoomMember{{UserID: user.ID, Name: user.FirstName, Username: user.Username, Avatar: user.PhotoURL, HasPaid: true}},
	}
	for _, s := range body.Services {
		room.Services = append(room.Services, model.RoomService{
			Brand: s.Brand, Name: s.Name, Amount: s.Amount, Currency: s.Currency,
			Note: s.Note, IconName: s.IconName, IconColor: s.IconColor,
		})
	}
	if err := h.repo.Create(&room); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "create failed"})
	}
	return c.Status(201).JSON(roomSummary(&room))
}

func (h *RoomHandler) Join(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	room, err := h.repo.GetByInviteCode(c.Params("invite"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "bad invite"})
	}
	if !h.repo.IsMember(room.ID, user.ID) {
		h.repo.AddMember(&model.RoomMember{RoomID: room.ID, UserID: user.ID, Name: user.FirstName, Username: user.Username, Avatar: user.PhotoURL})
	}
	return c.JSON(roomSummary(room))
}

// Remind sends a real Telegram message to all unpaid members.
// Anti-spam: 24-hour cooldown per room. Only the room owner can trigger.
func (h *RoomHandler) Remind(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad id"})
	}
	room, err := h.repo.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}

	// Cooldown check: 24 hours
	cooldownHours := 24
	if room.LastRemindedAt != nil {
		nextAllowed := room.LastRemindedAt.Add(time.Duration(cooldownHours) * time.Hour)
		if time.Now().Before(nextAllowed) {
			return c.Status(429).JSON(fiber.Map{
				"error":          "cooldown",
				"cooldown_until": nextAllowed,
			})
		}
	}

	unpaid, err := h.repo.GetUnpaidMembers(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "fetch unpaid failed"})
	}
	names := make([]string, len(unpaid))
	for i, m := range unpaid {
		names[i] = m.Name
	}

	tgIDs, err := h.repo.GetUnpaidMemberTelegramIDs(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "fetch telegram ids failed"})
	}
	sendCtx, cancel := context.WithTimeout(c.Context(), 15*time.Second)
	defer cancel()
	for _, tgID := range tgIDs {
		text := fmt.Sprintf("🔔 Напоминание: оплатите вашу долю в комнате «%s».", room.Name)
		if err := h.notifier.SendMessage(sendCtx, tgID, text); err != nil {
			log.Printf("[remind] failed to send to %d: %v", tgID, err)
		}
	}

	if err := h.repo.SetLastReminded(id); err != nil {
		log.Printf("[remind] set last_reminded_at failed: %v", err)
	}

	return c.JSON(fiber.Map{"reminded": len(unpaid), "members": names})
}

func (h *RoomHandler) MarkPaid(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad id"})
	}
	room, err := h.repo.GetByID(roomID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}
	targetUID, err := strconv.ParseUint(c.Params("uid"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad uid"})
	}
	if err := h.repo.MarkPaid(roomID, uint(targetUID)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "mark failed"})
	}
	return h.refreshAndReturn(c, roomID, user.ID)
}

func (h *RoomHandler) MarkUnpaid(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad id"})
	}
	room, err := h.repo.GetByID(roomID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}
	targetUID, err := strconv.ParseUint(c.Params("uid"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad uid"})
	}
	if err := h.repo.MarkUnpaid(roomID, uint(targetUID)); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "mark failed"})
	}
	return h.refreshAndReturn(c, roomID, user.ID)
}

func (h *RoomHandler) DeleteRoom(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	id, _ := uuid.Parse(c.Params("id"))
	room, err := h.repo.GetByID(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}
	if err := h.repo.Delete(id); err != nil {
		log.Printf("[room.DeleteRoom] room=%s delete error: %v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "delete failed"})
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// UpdateRoom allows owner to update room settings (e.g. billing_day).
func (h *RoomHandler) UpdateRoom(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad id"})
	}
	room, err := h.repo.GetByID(roomID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}
	var body struct {
		BillingDay *int `json:"billing_day"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad body"})
	}
	if body.BillingDay != nil {
		day := *body.BillingDay
		if day < 1 || day > 31 {
			return c.Status(400).JSON(fiber.Map{"error": "billing_day must be 1-31"})
		}
		h.repo.UpdateBillingDay(roomID, day)
	}
	return h.refreshAndReturn(c, roomID, user.ID)
}

// RemoveMember allows the room owner to kick a member (not themselves).
func (h *RoomHandler) RemoveMember(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad room id"})
	}
	room, err := h.repo.GetByID(roomID)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "room not found"})
	}
	if room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "owner only"})
	}
	memberUID, err := strconv.ParseUint(c.Params("uid"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad user id"})
	}
	targetUID := uint(memberUID)
	if targetUID == user.ID {
		return c.Status(400).JSON(fiber.Map{"error": "cannot remove yourself"})
	}
	if err := h.repo.RemoveMember(roomID, targetUID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "remove failed"})
	}
	return h.refreshAndReturn(c, roomID, user.ID)
}

func (h *RoomHandler) AddService(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, _ := uuid.Parse(c.Params("id"))
	room, err := h.repo.GetByID(roomID)
	if err != nil || room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	var body struct {
		Brand     string  `json:"brand"`
		Name      string  `json:"name"`
		Amount    float64 `json:"amount"`
		Currency  string  `json:"currency"`
		Note      string  `json:"note"`
		IconName  string  `json:"icon_name"`
		IconColor string  `json:"icon_color"`
	}
	c.Bind().JSON(&body)
	h.repo.AddService(&model.RoomService{
		RoomID: roomID, Brand: body.Brand, Name: body.Name,
		Amount: body.Amount, Currency: body.Currency,
		Note: body.Note, IconName: body.IconName, IconColor: body.IconColor,
	})
	return c.Status(201).JSON(fiber.Map{"ok": true})
}

func (h *RoomHandler) RemoveService(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, _ := uuid.Parse(c.Params("id"))
	room, err := h.repo.GetByID(roomID)
	if err != nil || room.OwnerID != user.ID {
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
	sid, err := strconv.ParseUint(c.Params("sid"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid service id"})
	}
	h.repo.RemoveService(roomID, uint(sid))
	return c.JSON(fiber.Map{"ok": true})
}

func roomSummary(r *model.SharedRoom) fiber.Map {
	total := 0.0
	var services []fiber.Map
	for _, s := range r.Services {
		total += s.Amount
		services = append(services, fiber.Map{"brand": s.Brand})
	}
	if services == nil {
		services = make([]fiber.Map, 0)
	}
	pm := 0.0
	if len(r.Members) > 0 {
		pm = total / float64(len(r.Members))
	}
	return fiber.Map{"id": r.ID, "name": r.Name, "members": len(r.Members), "total_per_member": pm, "currency": r.Currency, "services": services}
}
