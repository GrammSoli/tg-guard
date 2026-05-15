package config

import (
	"fmt"
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
	// Default to the Crypto Pay TESTNET endpoint. Normalise to a single
	// trailing slash either way, so callers can blindly append the
	// method name (cfg.CryptoPayAPIURL + "createInvoice").
	if cfg.CryptoPayAPIURL == "" {
		cfg.CryptoPayAPIURL = "https://testnet-pay.crypt.bot/api/"
	}
	if !strings.HasSuffix(cfg.CryptoPayAPIURL, "/") {
		cfg.CryptoPayAPIURL += "/"
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

// IsAdmin checks whether a given Telegram user ID is in the admin list.
func (c *Config) IsAdmin(telegramID int64) bool {
	for _, id := range c.AdminTelegramIDs {
		if id == telegramID {
			return true
		}
	}
	return false
}
