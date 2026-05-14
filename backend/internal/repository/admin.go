package repository

import (
	"fmt"
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
	TotalUsers    int64 `json:"total_users"`
	UsersToday    int64 `json:"users_today"`
	UsersYesterday int64 `json:"users_yesterday"`
	UsersWeek     int64 `json:"users_week"`

	// Locale breakdown
	LocaleRU    int64 `json:"locale_ru"`
	LocaleEN    int64 `json:"locale_en"`
	LocaleOther int64 `json:"locale_other"`

	// Monetization
	Donators      int64 `json:"donators"`
	DonorsToday   int64 `json:"donors_today"`

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
			COUNT(*) FILTER (WHERE u.created_at >= $1)                        AS users_today,
			COUNT(*) FILTER (WHERE u.created_at >= $2 AND u.created_at < $1)  AS users_yesterday,
			COUNT(*) FILTER (WHERE u.created_at >= $3)                        AS users_week,

			-- Locale breakdown
			COUNT(*) FILTER (WHERE LOWER(u.locale) = 'ru')                    AS locale_ru,
			COUNT(*) FILTER (WHERE LOWER(u.locale) = 'en')                    AS locale_en,
			COUNT(*) FILTER (WHERE LOWER(u.locale) NOT IN ('ru','en'))        AS locale_other,

			-- Monetization
			COUNT(*) FILTER (WHERE u.is_donator)                              AS donators,
			COUNT(*) FILTER (WHERE u.is_donator AND u.updated_at >= $1)       AS donors_today,

			-- Activity
			COUNT(*) FILTER (WHERE u.updated_at >= NOW() - INTERVAL '1 day')  AS dau,
			COUNT(*) FILTER (WHERE u.updated_at >= NOW() - INTERVAL '30 days') AS mau
		FROM users u
		WHERE u.deleted_at IS NULL
	`, todayStart, yesterdayStart, weekStart).Scan(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("user stats: %w", err)
	}

	// Content stats (subscriptions + rooms) — separate lightweight queries
	r.db.Raw(`SELECT COUNT(*) FROM subscriptions`).Scan(&stats.TotalSubscriptions)
	r.db.Raw(`SELECT COUNT(*) FROM subscriptions WHERE created_at >= $1`, todayStart).Scan(&stats.SubsToday)
	r.db.Raw(`SELECT COUNT(*) FROM shared_rooms`).Scan(&stats.TotalRooms)

	// Today's signups by traffic source (top 5)
	if stats.UsersToday > 0 {
		var sources []TrafficSourceStat
		r.db.Raw(`
			SELECT
				COALESCE(NULLIF(traffic_source_id, ''), 'organic') AS source,
				COUNT(*) AS count
			FROM users
			WHERE created_at >= $1 AND deleted_at IS NULL
			GROUP BY source
			ORDER BY count DESC
			LIMIT 5
		`, todayStart).Scan(&sources)
		stats.TodaySources = sources
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
			"cpa_enabled":             s.CPAEnabled,
			"recommendations_enabled": s.RecommendationsEnabled,
			"channel_gate_enabled":    s.ChannelGateEnabled,
			"target_channel":          s.TargetChannel,
		}).Error
}

// ── User Management ────────────────────────────────────

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

func (r *AdminRepo) IncrementViews(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.Model(&model.SponsoredOffer{}).
		Where("id IN ?", ids).
		UpdateColumn("views", gorm.Expr("views + 1")).Error
}

func (r *AdminRepo) IncrementClick(id uint) error {
	return r.db.Model(&model.SponsoredOffer{}).
		Where("id = ?", id).
		UpdateColumn("clicks", gorm.Expr("clicks + 1")).Error
}
