package handler

import (
	"fmt"
	"log"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
)

// PaymentHandler handles Premium payment endpoints.
type PaymentHandler struct {
	cfg *config.Config
	db  *gorm.DB
	bot *tgbot.Bot
}

func NewPaymentHandler(db *gorm.DB, cfg *config.Config, b *tgbot.Bot) *PaymentHandler {
	return &PaymentHandler{cfg: cfg, db: db, bot: b}
}

// CreateStarsInvoice generates a Telegram Stars payment link for the
// authenticated user. The price and copy are locale-split (ru/en) and
// read from AppSettings so the admin can adjust them in real-time from
// the in-bot panel.
//
// POST /api/v1/payments/stars
func (h *PaymentHandler) CreateStarsInvoice(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.bot == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "bot unavailable"})
	}

	// Read current pricing from the singleton AppSettings row.
	var settings model.AppSettings
	if err := h.db.FirstOrCreate(&settings, model.AppSettings{ID: 1}).Error; err != nil {
		log.Printf("[payment.stars] settings read error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "settings unavailable"})
	}

	// Locale-aware price and copy selection.
	locale := user.Locale
	if locale == "" {
		locale = "en"
	}

	var price int
	var title, description, label string

	if strings.HasPrefix(locale, "ru") {
		price = settings.PriceStarsRU
		title = "SubGuard Премиум"
		description = "Разблокировка всех премиум-функций"
		label = "Премиум"
	} else {
		price = settings.PriceStarsEN
		title = "SubGuard Premium"
		description = "Unlock all premium features"
		label = "Premium"
	}

	if price <= 0 {
		log.Printf("[payment.stars] invalid price=%d for locale=%s", price, locale)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid price configuration"})
	}

	// Generate the invoice link via Telegram Bot API.
	// ProviderToken MUST be empty string for Telegram Stars (XTR).
	invoiceURL, err := h.bot.CreateInvoiceLink(c.Context(), &tgbot.CreateInvoiceLinkParams{
		Title:         title,
		Description:   description,
		Payload:       fmt.Sprintf("premium_stars_%d", user.ID),
		ProviderToken: "",
		Currency:      "XTR",
		Prices: []models.LabeledPrice{
			{Label: label, Amount: price},
		},
	})
	if err != nil {
		log.Printf("[payment.stars] CreateInvoiceLink error for user=%d: %v", user.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invoice generation failed"})
	}

	return c.JSON(fiber.Map{"invoice_url": invoiceURL})
}
