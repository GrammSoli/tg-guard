package seed

import (
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
)

// Known test IDs — keep in sync with Playwright fixtures.
const (
	TestOwnerTelegramID  int64 = 111111
	TestMemberTelegramID int64 = 222222
	TestDebtorTelegramID int64 = 333333
)

// SeedTestData creates deterministic test users, a shared room, and services.
// Idempotent — skips if test owner already exists.
func SeedTestData(db *gorm.DB) {
	var count int64
	db.Model(&model.User{}).Where("telegram_id = ?", TestOwnerTelegramID).Count(&count)
	if count > 0 {
		log.Println("[seed-test] test data already exists, skipping")
		return
	}

	log.Println("[seed-test] creating test data...")

	// ── Users ───────────────────────────────────────────
	owner := model.User{
		TelegramID: TestOwnerTelegramID,
		FirstName:  "TestOwner",
		Username:   "test_owner",
	}
	member := model.User{
		TelegramID: TestMemberTelegramID,
		FirstName:  "TestMember",
		Username:   "test_member",
	}
	debtor := model.User{
		TelegramID: TestDebtorTelegramID,
		FirstName:  "TestDebtor",
		Username:   "test_debtor",
	}
	db.Create(&owner)
	db.Create(&member)
	db.Create(&debtor)

	// ── Shared Room ─────────────────────────────────────
	roomID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	now := time.Now()
	room := model.SharedRoom{
		ID:         roomID,
		Name:       "Test Room",
		OwnerID:    owner.ID,
		InviteCode: "test_invite_e2e",
		Currency:   "USD",
		BillingDay: 15,
		CreatedAt:  now,
	}
	db.Create(&room)

	// ── Services ────────────────────────────────────────
	db.Create(&model.RoomService{
		RoomID:   roomID,
		Brand:    "netflix",
		Name:     "Netflix Premium",
		Amount:   15.99,
		Currency: "USD",
	})
	db.Create(&model.RoomService{
		RoomID:   roomID,
		Brand:    "youtube",
		Name:     "YouTube Premium",
		Amount:   11.99,
		Currency: "USD",
	})

	// ── Members ─────────────────────────────────────────
	paidAt := time.Now()
	db.Create(&model.RoomMember{
		RoomID:   roomID,
		UserID:   owner.ID,
		Name:     "TestOwner",
		Username: "test_owner",
		HasPaid:  true,
		PaidAt:   &paidAt,
	})
	db.Create(&model.RoomMember{
		RoomID:   roomID,
		UserID:   member.ID,
		Name:     "TestMember",
		Username: "test_member",
		HasPaid:  true,
		PaidAt:   &paidAt,
	})
	db.Create(&model.RoomMember{
		RoomID:   roomID,
		UserID:   debtor.ID,
		Name:     "TestDebtor",
		Username: "test_debtor",
		HasPaid:  false,
	})

	log.Printf("[seed-test] created test room %s with 3 members (invite: test_invite_e2e)", roomID)
}
