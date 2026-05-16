package repository

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
)

// AdminRepo provides data access for admin-specific operations.
type AdminRepo struct {
	db *gorm.DB
}

func NewAdminRepo(db *gorm.DB) *AdminRepo {
	return &AdminRepo{db: db}
}

// ── Stats ──────────────────────────────────────────────

// TrafficSourceStat is one row of the "today's signups by source" breakdown.
type TrafficSourceStat struct {
	Source string `json:"source"`
	Count  int64  `json:"count"`
}

type StatsResult struct {
	// Users cohorts
	TotalUsers     int64 `json:"total_users"`
	ActiveUsers    int64 `json:"active_users"`
	ChurnedUsers   int64 `json:"churned_users"`
	UsersToday     int64 `json:"users_today"`
	UsersYesterday int64 `json:"users_yesterday"`
	UsersWeek      int64 `json:"users_week"`

	// Locale breakdown
	LocaleRU    int64 `json:"locale_ru"`
	LocaleEN    int64 `json:"locale_en"`
	LocaleOther int64 `json:"locale_other"`

	// Monetization
	Donators    int64 `json:"donators"`
	DonorsToday int64 `json:"donors_today"`

	// Content
	TotalSubscriptions int64 `json:"total_subscriptions"`
	SubsToday          int64 `json:"subs_today"`
	TotalRooms         int64 `json:"total_rooms"`

	// Activity (legacy, kept for API compat)
	DAU int64 `json:"dau"`
	MAU int64 `json:"mau"`

	// Traffic attribution for today's signups (populated separately)
	TodaySources []TrafficSourceStat `json:"today_sources,omitempty" gorm:"-"`
}

func (r *AdminRepo) GetStats() (*StatsResult, error) {
	now := time.Now().UTC()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	weekStart := todayStart.AddDate(0, 0, -7)

	var stats StatsResult
	err := r.db.Raw(`
		SELECT
			-- User cohorts (only non-deleted, non-banned)
			COUNT(*)                                                          AS total_users,
			COUNT(*) FILTER (WHERE u.is_active = true)                        AS active_users,
			COUNT(*) FILTER (WHERE u.is_active = false)                       AS churned_users,
			COUNT(*) FILTER (WHERE u.created_at >= $1)                        AS users_today,
			COUNT(*) FILTER (WHERE u.created_at >= $2 AND u.created_at < $1)  AS users_yesterday,
			COUNT(*) FILTER (WHERE u.created_at >= $3)                        AS users_week,

			-- Locale breakdown — live users only. Blocked accounts skew the
			-- % split and aren't real audience anymore.
			COUNT(*) FILTER (WHERE LOWER(u.locale) = 'ru'              AND u.is_active = true) AS locale_ru,
			COUNT(*) FILTER (WHERE LOWER(u.locale) = 'en'              AND u.is_active = true) AS locale_en,
			COUNT(*) FILTER (WHERE LOWER(u.locale) NOT IN ('ru','en')  AND u.is_active = true) AS locale_other,

			-- Monetization
			COUNT(*) FILTER (WHERE u.is_donator)                              AS donators,
			COUNT(*) FILTER (WHERE u.is_donator AND u.updated_at >= $1)       AS donors_today,

			-- Activity. Inactive users must be excluded — blocking the bot
			-- fires my_chat_member which bumps users.updated_at, so without
			-- this filter every churn event would also re-inflate DAU.
			COUNT(*) FILTER (WHERE u.updated_at >= NOW() - INTERVAL '1 day'  AND u.is_active = true) AS dau,
			COUNT(*) FILTER (WHERE u.updated_at >= NOW() - INTERVAL '30 days' AND u.is_active = true) AS mau
		FROM users u
		WHERE u.deleted_at IS NULL
	`, todayStart, yesterdayStart, weekStart).Scan(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("user stats: %w", err)
	}

	// Content stats — collapsed from three Raw round-trips into a single
	// query with scalar subselects. PostgreSQL plans these in parallel
	// when they touch different tables, so on top of the RTT saving we
	// also get cheaper execution. Audit O3.
	type contentStats struct {
		TotalSubscriptions int64
		SubsToday          int64
		TotalRooms         int64
	}
	var content contentStats
	// Content stats are headline numbers in the admin dashboard — a
	// query failure here used to silently zero them out (the dashboard
	// would display "0 subscriptions / 0 rooms" and the operator would
	// think the DB really was empty, not broken). Surface the error so
	// the handler returns 500 and Sentry captures the cause.
	if err := r.db.Raw(`
		SELECT
			(SELECT COUNT(*) FROM subscriptions)                       AS total_subscriptions,
			(SELECT COUNT(*) FROM subscriptions WHERE created_at >= $1) AS subs_today,
			(SELECT COUNT(*) FROM shared_rooms)                        AS total_rooms
	`, todayStart).Scan(&content).Error; err != nil {
		return nil, fmt.Errorf("content stats: %w", err)
	}
	stats.TotalSubscriptions = content.TotalSubscriptions
	stats.SubsToday = content.SubsToday
	stats.TotalRooms = content.TotalRooms

	// Today's signups by traffic source (top 5). Failure here is logged
	// but NOT fatal — `TodaySources` is a nice-to-have breakdown under
	// the main "users today" number; we'd rather show the rest of the
	// dashboard with a blank top-sources strip than 500 the whole call
	// over a secondary aggregate.
	if stats.UsersToday > 0 {
		var sources []TrafficSourceStat
		if err := r.db.Raw(`
			SELECT
				COALESCE(NULLIF(traffic_source_id, ''), 'organic') AS source,
				COUNT(*) AS count
			FROM users
			WHERE created_at >= $1 AND deleted_at IS NULL
			GROUP BY source
			ORDER BY count DESC
			LIMIT 5
		`, todayStart).Scan(&sources).Error; err != nil {
			log.Printf("[admin.GetStats] today sources query failed: %v", err)
		} else {
			stats.TodaySources = sources
		}
	}

	return &stats, nil
}

type PopularServiceStat struct {
	Brand string `json:"brand"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func (r *AdminRepo) GetPopularServices(limit int) ([]PopularServiceStat, error) {
	var results []PopularServiceStat
	err := r.db.Model(&model.Subscription{}).
		Select("brand, name, COUNT(*) as count").
		Group("brand, name").
		Order("count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// ── Catalog CRUD ───────────────────────────────────────

func (r *AdminRepo) ListCatalog() ([]model.ServiceCatalog, error) {
	var items []model.ServiceCatalog
	err := r.db.Order("name ASC").Find(&items).Error
	return items, err
}

func (r *AdminRepo) CreateCatalogItem(item *model.ServiceCatalog) error {
	return r.db.Create(item).Error
}

// UpdateCatalogItem writes an explicit field list so unset Optional
// fields (PartnerLink, PromoText, BrandColor) aren't overwritten with
// "" when the caller sends a partial body. .Save() would zero them.
func (r *AdminRepo) UpdateCatalogItem(item *model.ServiceCatalog) error {
	return r.db.Model(&model.ServiceCatalog{}).
		Where("id = ?", item.ID).
		Updates(map[string]interface{}{
			"name":             item.Name,
			"category":         item.Category,
			"domain":           item.Domain,
			"brand_color":      item.BrandColor,
			"default_amount":   item.DefaultAmount,
			"default_currency": item.DefaultCurrency,
			"partner_link":     item.PartnerLink,
			"promo_text":       item.PromoText,
			"active":           item.Active,
		}).Error
}

func (r *AdminRepo) DeleteCatalogItem(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.ServiceCatalog{}).Error
}

// ── App Settings ───────────────────────────────────────

func (r *AdminRepo) GetSettings() (*model.AppSettings, error) {
	var s model.AppSettings
	err := r.db.FirstOrCreate(&s, model.AppSettings{ID: 1}).Error
	return &s, err
}

// UpdateSettings persists global app settings. Uses explicit fields via
// .Updates() rather than .Save() to avoid resetting unset string fields
// to "" on partial bodies. The singleton row has id=1.
func (r *AdminRepo) UpdateSettings(s *model.AppSettings) error {
	s.ID = 1
	return r.db.Model(&model.AppSettings{}).
		Where("id = ?", 1).
		Updates(map[string]interface{}{
			"cpa_enabled":                  s.CPAEnabled,
			"recommendations_enabled":      s.RecommendationsEnabled,
			"channel_gate_enabled":         s.ChannelGateEnabled,
			"target_channel":               s.TargetChannel,
			"paywall_enabled":              s.PaywallEnabled,
			"free_subs_limit":              s.FreeSubsLimit,
			"free_room_limit":              s.FreeRoomLimit,
			"maintenance_mode":             s.MaintenanceMode,
			"pause_notifications":          s.PauseNotifications,
			"price_stars_month_ru":         s.PriceStarsMonthRU,
			"price_stars_lifetime_ru":      s.PriceStarsLifetimeRU,
			"price_stars_month_en":         s.PriceStarsMonthEN,
			"price_stars_lifetime_en":      s.PriceStarsLifetimeEN,
			"price_crypto_month_usd_ru":    s.PriceCryptoMonthUSDRU,
			"price_crypto_lifetime_usd_ru": s.PriceCryptoLifetimeUSDRU,
			"price_crypto_month_usd_en":    s.PriceCryptoMonthUSDEN,
			"price_crypto_lifetime_usd_en": s.PriceCryptoLifetimeUSDEN,
		}).Error
}

// CountUserSubscriptions returns the number of subscriptions for a user.
func (r *AdminRepo) CountUserSubscriptions(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.Subscription{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// CountUserOwnedRooms returns the number of rooms a user owns.
func (r *AdminRepo) CountUserOwnedRooms(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.SharedRoom{}).Where("owner_id = ?", userID).Count(&count).Error
	return count, err
}

// ── User Management ────────────────────────────────────

// NOTE: A previous `GetExportUsers()` helper materialised the entire
// users table into a single slice. It's been removed — the CSV export
// streams rows via FindInBatches directly from bot/admin_panel.go to
// keep peak memory bounded. See audit C4.

func (r *AdminRepo) FindUserByTelegramID(tgID int64) (*model.User, error) {
	var u model.User
	err := r.db.Where("telegram_id = ?", tgID).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *AdminRepo) FindUserByUsername(username string) (*model.User, error) {
	var u model.User
	err := r.db.Where("username = ?", username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *AdminRepo) SetDonatorStatus(id uint, granted bool) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("is_donator", granted).Error
}

func (r *AdminRepo) SetBannedStatus(id uint, banned bool) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("is_banned", banned).Error
}

func (r *AdminRepo) SoftDeleteUser(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}

// ── Traffic Campaigns ──────────────────────────────────

func (r *AdminRepo) ListCampaigns() ([]model.TrafficCampaign, error) {
	var items []model.TrafficCampaign
	err := r.db.Order("clicks DESC").Find(&items).Error
	return items, err
}

func (r *AdminRepo) IncrementCampaign(tag string, field string) error {
	// Defence-in-depth: only allow known column names to prevent SQL injection
	// via the interpolated field parameter.
	allowed := map[string]bool{"clicks": true, "bot_starts": true, "auths": true}
	if !allowed[field] {
		return fmt.Errorf("invalid campaign field: %q", field)
	}
	return r.db.Exec(
		`INSERT INTO traffic_campaigns (tag, `+field+`, created_at)
		 VALUES (?, 1, NOW())
		 ON CONFLICT (tag) DO UPDATE SET `+field+` = traffic_campaigns.`+field+` + 1`,
		tag,
	).Error
}

// EnsureCampaign creates a campaign row if it doesn't exist yet (eager creation).
func (r *AdminRepo) EnsureCampaign(tag string) error {
	return r.db.Exec(
		`INSERT INTO traffic_campaigns (tag, clicks, bot_starts, auths, created_at)
		 VALUES (?, 0, 0, 0, NOW())
		 ON CONFLICT (tag) DO NOTHING`,
		tag,
	).Error
}

func (r *AdminRepo) GetCampaignByTag(tag string) (*model.TrafficCampaign, error) {
	var c model.TrafficCampaign
	err := r.db.Where("tag = ?", tag).First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *AdminRepo) DeleteCampaign(tag string) error {
	return r.db.Where("tag = ?", tag).Delete(&model.TrafficCampaign{}).Error
}

// ── Sponsored Offers ───────────────────────────────────

func (r *AdminRepo) ListOffers() ([]model.SponsoredOffer, error) {
	var items []model.SponsoredOffer
	err := r.db.Order("created_at DESC").Find(&items).Error
	return items, err
}

func (r *AdminRepo) ListActiveOffers(lang string) ([]model.SponsoredOffer, error) {
	var items []model.SponsoredOffer
	err := r.db.Where("is_active = ? AND (target_language = ? OR target_language = 'all')", true, lang).
		Order("created_at DESC").Find(&items).Error
	return items, err
}

func (r *AdminRepo) CreateOffer(o *model.SponsoredOffer) error {
	return r.db.Create(o).Error
}

func (r *AdminRepo) ToggleOffer(id uint, active bool) error {
	return r.db.Model(&model.SponsoredOffer{}).Where("id = ?", id).Update("is_active", active).Error
}

func (r *AdminRepo) GetOffer(id uint) (*model.SponsoredOffer, error) {
	var o model.SponsoredOffer
	err := r.db.First(&o, id).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *AdminRepo) DeleteOffer(id uint) error {
	return r.db.Delete(&model.SponsoredOffer{}, id).Error
}

// IncrementViews bumps `views` for the given offer IDs, but ONLY for
// offers that (a) are currently active and (b) target the caller's
// language (or are tagged "all"). This kills two abuse classes that
// the previous unguarded UPDATE was vulnerable to:
//
//   - An EN-locale user firing TrackView with the IDs of every RU
//     offer they'd never legitimately see, inflating impressions.
//   - A bot crawling for offer IDs and crunching counters at scale.
//
// Audit Tier-1 #7.
func (r *AdminRepo) IncrementViews(ids []uint, locale string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Model(&model.SponsoredOffer{}).
		Where("id IN ? AND is_active = ? AND (target_language = ? OR target_language = ?)",
			ids, true, locale, "all").
		UpdateColumn("views", gorm.Expr("views + 1")).Error
}

// IncrementClick bumps `clicks` for one offer iff it's active and
// targets the caller's locale (same constraint as IncrementViews).
func (r *AdminRepo) IncrementClick(id uint, locale string) error {
	return r.db.Model(&model.SponsoredOffer{}).
		Where("id = ? AND is_active = ? AND (target_language = ? OR target_language = ?)",
			id, true, locale, "all").
		UpdateColumn("clicks", gorm.Expr("clicks + 1")).Error
}

// ── Broadcast ──────────────────────────────────────────

// CountBroadcastRecipients returns the count of eligible users for a
// broadcast filtered by language segment. Excludes banned, soft-deleted,
// and inactive (bot-blocked) users — there's no point counting accounts
// the Telegram API will hard-fail with "bot was blocked by the user".
// When lang == "en", includes users whose locale is empty or NULL
// (fallback — undetected locale defaults to English audience).
func (r *AdminRepo) CountBroadcastRecipients(lang string) (int64, error) {
	var count int64
	q := r.db.Model(&model.User{}).Where("is_banned = false AND deleted_at IS NULL AND is_active = true")
	switch lang {
	case "ru":
		q = q.Where("LOWER(locale) = 'ru'")
	case "en":
		q = q.Where("LOWER(locale) = 'en' OR locale IS NULL OR locale = ''")
	}
	// "all" — no extra filter
	err := q.Count(&count).Error
	return count, err
}
