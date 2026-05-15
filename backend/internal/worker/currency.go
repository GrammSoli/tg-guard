package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// CurrencyWorker fetches exchange rates daily and caches them in Redis.
//
// Storage shape (changed from individual SET keys to a single HASH):
//
//	HSET subguard:fx:USD <currency> <rate vs USD>
//	EXPIRE subguard:fx:USD 25h
//
// Where rate vs USD means "how many <currency> you get for 1 USD" — the
// natural form returned by exchangerate-api.com / open.er-api.com on the
// USD-base endpoint. The /api/v1/fx handler reads the hash with one
// HGETALL and forwards the whole map to the frontend, where
// convertCurrency cross-converts via USD.
type CurrencyWorker struct {
	rdb    *redis.Client
	client *http.Client
	apiURL string
}

// currencyCacheTTL gives the daily refresh a 1h overlap so the previous
// snapshot is still readable while the new one is being written.
const currencyCacheTTL = 25 * time.Hour

// currencyHashKey is the Redis hash holding "1 USD → X foreign" rates.
// One key for the whole cache; HGETALL on the read side is a single
// round-trip regardless of how many currencies we cover.
const currencyHashKey = "subguard:fx:USD"

func NewCurrencyWorker(rdb *redis.Client, apiURL string) *CurrencyWorker {
	return &CurrencyWorker{
		rdb:    rdb,
		client: &http.Client{Timeout: 10 * time.Second},
		apiURL: apiURL,
	}
}

// Start launches the currency update loop. Runs immediately then every 24h.
func (w *CurrencyWorker) Start(ctx context.Context) {
	log.Printf("[currency-worker] starting (api=%s)", w.apiURL)
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

// ratesResponse covers both provider shapes:
//   - open.er-api.com: { "result": "success", "rates": {...} }
//   - exchangerate-api.com v6: { "result": "success", "conversion_rates": {...} }
//
// We accept whichever field is populated. "result" is checked for the
// string "success" before we trust the body — both providers use it.
type ratesResponse struct {
	Result          string             `json:"result"`
	Rates           map[string]float64 `json:"rates"`
	ConversionRates map[string]float64 `json:"conversion_rates"`
}

func (w *CurrencyWorker) update(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.apiURL, nil)
	if err != nil {
		log.Printf("[currency-worker] build request error: %v", err)
		return
	}
	resp, err := w.client.Do(req)
	if err != nil {
		log.Printf("[currency-worker] fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[currency-worker] unexpected HTTP status %d from %s", resp.StatusCode, w.apiURL)
		return
	}

	var data ratesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[currency-worker] decode error: %v", err)
		return
	}
	if data.Result != "success" {
		log.Printf("[currency-worker] provider returned non-success result: %q", data.Result)
		return
	}

	rates := data.ConversionRates
	if len(rates) == 0 {
		rates = data.Rates
	}
	if len(rates) == 0 {
		log.Printf("[currency-worker] empty rates payload — provider returned neither `conversion_rates` nor `rates`")
		return
	}

	// HSET expects []interface{} pairs (field, value, field, value, ...).
	// Format floats as strings explicitly so a future-Go change to
	// reflection-based encoding doesn't drop precision silently. Use 'g'
	// formatting so common rates (e.g. 1, 0.92) stay short and large ones
	// (KZT ≈ 500) keep significant digits.
	pairs := make([]interface{}, 0, len(rates)*2)
	for cur, rate := range rates {
		pairs = append(pairs, cur, strconv.FormatFloat(rate, 'g', -1, 64))
	}

	// Pipeline HSET + EXPIRE so Redis sees a single network round-trip and
	// the TTL is applied atomically with the data write. Replaces the
	// previous N-individual-SETs-per-currency pattern, which on ~170
	// currencies × 5 bases meant 850 round-trips per refresh.
	pipe := w.rdb.Pipeline()
	pipe.HSet(ctx, currencyHashKey, pairs...)
	pipe.Expire(ctx, currencyHashKey, currencyCacheTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[currency-worker] redis pipeline error: %v", err)
		return
	}

	log.Printf("[currency-worker] cached %d USD rates", len(rates))
}

// fetchRedisHash is a small helper for tests + handler — reads the hash
// with one round-trip and returns a parsed float map.
func FetchRates(ctx context.Context, rdb *redis.Client) (map[string]float64, error) {
	raw, err := rdb.HGetAll(ctx, currencyHashKey).Result()
	if err != nil {
		return nil, fmt.Errorf("HGETALL %s: %w", currencyHashKey, err)
	}
	out := make(map[string]float64, len(raw))
	for k, v := range raw {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			// Skip malformed value rather than failing the whole read —
			// one bad cell shouldn't blank-out the whole rates response.
			continue
		}
		out[k] = f
	}
	return out, nil
}
