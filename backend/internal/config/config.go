package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration parsed from environment variables.
type Config struct {
	// Telegram
	BotToken         string
	AdminTelegramIDs []int64
	WebhookSecret    string
	CryptoPayToken   string
	// CryptoPayAPIURL is the Crypto Pay API base, trailing slash
	// guaranteed by Load(). Defaults to the TESTNET endpoint so a
	// deploy that forgets to set it fails safe — no real funds move.
	// Production MUST set CRYPTO_PAY_API_URL=https://pay.crypt.bot/api/
	// explicitly (alongside a mainnet CRYPTO_PAY_TOKEN).
	CryptoPayAPIURL string

	// Database
	DatabaseURL string

	// Redis
	RedisURL string

	// App
	APIPort string
	BaseURL string

	// CurrencyAPIURL is the FULL URL of the latest-rates endpoint used by
	// the currency worker. The provider response must contain either a
	// "conversion_rates" (exchangerate-api.com v6) or "rates"
	// (open.er-api.com) field of {currency: rate-vs-base} pairs, where
	// rate-vs-base means "how many units of currency for 1 unit of
	// base". The base IS the URL's trailing segment — we currently
	// always request USD, so the URL must end "/latest/USD".
	//
	// Empty falls back to the free open.er-api.com endpoint so dev /
	// test / first-deploy works without any external account; prod
	// should set this to https://v6.exchangerate-api.com/v6/<KEY>/latest/USD
	// for the higher-quality, key-authenticated feed.
	CurrencyAPIURL string
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		BotToken:        os.Getenv("BOT_TOKEN"),
		WebhookSecret:   os.Getenv("WEBHOOK_SECRET"),
		CryptoPayToken:  os.Getenv("CRYPTO_PAY_TOKEN"),
		CryptoPayAPIURL: os.Getenv("CRYPTO_PAY_API_URL"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		RedisURL:        os.Getenv("REDIS_URL"),
		APIPort:         os.Getenv("API_PORT"),
		BaseURL:         os.Getenv("BASE_URL"),
		CurrencyAPIURL:  os.Getenv("CURRENCY_API_URL"),
	}
	if cfg.CurrencyAPIURL == "" {
		cfg.CurrencyAPIURL = "https://open.er-api.com/v6/latest/USD"
	}

	if cfg.BotToken == "" {
		return nil, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.APIPort == "" {
		cfg.APIPort = "3000"
	}
	if cfg.RedisURL == "" {
		cfg.RedisURL = "redis://redis:6379/0"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://app.subguard.click"
	}
	// Crypto Pay endpoint resolution:
	//   - token set + URL set     → use the explicit URL (the prod path).
	//   - token set + URL missing → fail fast. The previous default fell
	//     back to TESTNET, which silently routed mainnet-token invoice
	//     creates to the testnet endpoint, where they 401 — users saw 500
	//     and no operator could tell from logs that the URL was wrong.
	//   - no token                → keep the safe testnet default so dev /
	//     test builds with the feature disabled still parse a valid URL.
	// Either way we normalise to a trailing slash so callers can blindly
	// append the method name (cfg.CryptoPayAPIURL + "createInvoice").
	if cfg.CryptoPayToken != "" && cfg.CryptoPayAPIURL == "" {
		return nil, fmt.Errorf("CRYPTO_PAY_API_URL is required when CRYPTO_PAY_TOKEN is set " +
			"(e.g. https://pay.crypt.bot/api/ for mainnet)")
	}
	if cfg.CryptoPayAPIURL == "" {
		cfg.CryptoPayAPIURL = "https://testnet-pay.crypt.bot/api/"
	}
	if !strings.HasSuffix(cfg.CryptoPayAPIURL, "/") {
		cfg.CryptoPayAPIURL += "/"
	}

	// Refuse APP_ENV=test on what looks like a production deploy. The
	// auth middleware exposes a shared-secret bypass under APP_ENV=test —
	// safe in CI/dev, catastrophic if leaked to prod (anyone holding
	// TEST_AUTH_SECRET could impersonate any user). "Looks like prod" =
	// BaseURL host is not a loopback / *.local / *.internal address.
	// ALLOW_TEST_MODE_IN_PROD=1 is a deliberate override for the rare case
	// (e.g. running staging against a public URL) — required to be set
	// explicitly so accidental copy-paste of APP_ENV=test into prod env
	// fails loud at boot.
	if os.Getenv("APP_ENV") == "test" {
		if !isLocalBaseURL(cfg.BaseURL) && os.Getenv("ALLOW_TEST_MODE_IN_PROD") != "1" {
			return nil, fmt.Errorf("APP_ENV=test refused: BASE_URL=%q does not look like a local/staging host. "+
				"Set ALLOW_TEST_MODE_IN_PROD=1 to override (DANGEROUS — exposes the auth bypass)", cfg.BaseURL)
		}
	}

	// Parse comma-separated admin Telegram IDs
	if raw := os.Getenv("ADMIN_TELEGRAM_IDS"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid ADMIN_TELEGRAM_IDS value %q: %w", s, err)
			}
			cfg.AdminTelegramIDs = append(cfg.AdminTelegramIDs, id)
		}
	}

	return cfg, nil
}

// isLocalBaseURL reports whether the given URL points at a developer /
// CI host (loopback, *.local mDNS, *.internal). Used by Load() to gate
// the APP_ENV=test auth bypass — prod URLs return false and the boot
// fails loud.
func isLocalBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	}
	return strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".localhost")
}

// IsAdmin checks whether a given Telegram user ID is in the admin list.
func (c *Config) IsAdmin(telegramID int64) bool {
	for _, id := range c.AdminTelegramIDs {
		if id == telegramID {
			return true
		}
	}
	return false
}
