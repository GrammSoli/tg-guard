package repository

import (
	"fmt"

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

type StatsResult struct {
	TotalUsers          int64 `json:"total_users"`
	DAU                 int64 `json:"dau"`
	MAU                 int64 `json:"mau"`
	Donators            int64 `json:"donators"`
	TotalSubscriptions  int64 `json:"total_subscriptions"`
	TotalRooms          int64 `json:"total_rooms"`
}

func (r *AdminRepo) GetStats() (*StatsResult, error) {
	var stats StatsResult
	err := r.db.Raw(`
		SELECT
			(SELECT count(*) FROM users) AS total_users,
			(SELECT count(*) FROM users WHERE updated_at >= now() - interval '1 day') AS dau,
			(SELECT count(*) FROM users WHERE updated_at >= now() - interval '30 days') AS mau,
			(SELECT count(*) FROM users WHERE is_donator) AS donators,
			(SELECT count(*) FROM subscriptions) AS total_subscriptions,
			(SELECT count(*) FROM shared_rooms) AS total_rooms
	`).Scan(&stats).Error
	if err != nil {
		return nil, err
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
		`INSERT INTO traffic_campaigns (tag, `+field+`, created_at, updated_at)
		 VALUES (?, 1, NOW(), NOW())
		 ON CONFLICT (tag) DO UPDATE SET `+field+` = traffic_campaigns.`+field+` + 1, updated_at = NOW()`,
		tag,
	).Error
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

// ── User lookup (admin) ────────────────────────────────

func (r *AdminRepo) FindUserByTelegramID(tgID int64) (*model.User, error) {
	var u model.User
	err := r.db.Where("telegram_id = ?", tgID).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *AdminRepo) SetDonatorStatus(userID uint, isDonator bool) error {
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("is_donator", isDonator).Error
}
