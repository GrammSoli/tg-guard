package model

import (
	"time"

	"github.com/google/uuid"
)

// User represents a Telegram user registered in SubGuard.
type User struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TelegramID      int64     `gorm:"uniqueIndex;not null" json:"telegram_id"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Username        string    `json:"username"`
	PhotoURL        string    `json:"photo_url,omitempty"`
	Locale          string    `gorm:"default:en;size:5" json:"locale"`
	Timezone        string    `gorm:"default:UTC;size:64" json:"timezone"`
	BaseCurrency    string    `gorm:"default:USD;size:3" json:"base_currency"`
	IsDonator       bool      `gorm:"default:false" json:"is_donator"`
	// NotificationsEnabled controls whether the notification worker sends
	// payment-reminder DMs to this user. Default true to preserve the
	// pre-existing behaviour for users who registered before this field.
	NotificationsEnabled bool `gorm:"default:true;not null" json:"notifications_enabled"`
	// NotificationTime is the local-time-of-day ("HH:MM", 24h) at which the
	// notification worker is allowed to fire reminders to this user. Stored
	// as a string for simplicity — only the worker parses it. Interpreted in
	// the user's Timezone above.
	NotificationTime string    `gorm:"default:'10:00';size:5;not null" json:"notification_time"`
	TrafficSourceID  string    `gorm:"size:64" json:"traffic_source_id,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Subscription represents a user's tracked subscription.
type Subscription struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID        uint       `gorm:"index;not null" json:"-"`
	User          User       `gorm:"foreignKey:UserID" json:"-"`
	Name          string     `gorm:"not null;size:100" json:"name"`
	Brand         string     `gorm:"default:default;size:32" json:"brand"`
	Tag           string     `gorm:"size:64" json:"tag,omitempty"`
	// Note is a freeform user-supplied tag to distinguish duplicate
	// subscriptions ("Netflix — parents", "Netflix — work"). Optional.
	Note          string     `gorm:"size:128" json:"note,omitempty"`
	Amount        float64    `gorm:"not null" json:"amount"`
	Currency      string     `gorm:"default:USD;size:3" json:"currency"`
	Period        string     `gorm:"default:monthly;size:10" json:"period"` // monthly | yearly | weekly
	NextPaymentAt time.Time  `gorm:"index;not null" json:"next_payment_at"`
	IsTrial       bool       `gorm:"default:false" json:"is_trial"`
	TrialEndsAt   *time.Time `json:"trial_ends_at"`
	IsAutoPay     bool       `gorm:"default:true" json:"is_auto_pay"`
	NotifiedAt    *time.Time `gorm:"index" json:"-"` // tracks last notification sent
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// SharedRoom represents a group subscription room.
type SharedRoom struct {
	ID             uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name           string        `gorm:"not null;size:50" json:"name"`
	OwnerID        uint          `gorm:"index;not null" json:"owner_id"`
	InviteCode     string        `gorm:"uniqueIndex;size:16" json:"invite_code"`
	Currency       string        `gorm:"default:USD;size:3" json:"currency"`
	BillingDay     int           `gorm:"default:1" json:"billing_day"`
	LastRemindedAt *time.Time    `json:"last_reminded_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	Services       []RoomService `gorm:"foreignKey:RoomID" json:"services"`
	Members        []RoomMember  `gorm:"foreignKey:RoomID" json:"members"`
}

// RoomService is a subscription service attached to a shared room.
type RoomService struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	RoomID        uuid.UUID  `gorm:"type:uuid;index;not null" json:"-"`
	Brand         string     `gorm:"size:32" json:"brand"`
	Name          string     `gorm:"size:100" json:"name"`
	Amount        float64    `json:"amount"`
	Currency      string     `gorm:"size:3" json:"currency"`
	NextPaymentAt *time.Time `json:"next_payment_at,omitempty"`
}

// RoomMember links a user to a shared room with payment status.
type RoomMember struct {
	RoomID   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"-"`
	UserID   uint       `gorm:"primaryKey" json:"user_id"`
	Name     string     `gorm:"size:100" json:"name"`
	Username string     `gorm:"size:64" json:"username,omitempty"`
	Avatar   string     `gorm:"size:512" json:"avatar,omitempty"`
	HasPaid  bool       `gorm:"index;default:false" json:"has_paid"`
	PaidAt   *time.Time `json:"paid_at,omitempty"`
}

// ServiceCatalog holds the preset services available for users to pick from.
type ServiceCatalog struct {
	ID              string  `gorm:"primaryKey;size:32" json:"id"`
	Name            string  `gorm:"not null;size:100" json:"name"`
	Category        string  `gorm:"size:32" json:"category"`
	Domain          string  `gorm:"size:128" json:"domain"`
	BrandColor      string  `gorm:"size:7" json:"brand_color,omitempty"`
	DefaultAmount   float64 `json:"default_amount"`
	DefaultCurrency string  `gorm:"size:3;default:USD" json:"default_currency"`
	PartnerLink     string  `gorm:"size:512" json:"partner_link,omitempty"`
	PromoText       string  `gorm:"size:255" json:"promo_text,omitempty"`
	Active          bool    `gorm:"default:true" json:"active"`
}

// TrafficCampaign tracks deep-link ad campaigns.
type TrafficCampaign struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Tag       string    `gorm:"uniqueIndex;size:64;not null" json:"tag"`
	Clicks    int64     `gorm:"default:0" json:"clicks"`
	BotStarts int64     `gorm:"default:0" json:"bot_starts"`
	Auths     int64     `gorm:"default:0" json:"auths"`
	CreatedAt time.Time `json:"created_at"`
}

// AppSettings stores global feature toggles (single-row table).
type AppSettings struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	CPAEnabled         bool   `gorm:"default:true" json:"cpa_enabled"`
	ChannelGateEnabled bool   `gorm:"default:false" json:"channel_gate_enabled"`
	TargetChannel      string `gorm:"size:64" json:"target_channel"`
}

// Donation logs a successful Telegram Stars payment.
type Donation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;not null" json:"user_id"`
	TelegramID int64     `json:"telegram_id"`
	Amount     int       `json:"amount"` // in Telegram Stars
	CreatedAt  time.Time `json:"created_at"`
}
