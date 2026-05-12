package handler

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

// CurrencyHandler provides currency rate lookups from Redis cache.
type CurrencyHandler struct {
	rdb *redis.Client
}

func NewCurrencyHandler(rdb *redis.Client) *CurrencyHandler {
	return &CurrencyHandler{rdb: rdb}
}

// GetRate returns the cached exchange rate between two currencies.
func (h *CurrencyHandler) GetRate(from, to string) (float64, error) {
	key := fmt.Sprintf("currency:%s:%s", from, to)
	val, err := h.rdb.Get(context.Background(), key).Float64()
	if err != nil {
		log.Printf("[currency] rate %s->%s not in cache, falling back to 1.0", from, to)
		return 1.0, nil
	}
	return val, nil
}
