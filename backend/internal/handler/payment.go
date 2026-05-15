package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
)

// PaymentHandler handles Premium payment endpoints (Stars + Crypto Pay).
type PaymentHandler struct {
	cfg *config.Config
	db  *gorm.DB
	bot *tgbot.Bot
}

func NewPaymentHandler(db *gorm.DB, cfg *config.Config, b *tgbot.Bot) *PaymentHandler {
	return &PaymentHandler{cfg: cfg, db: db, bot: b}
}

// ── Stars ───────────────────────────────────────────────────────────

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

	var settings model.AppSettings
	if err := h.db.FirstOrCreate(&settings, model.AppSettings{ID: 1}).Error; err != nil {
		log.Printf("[payment.stars] settings read error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "settings unavailable"})
	}

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

// ── Crypto Pay (@CryptoBot) ─────────────────────────────────────────
//
// The API base lives in config (cfg.CryptoPayAPIURL, trailing slash
// guaranteed) so we can point at the testnet or mainnet endpoint via
// the CRYPTO_PAY_API_URL env var without a rebuild.

type createInvoiceRequest struct {
	CurrencyType   string `json:"currency_type"`
	Fiat           string `json:"fiat"`
	Amount         string `json:"amount"`
	Description    string `json:"description"`
	Payload        string `json:"payload"`
	AllowComments  bool   `json:"allow_comments"`
	AllowAnonymous bool   `json:"allow_anonymous"`
}

type createInvoiceResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		InvoiceID int64  `json:"invoice_id"`
		PayURL    string `json:"pay_url"`
	} `json:"result"`
}

// CreateCryptoInvoice generates a Crypto Pay (fiat/USD) payment link.
//
// POST /api/v1/payments/crypto
func (h *PaymentHandler) CreateCryptoInvoice(c fiber.Ctx) error {
	user := middleware.UserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	if h.cfg.CryptoPayToken == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "crypto payments not configured"})
	}

	var settings model.AppSettings
	if err := h.db.FirstOrCreate(&settings, model.AppSettings{ID: 1}).Error; err != nil {
		log.Printf("[payment.crypto] settings read error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "settings unavailable"})
	}

	locale := user.Locale
	if locale == "" {
		locale = "en"
	}

	var price int
	var description string
	if strings.HasPrefix(locale, "ru") {
		price = settings.PriceCryptoUsdRU
		description = "SubGuard Премиум"
	} else {
		price = settings.PriceCryptoUsdEN
		description = "SubGuard Premium"
	}

	if price <= 0 {
		log.Printf("[payment.crypto] invalid price=%d for locale=%s", price, locale)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid price configuration"})
	}

	reqBody := createInvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "USD",
		Amount:         strconv.Itoa(price),
		Description:    description,
		Payload:        fmt.Sprintf("premium_crypto_%d", user.ID),
		AllowComments:  false,
		AllowAnonymous: false,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(c.Context(), http.MethodPost,
		h.cfg.CryptoPayAPIURL+"createInvoice", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("[payment.crypto] request build error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "request build failed"})
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Crypto-Pay-API-Token", h.cfg.CryptoPayToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[payment.crypto] API call error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto api unavailable"})
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[payment.crypto] API response status=%d body=%s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto invoice failed"})
	}

	var invoiceResp createInvoiceResponse
	if err := json.Unmarshal(respBody, &invoiceResp); err != nil || !invoiceResp.OK {
		log.Printf("[payment.crypto] parse error or not ok: %v, ok=%v", err, invoiceResp.OK)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "crypto invoice parse failed"})
	}

	return c.JSON(fiber.Map{"invoice_url": invoiceResp.Result.PayURL})
}

// ── Crypto Pay Webhook ──────────────────────────────────────────────

type cryptoWebhookUpdate struct {
	UpdateType  string          `json:"update_type"`
	UpdateID    int64           `json:"update_id"`
	RequestDate string          `json:"request_date"`
	Payload     json.RawMessage `json:"payload"`
}

type cryptoInvoice struct {
	InvoiceID int64  `json:"invoice_id"`
	Status    string `json:"status"`
	Amount    string `json:"amount"`
	Fiat      string `json:"fiat"`
	PaidAt    string `json:"paid_at"`
	Payload   string `json:"payload"`
}

// HandleCryptoWebhook processes payment notifications from Crypto Pay.
// This endpoint is PUBLIC — no AuthMiddleware, no MaintenanceGuard.
// Security is enforced via HMAC-SHA256 signature verification.
//
// POST /webhook/crypto
func (h *PaymentHandler) HandleCryptoWebhook(c fiber.Ctx) error {
	if h.cfg.CryptoPayToken == "" {
		return c.Status(fiber.StatusServiceUnavailable).SendString("not configured")
	}

	rawBody := c.Body()
	signature := c.Get("crypto-pay-api-signature")

	// Signature: HMAC-SHA256(SHA256(token), body)
	tokenHash := sha256.Sum256([]byte(h.cfg.CryptoPayToken))
	mac := hmac.New(sha256.New, tokenHash[:])
	mac.Write(rawBody)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		log.Printf("❌ [crypto/webhook] Invalid signature. Got=%q", signature)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	var update cryptoWebhookUpdate
	if err := json.Unmarshal(rawBody, &update); err != nil {
		log.Printf("❌ [crypto/webhook] JSON parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad json"})
	}

	log.Printf("📥 [crypto/webhook] update_type=%s update_id=%d", update.UpdateType, update.UpdateID)

	if update.UpdateType != "invoice_paid" {
		return c.SendStatus(fiber.StatusOK)
	}

	var invoice cryptoInvoice
	if err := json.Unmarshal(update.Payload, &invoice); err != nil {
		log.Printf("❌ [crypto/webhook] invoice parse error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "bad invoice"})
	}

	log.Printf("📥 [crypto/webhook] invoice_id=%d status=%s payload=%q amount=%s %s",
		invoice.InvoiceID, invoice.Status, invoice.Payload, invoice.Amount, invoice.Fiat)

	// Safe payload parsing: "premium_crypto_<userID>"
	if !strings.HasPrefix(invoice.Payload, "premium_crypto_") {
		log.Printf("❌ [crypto/webhook] unknown payload=%q, ignoring", invoice.Payload)
		return c.SendStatus(fiber.StatusOK)
	}
	parts := strings.Split(invoice.Payload, "_")
	if len(parts) != 3 {
		log.Printf("❌ [crypto/webhook] malformed payload=%q", invoice.Payload)
		return c.SendStatus(fiber.StatusOK)
	}
	userID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		log.Printf("❌ [crypto/webhook] invalid user ID: %v", err)
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("✅ [crypto/webhook] Parsed UserID: %d", userID)

	// Idempotency: "crypto_<invoice_id>" in the shared charge ID column
	chargeID := fmt.Sprintf("crypto_%d", invoice.InvoiceID)
	var existing model.Donation
	if err := h.db.Where("telegram_payment_charge_id = ?", chargeID).First(&existing).Error; err == nil {
		log.Printf("⚠️ [crypto/webhook] Duplicate invoice_id=%d — skipping", invoice.InvoiceID)
		return c.SendStatus(fiber.StatusOK)
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", uint(userID)).Error; err != nil {
		log.Printf("❌ [crypto/webhook] user=%d not found: %v", userID, err)
		return c.SendStatus(fiber.StatusOK)
	}
	log.Printf("👤 [crypto/webhook] Found user: ID=%d, TelegramID=%d, IsDonator=%v",
		user.ID, user.TelegramID, user.IsDonator)

	amountFloat, _ := strconv.ParseFloat(invoice.Amount, 64)
	amountCents := int(amountFloat * 100)

	log.Printf("💽 [crypto/webhook] Activating premium for user %d", user.ID)
	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).
			Where("id = ?", user.ID).
			Update("is_donator", true).Error; err != nil {
			return fmt.Errorf("set is_donator: %w", err)
		}
		donation := model.Donation{
			UserID:                  user.ID,
			TelegramID:              user.TelegramID,
			TelegramPaymentChargeID: chargeID,
			Amount:                  amountCents,
		}
		if err := tx.Create(&donation).Error; err != nil {
			return fmt.Errorf("create donation: %w", err)
		}
		return nil
	})
	if txErr != nil {
		log.Printf("❌ [crypto/webhook] tx error for user=%d: %v", user.ID, txErr)
		return c.Status(fiber.StatusInternalServerError).SendString("tx error")
	}
	log.Printf("🟢 [crypto/webhook] Premium activated for user=%d, invoice=%d", user.ID, invoice.InvoiceID)

	// Localized congratulation via Telegram
	if h.bot != nil {
		locale := user.Locale
		var text string
		if strings.HasPrefix(locale, "ru") {
			text = "🎉 <b>Спасибо за покупку!</b>\n\nPremium успешно активирован. Вернитесь в приложение, чтобы пользоваться всеми функциями!"
		} else {
			text = "🎉 <b>Thank you for your purchase!</b>\n\nPremium is activated. Return to the app to enjoy all features!"
		}

		log.Printf("✉️ [crypto/webhook] Sending congratulation to TG=%d", user.TelegramID)
		if _, err := h.bot.SendMessage(c.Context(), &tgbot.SendMessageParams{
			ChatID:    user.TelegramID,
			Text:      text,
			ParseMode: "HTML",
		}); err != nil {
			log.Printf("❌ [crypto/webhook] send error: %v", err)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
