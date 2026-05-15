package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/subguard/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// contextKey is the key used to store the authenticated user in Fiber locals.
const contextKeyUser = "user"

// maxInitDataAge bounds how stale an initData payload can be before we reject
// it. Telegram does NOT refresh initData during a session — it stays valid
// from the time the user opened the mini-app — so this window must cover a
// typical session length. 1h matches Telegram's own recommendation and what
// most production TMAs use.
const maxInitDataAge = 60 * time.Minute

// TelegramUser is the user object embedded inside initData.
type TelegramUser struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
	PhotoURL     string `json:"photo_url,omitempty"`
}

// AuthMiddleware validates the Telegram initData HMAC-SHA256 signature,
// extracts the user, and upserts them into the database.
// The authenticated model.User is stored in c.Locals("user").
func AuthMiddleware(botToken string, db *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		raw := c.Get("X-Telegram-Init-Data")
		if raw == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing X-Telegram-Init-Data header",
			})
		}

		// ── Test-mode bypass ───────────────────────────────
		// Active ONLY when APP_ENV=test AND TEST_AUTH_SECRET is set to a long
		// random value (>=32 chars). The client must send:
		//   X-Telegram-Init-Data: test_user_<tgID>:<TEST_AUTH_SECRET>
		// Defence-in-depth: config.Load() refuses APP_ENV=test on a
		// non-local BaseURL unless ALLOW_TEST_MODE_IN_PROD=1, so this
		// branch can only be reached on dev/CI or with an explicit
		// override — accidental `APP_ENV=test` in prod env never starts
		// the binary at all.
		if os.Getenv("APP_ENV") == "test" {
			if secret := os.Getenv("TEST_AUTH_SECRET"); len(secret) >= 32 && strings.HasPrefix(raw, "test_user_") {
				body := strings.TrimPrefix(raw, "test_user_")
				parts := strings.SplitN(body, ":", 2)
				if len(parts) == 2 && hmac.Equal([]byte(parts[1]), []byte(secret)) {
					if tgID, parseErr := strconv.ParseInt(parts[0], 10, 64); parseErr == nil {
						var user model.User
						if err := db.Where("telegram_id = ?", tgID).First(&user).Error; err == nil {
							c.Locals(contextKeyUser, &user)
							return c.Next()
						}
					}
				}
			}
		}

		tgUser, err := validateInitData(raw, botToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Atomic upsert — INSERT ... ON CONFLICT (telegram_id) DO UPDATE ...
		// RETURNING * in a single round-trip. Replaces the previous
		// SELECT → branch-on-NotFound → INSERT-with-race-fallback dance:
		//
		//   - No race window: two concurrent /me hits for the same new
		//     TelegramID both attempt INSERT, one wins, the loser's INSERT
		//     becomes an UPDATE — no unique-violation, no fallback re-read.
		//
		//   - Profile fields use COALESCE(NULLIF(excluded.X, ''), users.X)
		//     so a Telegram update that omits a field (Telegram occasionally
		//     drops last_name or photo_url for users with privacy locks)
		//     does NOT clobber what we have on file. Matches the prior
		//     `if tgUser.X != ""` guards.
		//
		//   - Locale is set on INSERT (from the initData language_code) and
		//     intentionally NOT in DoUpdates — the user can override it via
		//     PATCH /me or the bot's lang: picker, and we don't want every
		//     /me to revert that to the Telegram client default.
		//
		//   - deleted_at = NULL on conflict revives a soft-deleted account
		//     when the user reauthenticates. The DB-side UNIQUE index on
		//     telegram_id ignores deleted_at, so the conflict triggers
		//     against the soft-deleted row exactly as desired.
		var user model.User
		user = model.User{
			TelegramID: tgUser.ID,
			FirstName:  tgUser.FirstName,
			LastName:   tgUser.LastName,
			Username:   tgUser.Username,
			PhotoURL:   tgUser.PhotoURL,
			Locale:     localeFromCode(tgUser.LanguageCode),
		}
		if upsertErr := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "telegram_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"first_name": gorm.Expr("COALESCE(NULLIF(EXCLUDED.first_name, ''), users.first_name)"),
				"last_name":  gorm.Expr("COALESCE(NULLIF(EXCLUDED.last_name, ''), users.last_name)"),
				"username":   gorm.Expr("COALESCE(NULLIF(EXCLUDED.username, ''), users.username)"),
				"photo_url":  gorm.Expr("COALESCE(NULLIF(EXCLUDED.photo_url, ''), users.photo_url)"),
				"deleted_at": gorm.Expr("NULL"),
				"updated_at": gorm.Expr("NOW()"),
			}),
		}).Create(&user).Error; upsertErr != nil {
			log.Printf("[auth] upsert tg=%d: %v", tgUser.ID, upsertErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "database error",
			})
		}

		c.Locals(contextKeyUser, &user)

		// Block banned users from accessing the API.
		if user.IsBanned {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "account_banned",
			})
		}

		return c.Next()
	}
}

// UserFromCtx retrieves the authenticated user from Fiber context.
func UserFromCtx(c fiber.Ctx) *model.User {
	u, ok := c.Locals(contextKeyUser).(*model.User)
	if !ok {
		return nil
	}
	return u
}

// validateInitData verifies the HMAC-SHA256 signature of Telegram initData.
// See: https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
func validateInitData(initData string, botToken string) (*TelegramUser, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, fmt.Errorf("invalid initData format")
	}

	hash := values.Get("hash")
	if hash == "" {
		return nil, fmt.Errorf("hash is missing")
	}

	// Verify auth_date is not too old (5 minute window — Telegram refreshes
	// initData on every mini-app open, so 5min is plenty and limits replay).
	authDateStr := values.Get("auth_date")
	if authDateStr == "" {
		return nil, fmt.Errorf("auth_date is missing")
	}
	authDate, err := strconv.ParseInt(authDateStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid auth_date")
	}
	if time.Since(time.Unix(authDate, 0)) > maxInitDataAge {
		return nil, fmt.Errorf("initData expired")
	}

	// Build data_check_string: sorted key=value pairs, excluding "hash"
	var pairs []string
	for k, v := range values {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, k+"="+v[0])
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	// HMAC: secret_key = HMAC-SHA256("WebAppData", bot_token)
	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	secretKey := secretMAC.Sum(nil)

	// HMAC: result = HMAC-SHA256(data_check_string, secret_key)
	dataMAC := hmac.New(sha256.New, secretKey)
	dataMAC.Write([]byte(dataCheckString))
	computed := hex.EncodeToString(dataMAC.Sum(nil))

	if !hmac.Equal([]byte(computed), []byte(hash)) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Parse user JSON
	userJSON := values.Get("user")
	if userJSON == "" {
		return nil, fmt.Errorf("user field is missing")
	}

	var tgUser TelegramUser
	if err := json.Unmarshal([]byte(userJSON), &tgUser); err != nil {
		return nil, fmt.Errorf("invalid user JSON: %w", err)
	}

	return &tgUser, nil
}

func localeFromCode(code string) string {
	if strings.HasPrefix(code, "ru") {
		return "ru"
	}
	return "en"
}
