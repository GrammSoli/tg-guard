package handler

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
	"github.com/subguard/backend/internal/workerutil"
)

// broadcastAPILockKey is the Redis single-flight gate for /admin/broadcast.
// One key for the whole cluster — only ONE API-initiated broadcast can be
// in flight at a time. Mirrors bot/broadcast.go's per-message lock but a
// single global slot is fine for the HTTP path because the admin can
// only see one "Send" button at a time. TTL covers the maximum run time
// (matches runBroadcast's 24h ctx); defer-Del releases earlier.
const broadcastAPILockKey = "broadcast_api_lock"

// AdminHandler handles all admin-panel endpoints.
//
// rdb is used by Broadcast() to acquire a cluster-wide single-flight
// lock; nil disables the lock and falls back to "best-effort, may
// double-fire under concurrent clicks" — matches the original behaviour
// for paths that never wire Redis (currently none in prod, only tests).
// wg is the server-lifecycle WaitGroup; goroutines launched from this
// handler register on it so graceful shutdown drains them before the
// DB/Redis pools close.
type AdminHandler struct {
	repo   *repository.AdminRepo
	cfg    *config.Config
	db     *gorm.DB
	bot    *bot.Bot
	rdb    *redis.Client
	wg     *sync.WaitGroup
	appCtx context.Context
}

// NewAdminHandler builds an AdminHandler. appCtx should be the server's
// parent lifecycle context so background goroutines (broadcast) cancel on
// shutdown instead of leaking past SIGTERM. rdb is required for the
// /admin/broadcast single-flight lock — pass nil only in tests that
// don't exercise that path.
func NewAdminHandler(db *gorm.DB, cfg *config.Config, b *bot.Bot, rdb *redis.Client, appCtx context.Context) *AdminHandler {
	if appCtx == nil {
		appCtx = context.Background()
	}
	return &AdminHandler{
		repo:   repository.NewAdminRepo(db),
		cfg:    cfg,
		db:     db,
		bot:    b,
		rdb:    rdb,
		appCtx: appCtx,
	}
}

// WithLifecycle wires the server lifecycle WaitGroup so background
// broadcast goroutines spawned from HTTP handlers register for graceful
// drain. Mirrors UserHandler.WithLifecycle. Call from main.go.
func (h *AdminHandler) WithLifecycle(wg *sync.WaitGroup) *AdminHandler {
	h.wg = wg
	return h
}

// AdminOnly is a middleware that restricts access to admin users.
func (h *AdminHandler) AdminOnly(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil || !h.cfg.IsAdmin(user.TelegramID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
	}
	return c.Next()
}

// GetPublicConfig returns the paywall configuration visible to all
// authenticated users. The frontend reads this on boot to know whether
// to enforce soft limits client-side.
// GET /api/v1/config
func (h *AdminHandler) GetPublicConfig(c fiber.Ctx) error {
	s, err := h.repo.GetSettings()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "config failed"})
	}
	return c.JSON(fiber.Map{
		"paywall_enabled": s.PaywallEnabled,
		"free_subs_limit": s.FreeSubsLimit,
		"free_room_limit": s.FreeRoomLimit,
		// Plan-split pricing (Month / Lifetime). Stars locale-split;
		// crypto a single USD amount per plan. The PremiumSheet picks
		// the Stars pair by i18n language and shows both plans.
		"price_stars_month_ru":         s.PriceStarsMonthRU,
		"price_stars_lifetime_ru":      s.PriceStarsLifetimeRU,
		"price_stars_month_en":         s.PriceStarsMonthEN,
		"price_stars_lifetime_en":      s.PriceStarsLifetimeEN,
		"price_crypto_month_usd_ru":    s.PriceCryptoMonthUSDRU,
		"price_crypto_lifetime_usd_ru": s.PriceCryptoLifetimeUSDRU,
		"price_crypto_month_usd_en":    s.PriceCryptoMonthUSDEN,
		"price_crypto_lifetime_usd_en": s.PriceCryptoLifetimeUSDEN,
	})
}

// GetStats returns live KPI metrics.
func (h *AdminHandler) GetStats(c fiber.Ctx) error {
	stats, err := h.repo.GetStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "stats failed"})
	}
	// Zero time as the lower bound = all-time popularity for this API.
	popular, _ := h.repo.GetPopularServices(10, time.Time{})
	return c.JSON(fiber.Map{
		"stats":            stats,
		"popular_services": popular,
	})
}

// ListCatalog returns the service catalog.
func (h *AdminHandler) ListCatalog(c fiber.Ctx) error {
	items, err := h.repo.ListCatalog()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "catalog failed"})
	}
	return c.JSON(items)
}

// CreateCatalogItem adds a new service to the catalog.
func (h *AdminHandler) CreateCatalogItem(c fiber.Ctx) error {
	var item model.ServiceCatalog
	if err := c.Bind().JSON(&item); err != nil || item.ID == "" || item.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "id and name required"})
	}
	if item.DefaultAmount <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "default amount must be > 0"})
	}
	if item.DefaultCurrency == "" {
		item.DefaultCurrency = "USD"
	}
	if err := h.repo.CreateCatalogItem(&item); err != nil {
		return c.Status(409).JSON(fiber.Map{"error": "create failed or id already exists"})
	}
	return c.Status(201).JSON(item)
}

// UpdateCatalogItem modifies an existing catalog service.
func (h *AdminHandler) UpdateCatalogItem(c fiber.Ctx) error {
	var item model.ServiceCatalog
	if err := c.Bind().JSON(&item); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad body"})
	}
	item.ID = c.Params("id")
	if err := h.repo.UpdateCatalogItem(&item); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	return c.JSON(item)
}

// DeleteCatalogItem removes a catalog service.
func (h *AdminHandler) DeleteCatalogItem(c fiber.Ctx) error {
	if err := h.repo.DeleteCatalogItem(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "delete failed"})
	}
	return c.JSON(fiber.Map{"deleted": true})
}

// GetSettings returns global app settings.
func (h *AdminHandler) GetSettings(c fiber.Ctx) error {
	s, err := h.repo.GetSettings()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "settings failed"})
	}
	return c.JSON(s)
}

// UpdateSettings patches global settings.
func (h *AdminHandler) UpdateSettings(c fiber.Ctx) error {
	var body model.AppSettings
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "bad body"})
	}
	if err := h.repo.UpdateSettings(&body); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "update failed"})
	}
	return c.JSON(body)
}

// broadcastTickInterval throttles outbound Telegram sends. ~28 msg/s sits
// under Telegram's 30 msg/s global ceiling; per-chat 1/s isn't a concern
// because we send to distinct chats.
const broadcastTickInterval = 36 * time.Millisecond

// Broadcast sends a message to all users.
//
// Single-flight guarantees:
//   - Cluster-wide Redis lock (broadcastAPILockKey) — a second concurrent
//     POST returns 409 instead of spawning a parallel goroutine that
//     would double-message every user.
//   - WaitGroup registration — graceful shutdown waits for an in-flight
//     broadcast (or its 24h ctx deadline) before closing DB/Redis pools.
//     Without this the worker would write into a closing connection
//     pool mid-batch.
//
// Both protections were missing from the previous implementation (audit
// #10); a double-click in the admin TMA would fan out two cluster-wide
// broadcasts, and a SIGTERM during a broadcast would leak the goroutine
// past the DB close.
func (h *AdminHandler) Broadcast(c fiber.Ctx) error {
	var body struct {
		TextRU   string `json:"text_ru"`
		TextEN   string `json:"text_en"`
		ImageURL string `json:"image_url"`
	}
	if err := c.Bind().JSON(&body); err != nil || (body.TextRU == "" && body.TextEN == "") {
		return c.Status(400).JSON(fiber.Map{"error": "text required"})
	}

	// Acquire the cluster-wide single-flight lock BEFORE doing any other
	// work. SetNX returns acquired=false if another instance / a
	// duplicate click is already running. Redis errors fail-open: log,
	// proceed — better to occasionally double-broadcast than to block
	// the admin entirely when Redis is degraded.
	lockAcquired := false
	if h.rdb != nil {
		acquired, err := h.rdb.SetNX(c.Context(), broadcastAPILockKey, "1", 24*time.Hour).Result()
		if err != nil {
			log.Printf("[broadcast] redis SetNX error: %v — proceeding without lock", err)
		} else if !acquired {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "broadcast_already_running",
			})
		} else {
			lockAcquired = true
		}
	}

	var count int64
	// Match the filter inside runBroadcast so the "recipients" number we
	// return to the admin isn't inflated by banned/deleted/blocked users
	// the actual run will skip.
	h.db.Model(&model.User{}).
		Where("is_banned = false AND deleted_at IS NULL AND is_active = true").
		Count(&count)

	if h.wg != nil {
		h.wg.Add(1)
	}
	go func() {
		if h.wg != nil {
			defer h.wg.Done()
		}
		// Release the lock on goroutine exit (success, error, OR panic
		// recovered by Supervise's outer frame). Uses a fresh ctx since
		// h.appCtx may be cancelled by the time we get here on shutdown.
		if lockAcquired {
			defer func() {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := h.rdb.Del(releaseCtx, broadcastAPILockKey).Err(); err != nil {
					log.Printf("[broadcast] lock release error: %v (TTL will reclaim)", err)
				}
			}()
		}
		workerutil.Supervise("broadcast", func() {
			h.runBroadcast(body.TextRU, body.TextEN, body.ImageURL)
		})
	}()

	return c.JSON(fiber.Map{
		"status":     "queued",
		"recipients": count,
	})
}

// broadcastAPIConcurrency caps in-flight sendOne calls. Same rationale
// as bot/broadcast.go broadcastConcurrency — see audit Tier-3 #6.
const broadcastAPIConcurrency = 8

// broadcastUserJob carries the per-recipient data the worker pool needs.
// Locale-resolved text is computed inside the producer (cheap) so the
// pool can stay agnostic to language fallback.
type broadcastUserJob struct {
	chatID int64
	text   string
}

// runBroadcast streams users in batches and sends Telegram messages while
// honouring the server's lifecycle context. Cancellation on SIGTERM is
// respected; a hung Telegram API call is bounded by a per-send timeout.
//
// Sends run through a worker pool gated by broadcastTickInterval — the
// previous sequential `ticker.Wait; sendOne` couldn't reach the rate
// cap on high-latency networks because each send blocked for its own
// round-trip. Mirrors the bot/broadcast.go fix.
func (h *AdminHandler) runBroadcast(textRU, textEN, imageURL string) {
	ctx, cancel := context.WithTimeout(h.appCtx, 24*time.Hour)
	defer cancel()

	ticker := time.NewTicker(broadcastTickInterval)
	defer ticker.Stop()

	var sent, failed int64

	jobCh := make(chan broadcastUserJob, broadcastAPIConcurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < broadcastAPIConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if h.sendOne(ctx, job.chatID, job.text, imageURL) {
					atomic.AddInt64(&sent, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}()
	}

	// Exclude banned, soft-deleted, and inactive (bot-blocked) users from
	// the broadcast roster. Mirrors the filter in bot/broadcast.go and
	// repository.CountBroadcastRecipients so the queued/recipients count
	// returned from the HTTP handler matches what runBroadcast actually
	// iterates.
	err := h.db.WithContext(ctx).
		Model(&model.User{}).
		Where("is_banned = false AND deleted_at IS NULL AND is_active = true").
		FindInBatches(&[]model.User{}, 500, func(tx *gorm.DB, _ int) error {
			users, ok := tx.Statement.Dest.(*[]model.User)
			if !ok {
				return errors.New("broadcast: unexpected batch dest type")
			}
			for _, u := range *users {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}

				msgText := textEN
				if u.Locale == "ru" && textRU != "" {
					msgText = textRU
				}
				if msgText == "" {
					msgText = textEN
				}
				if msgText == "" {
					// No text in either locale — skip rather than send
					// a blank message.
					continue
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				case jobCh <- broadcastUserJob{chatID: u.TelegramID, text: msgText}:
				}
			}
			return nil
		}).Error

	close(jobCh)
	wg.Wait()

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("[broadcast] batch iteration error: %v", err)
	}
	log.Printf("[broadcast] finished: %d sent, %d failed",
		atomic.LoadInt64(&sent), atomic.LoadInt64(&failed))
}

// sendOne performs a single Telegram send with a bounded per-call timeout
// and a retry_after-aware back-off on 429. Returns true on success.
//
// Behaviour on 429: parse the retry_after seconds the Telegram API embeds
// in the error message ("Too Many Requests: retry after 17") via
// workerutil.ParseRetryAfter, sleep that long, then retry — up to two
// retries. If retry_after is missing or unparseable, fall back to 1s.
func (h *AdminHandler) sendOne(parent context.Context, chatID int64, text, imageURL string) bool {
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		sendCtx, cancel := context.WithTimeout(parent, 10*time.Second)
		var err error
		if imageURL != "" {
			_, err = h.bot.SendPhoto(sendCtx, &bot.SendPhotoParams{
				ChatID:    chatID,
				Photo:     &models.InputFileString{Data: imageURL},
				Caption:   text,
				ParseMode: "Markdown",
			})
		} else {
			_, err = h.bot.SendMessage(sendCtx, &bot.SendMessageParams{
				ChatID:    chatID,
				Text:      text,
				ParseMode: "Markdown",
			})
		}
		cancel()

		if err == nil {
			return true
		}

		// Permanent per-recipient failures — bail without burning the
		// remaining attempts. Symmetric with bot/broadcast.go copyOne;
		// the previous version of this function retried 3 times against
		// users who had blocked the bot, wasting roughly 3× the API
		// budget on dead recipients in a typical batch.
		if workerutil.IsPermanentSendFailure(err) {
			log.Printf("[broadcast] skip chat %d: %v", chatID, err)
			return false
		}

		// 429 path: respect the retry_after hint when present, else fall
		// back to 1s. Don't sleep on the last attempt — it's pointless.
		if workerutil.IsRateLimit(err) && attempt < maxAttempts-1 {
			delay, ok := workerutil.ParseRetryAfter(err)
			if !ok {
				delay = time.Second
			}
			log.Printf("[broadcast] 429 for chat %d, sleeping %s (attempt %d/%d)",
				chatID, delay, attempt+1, maxAttempts)
			select {
			case <-parent.Done():
				return false
			case <-time.After(delay):
			}
			continue
		}

		log.Printf("[broadcast] failed to send to %d (attempt %d): %v",
			chatID, attempt+1, err)
		return false
	}
	return false
}

// ListCampaigns returns traffic campaign stats.
func (h *AdminHandler) ListCampaigns(c fiber.Ctx) error {
	items, err := h.repo.ListCampaigns()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "campaigns failed"})
	}
	return c.JSON(items)
}
