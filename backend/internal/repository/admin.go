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

func (r *AdminRepo) UpdateCatalogItem(item *model.ServiceCatalog) error {
	return r.db.Save(item).Error
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

func (r *AdminRepo) UpdateSettings(s *model.AppSettings) error {
	s.ID = 1
	return r.db.Save(s).Error
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
