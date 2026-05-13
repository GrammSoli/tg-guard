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

// currencyCacheTTL gives the daily refresh a 1h overlap so the previous
// snapshot is still readable while the new one is being written.
const currencyCacheTTL = 25 * time.Hour

// currencyKey assembles an app-namespaced Redis key. The "subguard:fx:"
// prefix keeps us out of the way if this Redis is ever shared with
// another service.
func currencyKey(base, target string) string {
	return fmt.Sprintf("subguard:fx:%s:%s", base, target)
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

		// Store each rate in Redis with 25h TTL. We accumulate failures
		// rather than aborting the whole base — a single transient Redis
		// write error shouldn't drop the other rates for this base.
		var failed int
		for target, rate := range data.Rates {
			if err := w.rdb.Set(ctx, currencyKey(base, target), rate, currencyCacheTTL).Err(); err != nil {
				failed++
				if failed <= 3 {
					log.Printf("[currency-worker] redis SET %s->%s error: %v", base, target, err)
				}
			}
		}
		if failed > 0 {
			log.Printf("[currency-worker] base=%s: %d of %d rates failed to cache",
				base, failed, len(data.Rates))
		} else {
			log.Printf("[currency-worker] updated %d rates for %s", len(data.Rates), base)
		}
	}
}
