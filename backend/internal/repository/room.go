package repository

import (
	"crypto/rand"
	"math/big"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
)

// RoomRepo handles CRUD for shared rooms, members, and services.
type RoomRepo struct {
	db *gorm.DB
}

func NewRoomRepo(db *gorm.DB) *RoomRepo {
	return &RoomRepo{db: db}
}

// DB exposes the underlying *gorm.DB for callers that need to issue a
// transaction spanning repo logic and ad-hoc queries (e.g. the paywall
// FOR UPDATE wrap in handler/room.go Create). Avoid using outside that
// narrow case — prefer adding a named method on the repo instead.
func (r *RoomRepo) DB() *gorm.DB {
	return r.db
}

func (r *RoomRepo) ListByUser(userID uint) ([]model.SharedRoom, error) {
	var rooms []model.SharedRoom
	err := r.db.
		Joins("JOIN room_members ON room_members.room_id = shared_rooms.id").
		Where("room_members.user_id = ?", userID).
		Preload("Services").
		Preload("Members").
		Find(&rooms).Error
	return rooms, err
}

// memberUserSelect keeps the Members.User preload to the columns
// handler/room.go GetDetail actually projects onto RoomMember (name,
// username, photo_url) plus the PK. Avoids paging in 200+ bytes of
// timezone/locale/notification config we'd never use here. See audit A3.
const memberUserSelect = "id, first_name, username, photo_url"

func (r *RoomRepo) GetByID(id uuid.UUID) (*model.SharedRoom, error) {
	var room model.SharedRoom
	err := r.db.
		Preload("Services").
		Preload("Members.User", func(db *gorm.DB) *gorm.DB {
			return db.Select(memberUserSelect)
		}).
		Where("id = ?", id).First(&room).Error
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) GetByInviteCode(code string) (*model.SharedRoom, error) {
	var room model.SharedRoom
	err := r.db.
		Preload("Services").
		Preload("Members.User", func(db *gorm.DB) *gorm.DB {
			return db.Select(memberUserSelect)
		}).
		Where("invite_code = ?", code).First(&room).Error
	if err != nil {
		return nil, err
	}
	return &room, nil
}

func (r *RoomRepo) Create(room *model.SharedRoom) error {
	room.InviteCode = GenerateInviteCode()
	return r.db.Create(room).Error
}

func (r *RoomRepo) Delete(id uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("room_id = ?", id).Delete(&model.RoomService{}).Error; err != nil {
			return err
		}
		if err := tx.Where("room_id = ?", id).Delete(&model.RoomMember{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&model.SharedRoom{}).Error
	})
}

func (r *RoomRepo) AddMember(member *model.RoomMember) error {
	return r.db.Create(member).Error
}

// IsMember reports whether userID belongs to the given room. The hot path
// (GetDetail / MarkPaid / Remind / RemoveMember) calls this on every
// authenticated room interaction, so we use EXISTS — the PostgreSQL planner
// short-circuits on the first matching row instead of COUNT-ing the entire
// matching set. See audit A2 — at 100k DAU this is a measurable saving.
func (r *RoomRepo) IsMember(roomID uuid.UUID, userID uint) bool {
	var exists bool
	r.db.Raw(
		`SELECT EXISTS(SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ?)`,
		roomID, userID,
	).Scan(&exists)
	return exists
}

func (r *RoomRepo) MarkPaid(roomID uuid.UUID, userID uint) error {
	return r.db.Model(&model.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Updates(map[string]interface{}{"has_paid": true, "paid_at": gorm.Expr("NOW()")}).Error
}

func (r *RoomRepo) MarkUnpaid(roomID uuid.UUID, userID uint) error {
	return r.db.Model(&model.RoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Updates(map[string]interface{}{"has_paid": false, "paid_at": nil}).Error
}

func (r *RoomRepo) AddService(svc *model.RoomService) error {
	return r.db.Create(svc).Error
}

func (r *RoomRepo) RemoveService(roomID uuid.UUID, serviceID uint) error {
	return r.db.Where("room_id = ? AND id = ?", roomID, serviceID).Delete(&model.RoomService{}).Error
}

func (r *RoomRepo) GetUnpaidMembers(roomID uuid.UUID) ([]model.RoomMember, error) {
	var members []model.RoomMember
	err := r.db.Where("room_id = ? AND has_paid = false", roomID).Find(&members).Error
	return members, err
}

// GetUnpaidMemberTelegramIDs returns telegram_id values for all unpaid members of a room.
func (r *RoomRepo) GetUnpaidMemberTelegramIDs(roomID uuid.UUID) ([]int64, error) {
	var ids []int64
	err := r.db.Model(&model.User{}).
		Joins("JOIN room_members ON room_members.user_id = users.id").
		Where("room_members.room_id = ? AND room_members.has_paid = false", roomID).
		Pluck("users.telegram_id", &ids).Error
	return ids, err
}

// RemoveMember removes a member from a room by room ID and user ID.
func (r *RoomRepo) RemoveMember(roomID uuid.UUID, userID uint) error {
	return r.db.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&model.RoomMember{}).Error
}

// SetLastReminded updates the last_reminded_at timestamp for anti-spam cooldown.
func (r *RoomRepo) SetLastReminded(roomID uuid.UUID) error {
	return r.db.Model(&model.SharedRoom{}).Where("id = ?", roomID).Update("last_reminded_at", gorm.Expr("NOW()")).Error
}

// UpdateBillingDay sets the billing day for a room.
func (r *RoomRepo) UpdateBillingDay(roomID uuid.UUID, day int) error {
	return r.db.Model(&model.SharedRoom{}).Where("id = ?", roomID).Update("billing_day", day).Error
}

const inviteChars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghkmnpqrstuvwxyz23456789"

// GenerateInviteCode produces a 12-char alphanumeric token from a
// confusables-safe alphabet (no 0/O/1/I/l). Exported so the
// handler/room.go paywall-locked Create can generate within the tx
// without an extra repo round-trip.
func GenerateInviteCode() string {
	code := make([]byte, 12)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(inviteChars))))
		code[i] = inviteChars[n.Int64()]
	}
	return string(code)
}
