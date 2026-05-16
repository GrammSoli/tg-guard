package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
	"github.com/subguard/backend/internal/repository"
	"github.com/subguard/backend/internal/tgutil"
	"github.com/subguard/backend/internal/timezone"
)

type RoomHandler struct {
	repo      *repository.RoomRepo
	adminRepo *repository.AdminRepo
	notifier  notifier.Notifier
	cfg       *config.Config
}

func NewRoomHandler(db *gorm.DB, cfg *config.Config, n notifier.Notifier) *RoomHandler {
	return &RoomHandler{
		repo:      repository.NewRoomRepo(db),
		adminRepo: repository.NewAdminRepo(db),
		notifier:  n,
		cfg:       cfg,
	}
}

// roomDeepLink builds the Mini App URL that opens directly on a room. The
// frontend reads the `room` query param on load and opens that room.
func roomDeepLink(baseURL string, roomID uuid.UUID) string {
	return fmt.Sprintf("%s/?room=%s", strings.TrimRight(baseURL, "/"), roomID)
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
	m["timezone"] = room.Timezone
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
	ownerTz := user.Timezone
	if ownerTz == "" {
		ownerTz = "UTC"
	}
	// BillingDay is the day-of-month in the OWNER'S timezone, not UTC.
	// A Sydney user creating a room at 09:00 local on the 1st would
	// otherwise get billing_day=31 (still the 31st in UTC) and find
	// their reset firing on the wrong day every month.
	localNow := time.Now().In(timezone.LoadOrUTC(ownerTz))
	room := model.SharedRoom{
		Name: body.Name, OwnerID: user.ID,
		Currency:   defaultStr(body.Currency, "USD"),
		BillingDay: localNow.Day(),
		Timezone:   ownerTz,
		Members:    []model.RoomMember{{UserID: user.ID, Name: user.FirstName, Username: user.Username, Avatar: user.PhotoURL, HasPaid: true}},
	}
	for _, s := range body.Services {
		room.Services = append(room.Services, model.RoomService{
			Brand: s.Brand, Name: s.Name, Amount: s.Amount, Currency: s.Currency,
			Note: s.Note, IconName: s.IconName, IconColor: s.IconColor,
		})
	}

	// Paywall + Create wrapped in a tx with SELECT … FOR UPDATE on the
	// user (owner) row. Without the lock two concurrent POST /rooms for
	// the same user could BOTH pass `count < limit` and BOTH insert,
	// overshooting the free-tier cap. Mirrors handler/subscription.go's
	// fix for the same race. Audit Tier-1 #4.
	var paywallCount, paywallLimit int64
	txErr := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		var locked model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", user.ID).First(&locked).Error; err != nil {
			return fmt.Errorf("lock user: %w", err)
		}
		if !locked.IsDonator {
			settings, sErr := h.adminRepo.GetSettings()
			if sErr == nil && settings.PaywallEnabled {
				var count int64
				if err := tx.Model(&model.SharedRoom{}).
					Where("owner_id = ?", user.ID).
					Count(&count).Error; err != nil {
					return fmt.Errorf("count rooms: %w", err)
				}
				if count >= int64(settings.FreeRoomLimit) {
					paywallCount = count
					paywallLimit = int64(settings.FreeRoomLimit)
					return errPaywallLimit
				}
			}
		}
		// Generate the invite code + insert via tx (mirrors RoomRepo.Create
		// but inside the locked transaction so other concurrent invite-code
		// generators don't clash on the unique index).
		room.InviteCode = repository.GenerateInviteCode()
		return tx.Create(&room).Error
	})
	if errors.Is(txErr, errPaywallLimit) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "paywall_limit",
			"limit": paywallLimit,
			"count": paywallCount,
		})
	}
	if txErr != nil {
		log.Printf("[room.Create] user=%d tx error: %v", user.ID, txErr)
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
	// Atomic membership insert. The previous "IsMember? AddMember"
	// pair had a TOCTOU window where two concurrent Join calls (same
	// invite link tapped twice from a flaky network, or different tabs)
	// both passed the EXISTS check and then raced on the (RoomID, UserID)
	// composite primary key — one of them surfaced a 200 to the client
	// despite the INSERT having failed inside a swallowed
	// AddMember-without-error-check call. The ON CONFLICT DO NOTHING
	// path makes the duplicate a silent no-op AND keeps the error
	// channel honest for real failures (DB outage, etc.).
	// Audit Tier-1 #5.
	member := model.RoomMember{
		RoomID:   room.ID,
		UserID:   user.ID,
		Name:     user.FirstName,
		Username: user.Username,
		Avatar:   user.PhotoURL,
	}
	if err := h.repo.DB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "room_id"}, {Name: "user_id"}},
		DoNothing: true,
	}).Create(&member).Error; err != nil {
		log.Printf("[room.Join] room=%s user=%d add member error: %v", room.ID, user.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "join failed"})
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
	// Escape the user-supplied room name — notifier sends with ParseMode
	// "Markdown" (legacy). Without this, a room called e.g. "*test*"
	// blows up Telegram's parser ("can't parse entities") and the reminder
	// silently never goes out; worse, "[link](http://attacker)" would
	// render as a clickable link inside a system-looking notification.
	// Escape ONCE here, not per-recipient, since the name is the same.
	escapedName := tgutil.EscapeMarkdown(room.Name)
	// Inline button that opens the Mini App straight onto this room, so the
	// reminded member lands on the payment screen in a single tap.
	openRoomKb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text:   "Перейти в комнату",
			WebApp: &models.WebAppInfo{URL: roomDeepLink(h.cfg.BaseURL, id)},
		}}},
	}
	for _, tgID := range tgIDs {
		text := fmt.Sprintf("🔔 Напоминание: оплатите вашу долю в комнате «%s».", escapedName)
		if err := h.notifier.SendMessageWithMarkup(sendCtx, tgID, text, openRoomKb); err != nil {
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
	// 400 on bad UUID — used to fall through to GetByID with the zero
	// UUID, which then 404'd indistinguishably from "real room not
	// found." Tightening the parse path makes the API contract
	// explicit. Audit Low.
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad room id"})
	}
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
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad body"})
	}
	if err := h.repo.AddService(&model.RoomService{
		RoomID: roomID, Brand: body.Brand, Name: body.Name,
		Amount: body.Amount, Currency: body.Currency,
		Note: body.Note, IconName: body.IconName, IconColor: body.IconColor,
	}); err != nil {
		log.Printf("[room.AddService] room=%s: %v", roomID, err)
		return c.Status(500).JSON(fiber.Map{"error": "add service failed"})
	}
	return c.Status(201).JSON(fiber.Map{"ok": true})
}

func (h *RoomHandler) RemoveService(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	roomID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad room id"})
	}
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
