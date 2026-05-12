package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// CurrencyWorker fetches exchange rates daily and caches them in Redis.
type CurrencyWorker struct {
	rdb    *redis.Client
	client *http.Client
}

func NewCurrencyWorker(rdb *redis.Client) *CurrencyWorker {
	return &CurrencyWorker{
		rdb:    rdb,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Start launches the currency update loop. Runs immediately then every 24h.
func (w *CurrencyWorker) Start(ctx context.Context) {
	log.Println("[currency-worker] starting")
	w.update(ctx) // initial fetch

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[currency-worker] stopped")
			return
		case <-ticker.C:
			w.update(ctx)
		}
	}
}

type ratesResponse struct {
	Result string             `json:"result"`
	Rates  map[string]float64 `json:"rates"`
}

func (w *CurrencyWorker) update(ctx context.Context) {
	bases := []string{"USD", "EUR", "GBP", "RUB", "KZT"}
	for _, base := range bases {
		select {
		case <-ctx.Done():
			return
		default:
		}
		url := fmt.Sprintf("https://open.er-api.com/v6/latest/%s", base)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			log.Printf("[currency-worker] build request %s error: %v", base, err)
			continue
		}
		resp, err := w.client.Do(req)
		if err != nil {
			log.Printf("[currency-worker] fetch %s error: %v", base, err)
			continue
		}

		var data ratesResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			resp.Body.Close()
			log.Printf("[currency-worker] decode %s error: %v", base, err)
			continue
		}
		resp.Body.Close()

		if data.Result != "success" {
			log.Printf("[currency-worker] %s returned status: %s", base, data.Result)
			continue
		}

		// Store each rate in Redis with 25h TTL
		for target, rate := range data.Rates {
			key := fmt.Sprintf("currency:%s:%s", base, target)
			w.rdb.Set(ctx, key, rate, 25*time.Hour)
		}
		log.Printf("[currency-worker] updated %d rates for %s", len(data.Rates), base)
	}
}
