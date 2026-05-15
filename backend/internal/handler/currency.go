package handler

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"

	"github.com/subguard/backend/internal/worker"
)

// CurrencyHandler serves the cached FX rates the currency worker writes
// to Redis. The frontend reads this on boot to drive convertCurrency()
// across the whole app — without it the UI falls back to a hard-coded
// 2025-mid snapshot that drifts roughly 10-20% per year.
type CurrencyHandler struct {
	rdb *redis.Client
}

func NewCurrencyHandler(rdb *redis.Client) *CurrencyHandler {
	return &CurrencyHandler{rdb: rdb}
}

// GetRates returns the latest USD-base rates from Redis.
//
//	{
//	  "base": "USD",
//	  "rates": { "USD": 1, "EUR": 0.92, "RUB": 91.5, ... }
//	}
//
// All rates are "1 USD = X foreign units" — the natural shape of every
// modern FX provider's /latest/USD endpoint, and the shape the frontend's
// cross-conversion math expects.
//
// On cache miss (worker hasn't populated yet or Redis is degraded) we
// return an empty rates map with status 200, NOT a 5xx — the frontend
// silently falls back to its static table and the dashboard keeps
// rendering. Returning 500 here would cascade into a paywall/stats/etc
// outage via /api/v1/config's loader on cold start.
//
// GET /api/v1/fx
func (h *CurrencyHandler) GetRates(c fiber.Ctx) error {
	rates, err := worker.FetchRates(c.Context(), h.rdb)
	if err != nil {
		log.Printf("[currency.GetRates] redis error: %v", err)
		// Degrade gracefully — empty map, frontend falls back.
		return c.JSON(fiber.Map{"base": "USD", "rates": map[string]float64{}})
	}
	return c.JSON(fiber.Map{"base": "USD", "rates": rates})
}
