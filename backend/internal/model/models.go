package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	// PremiumExpiresAt is when a time-limited Premium grant lapses. NULL
	// means lifetime (or no Premium at all — disambiguate via IsDonator).
	// The premium-expiration worker clears IsDonator once now() passes it.
	PremiumExpiresAt *time.Time `json:"premium_expires_at"`
	// NotificationsEnabled controls whether the notification worker sends
	// payment-reminder DMs to this user. Default true to preserve the
	// pre-existing behaviour for users who registered before this field.
	NotificationsEnabled bool `gorm:"default:true;not null" json:"notifications_enabled"`
	// NotificationTime is the local-time-of-day ("HH:MM", 24h) at which the
	// notification worker is allowed to fire reminders to this user. Stored
	// as a string for simplicity — only the worker parses it. Interpreted in
	// the user's Timezone above.
	NotificationTime string    `gorm:"default:'10:00';size:5;not null" json:"notification_time"`
	IsActive         bool           `gorm:"default:true;not null" json:"is_active"`
	IsBanned         bool           `gorm:"default:false;not null" json:"is_banned"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	TrafficSourceID  string         `gorm:"size:64" json:"traffic_source_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// Subscription represents a user's tracked subscription.
type Subscription struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	// UserID carries both a single-column index (for "list a user's subs"
	// queries) AND participates in the composite idx_sub_user_payment used
	// by the per-user-on-day worker scan path.
	UserID uint `gorm:"index;not null" json:"-"`
	// CASCADE so that DELETE FROM users transparently wipes their subs at
	// the DB layer — the explicit handler.DeleteMe still runs the same
	// cleanup in a transaction, this is defence in depth against direct
	// SQL writes / future admin scripts.
	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"-"`
	Name          string     `gorm:"not null;size:100" json:"name"`
	Brand         string     `gorm:"default:default;size:32" json:"brand"`
	Tag           string     `gorm:"size:64" json:"tag,omitempty"`
	// Note is a freeform user-supplied tag to distinguish duplicate
	// subscriptions ("Netflix — parents", "Netflix — work"). Optional.
	Note          string     `gorm:"size:128" json:"note,omitempty"`
	// IconName / IconColor are only meaningful for custom subscriptions
	// (Brand == "default"). The frontend's icon registry maps the string
	// name to a lucide-react icon component; unknown names fall back to
	// the letter-avatar placeholder. Storing as plain strings keeps the
	// backend agnostic about the React-side allow-list.
	IconName      string     `gorm:"size:32" json:"icon_name,omitempty"`
	IconColor     string     `gorm:"size:16" json:"icon_color,omitempty"`
	Amount        float64    `gorm:"not null" json:"amount"`
	Currency      string     `gorm:"default:USD;size:3;not null" json:"currency"`
	Period        string     `gorm:"default:monthly;size:10;not null" json:"period"` // monthly | yearly | weekly
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
	// CASCADE so DELETE FROM shared_rooms cleans up children at DB layer.
	// handler.DeleteRoom still wraps the manual deletes in a transaction;
	// this is defence in depth against direct SQL writes.
	Services []RoomService `gorm:"foreignKey:RoomID;constraint:OnDelete:CASCADE" json:"services"`
	Members  []RoomMember  `gorm:"foreignKey:RoomID;constraint:OnDelete:CASCADE" json:"members"`
}

// RoomService is a subscription service attached to a shared room.
type RoomService struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	RoomID        uuid.UUID  `gorm:"type:uuid;index;not null" json:"-"`
	Brand         string     `gorm:"size:32" json:"brand"`
	Name          string     `gorm:"size:100" json:"name"`
	Note          string     `gorm:"size:128" json:"note,omitempty"`
	IconName      string     `gorm:"size:32" json:"icon_name,omitempty"`
	IconColor     string     `gorm:"size:16" json:"icon_color,omitempty"`
	Amount        float64    `json:"amount"`
	Currency      string     `gorm:"size:3" json:"currency"`
	NextPaymentAt *time.Time `json:"next_payment_at,omitempty"`
}

// RoomMember links a user to a shared room with payment status.
//
// Indexing notes:
//   - The (RoomID, UserID) composite PK indexes "list members of room X"
//     queries (PK starts with RoomID).
//   - UserID gets its own non-unique index so the reverse direction —
//     "list rooms user X is in" / "delete all memberships of user X" —
//     doesn't seq-scan.
//   - HasPaid + RoomID composite covers the billing-reset worker query
//     `WHERE room_id = ? AND has_paid = false`.
type RoomMember struct {
	RoomID uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_rm_room_paid,priority:1" json:"-"`
	UserID uint      `gorm:"primaryKey;index" json:"user_id"`
	// User is a `belongs to` relation populated by Preload("Members.User")
	// in RoomRepo.GetByID. Lets handler/room.go skip the second
	// GetUsersByIDs roundtrip — see audit A3. JSON-omitted because the
	// flattened Name/Username/Avatar fields below already encode what
	// the API consumer needs (and we don't want to leak the full User).
	User     *User      `gorm:"foreignKey:UserID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
	Name     string     `gorm:"size:100" json:"name"`
	Username string     `gorm:"size:64" json:"username,omitempty"`
	Avatar   string     `gorm:"size:512" json:"avatar,omitempty"`
	HasPaid  bool       `gorm:"index:idx_rm_room_paid,priority:2;default:false" json:"has_paid"`
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
	ID                     uint   `gorm:"primaryKey" json:"id"`
	CPAEnabled             bool   `gorm:"default:true" json:"cpa_enabled"`
	RecommendationsEnabled bool   `gorm:"default:true" json:"recommendations_enabled"`
	ChannelGateEnabled     bool   `gorm:"default:false" json:"channel_gate_enabled"`
	TargetChannel          string `gorm:"size:64" json:"target_channel"`
	// Paywall (grandfathering) — soft limits for free-tier users.
	PaywallEnabled         bool   `gorm:"default:false" json:"paywall_enabled"`
	FreeSubsLimit          int    `gorm:"default:6" json:"free_subs_limit"`
	FreeRoomLimit          int    `gorm:"default:1" json:"free_room_limit"`
	// Emergency kill-switches, toggled from the in-bot admin panel.
	//   MaintenanceMode — when true the maintenance middleware answers
	//     every non-admin /api request with 503; the mini-app shows a
	//     full-screen "be right back" stub.
	//   PauseNotifications — when true the notification worker skips its
	//     send tick entirely (reminders not sent, not marked — so they
	//     fire normally once the switch is flipped back off).
	MaintenanceMode        bool   `gorm:"default:false;not null" json:"maintenance_mode"`
	PauseNotifications     bool   `gorm:"default:false;not null" json:"pause_notifications"`
	// Premium pricing, split by user locale. Stars prices are whole
	// Telegram Stars; crypto prices are whole USD. The mini-app reads
	// these from GET /api/v1/config and shows the locale-matched price
	// in the PremiumSheet; the bot admin panel edits them in ±50 (Stars)
	// / ±1 (crypto) steps.
	PriceStarsRU     int `gorm:"default:50;not null" json:"price_stars_ru"`
	PriceStarsEN     int `gorm:"default:100;not null" json:"price_stars_en"`
	PriceCryptoUsdRU int `gorm:"default:1;not null" json:"price_crypto_usd_ru"`
	PriceCryptoUsdEN int `gorm:"default:2;not null" json:"price_crypto_usd_en"`
	// Plan-split Premium pricing — Month vs Lifetime. Supersedes the
	// flat Price* fields above for the two-tier paywall. Stars stay
	// locale-split (RU/EN); crypto is a single USD amount per plan.
	PriceStarsMonthRU      int `gorm:"default:75;not null" json:"price_stars_month_ru"`
	PriceStarsLifetimeRU   int `gorm:"default:500;not null" json:"price_stars_lifetime_ru"`
	PriceStarsMonthEN      int `gorm:"default:150;not null" json:"price_stars_month_en"`
	PriceStarsLifetimeEN   int `gorm:"default:1000;not null" json:"price_stars_lifetime_en"`
	PriceCryptoMonthUSD    int `gorm:"default:2;not null" json:"price_crypto_month_usd"`
	PriceCryptoLifetimeUSD int `gorm:"default:20;not null" json:"price_crypto_lifetime_usd"`
}

// Donation logs a successful Telegram Stars payment.
//
// TelegramPaymentChargeID is Telegram's unique transaction identifier
// (from SuccessfulPayment). Indexed as UNIQUE to make webhook processing
// idempotent — Telegram retries SuccessfulPayment on non-200/timeout, and
// without this guard we'd create duplicate rows and spam the user with
// repeated "thank you" messages.
type Donation struct {
	ID                       uint      `gorm:"primaryKey" json:"id"`
	UserID                   uint      `gorm:"index;not null" json:"user_id"`
	TelegramID               int64     `json:"telegram_id"`
	TelegramPaymentChargeID  string    `gorm:"uniqueIndex;size:512;not null" json:"telegram_payment_charge_id"`
	Amount                   int       `json:"amount"` // in Telegram Stars
	CreatedAt                time.Time `json:"created_at"`
}

// SponsoredOffer is an admin-created promotional card displayed in the
// "Recommended" section of the dashboard. TargetLanguage controls which
// users see it: "ru", "en", or "all".
type SponsoredOffer struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Title          string    `gorm:"not null;size:128" json:"title"`
	Description    string    `gorm:"size:255" json:"description"`
	BadgeText      string    `gorm:"size:32" json:"badge_text"`
	URL            string    `gorm:"size:512;not null" json:"url"`
	IconName       string    `gorm:"size:512" json:"icon_name"`
	TargetLanguage string    `gorm:"size:5;default:'all'" json:"target_language"`
	IsActive       bool      `gorm:"default:true" json:"is_active"`
	Views          uint      `gorm:"default:0" json:"views"`
	Clicks         uint      `gorm:"default:0" json:"clicks"`
	CreatedAt      time.Time `json:"created_at"`
}
