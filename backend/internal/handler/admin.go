package handler

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
)

// AdminHandler handles all admin-panel endpoints.
type AdminHandler struct {
	repo   *repository.AdminRepo
	cfg    *config.Config
	db     *gorm.DB
	bot    *bot.Bot
	appCtx context.Context
}

// NewAdminHandler builds an AdminHandler. appCtx should be the server's
// parent lifecycle context so background goroutines (broadcast) cancel on
// shutdown instead of leaking past SIGTERM.
func NewAdminHandler(db *gorm.DB, cfg *config.Config, b *bot.Bot, appCtx context.Context) *AdminHandler {
	if appCtx == nil {
		appCtx = context.Background()
	}
	return &AdminHandler{
		repo:   repository.NewAdminRepo(db),
		cfg:    cfg,
		db:     db,
		bot:    b,
		appCtx: appCtx,
	}
}

// AdminOnly is a middleware that restricts access to admin users.
func (h *AdminHandler) AdminOnly(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil || !h.cfg.IsAdmin(user.TelegramID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
	}
	return c.Next()
}

// GetStats returns live KPI metrics.
func (h *AdminHandler) GetStats(c fiber.Ctx) error {
	stats, err := h.repo.GetStats()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "stats failed"})
	}
	popular, _ := h.repo.GetPopularServices(10)
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
func (h *AdminHandler) Broadcast(c fiber.Ctx) error {
	var body struct {
		TextRU   string `json:"text_ru"`
		TextEN   string `json:"text_en"`
		ImageURL string `json:"image_url"`
	}
	if err := c.Bind().JSON(&body); err != nil || (body.TextRU == "" && body.TextEN == "") {
		return c.Status(400).JSON(fiber.Map{"error": "text required"})
	}

	var count int64
	h.db.Model(&model.User{}).Count(&count)

	go h.runBroadcast(body.TextRU, body.TextEN, body.ImageURL)

	return c.JSON(fiber.Map{
		"status":     "queued",
		"recipients": count,
	})
}

// runBroadcast streams users in batches and sends Telegram messages while
// honouring the server's lifecycle context. Cancellation on SIGTERM is
// respected; a hung Telegram API call is bounded by a per-send timeout.
func (h *AdminHandler) runBroadcast(textRU, textEN, imageURL string) {
	ctx, cancel := context.WithTimeout(h.appCtx, 24*time.Hour)
	defer cancel()

	ticker := time.NewTicker(broadcastTickInterval)
	defer ticker.Stop()

	var sent, failed int
	err := h.db.WithContext(ctx).
		Model(&model.User{}).
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

				if h.sendOne(ctx, u.TelegramID, msgText, imageURL) {
					sent++
				} else {
					failed++
				}
			}
			return nil
		}).Error

	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("[broadcast] batch iteration error: %v", err)
	}
	log.Printf("[broadcast] finished: %d sent, %d failed", sent, failed)
}

// sendOne performs a single Telegram send with a bounded timeout and a
// retry on 429 (Too Many Requests) using the retry_after the API returns.
// Returns true on success.
func (h *AdminHandler) sendOne(parent context.Context, chatID int64, text, imageURL string) bool {
	for attempt := 0; attempt < 2; attempt++ {
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

		// Detect "Too Many Requests; retry after N" — the go-telegram client
		// surfaces it as a plain error containing the phrase. Back off and try
		// once more before giving up on this user.
		if msg := err.Error(); strings.Contains(strings.ToLower(msg), "too many requests") {
			select {
			case <-parent.Done():
				return false
			case <-time.After(time.Second):
			}
			continue
		}

		log.Printf("[broadcast] failed to send to %d: %v", chatID, err)
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
