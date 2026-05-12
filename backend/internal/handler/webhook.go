package handler

import (
	"crypto/hmac"
	"encoding/json"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofiber/fiber/v3"
)

// WebhookHandler receives Telegram Bot updates via webhook.
//
// The webhookSecret MUST be non-empty in production; main.go fails fast at
// startup if it is missing, so by the time a request reaches Handle() the
// secret is guaranteed to be set.
type WebhookHandler struct {
	bot           *bot.Bot
	webhookSecret string
}

func NewWebhookHandler(b *bot.Bot, secret string) *WebhookHandler {
	return &WebhookHandler{bot: b, webhookSecret: secret}
}

// Handle processes incoming Telegram webhook updates.
// POST /webhook
func (h *WebhookHandler) Handle(c fiber.Ctx) error {
	// Constant-time compare on the secret token shared with Telegram.
	token := c.Get("X-Telegram-Bot-Api-Secret-Token")
	if !hmac.Equal([]byte(token), []byte(h.webhookSecret)) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid secret"})
	}

	var update models.Update
	if err := json.Unmarshal(c.Body(), &update); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid update"})
	}

	// Process update via the bot's registered handlers
	h.bot.ProcessUpdate(c.Context(), &update)

	return c.SendStatus(fiber.StatusOK)
}
