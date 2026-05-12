package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/subguard/backend/internal/model"
	"gorm.io/gorm"
)

// contextKey is the key used to store the authenticated user in Fiber locals.
const contextKeyUser = "user"

// maxInitDataAge bounds how stale an initData payload can be before we reject
// it. Telegram refreshes initData on every mini-app open, so a short window
// keeps the replay surface minimal.
const maxInitDataAge = 5 * time.Minute

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
		// In production neither var should be set; if APP_ENV=test slips into
		// prod by mistake, requests without the shared secret still fall
		// through to the normal HMAC path below.
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

		// Upsert user into DB
		user := model.User{TelegramID: tgUser.ID}
		result := db.Where("telegram_id = ?", tgUser.ID).First(&user)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				user = model.User{
					TelegramID: tgUser.ID,
					FirstName:  tgUser.FirstName,
					LastName:   tgUser.LastName,
					Username:   tgUser.Username,
					PhotoURL:   tgUser.PhotoURL,
					Locale:     localeFromCode(tgUser.LanguageCode),
				}
				if err := db.Create(&user).Error; err != nil {
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
						"error": "failed to create user",
					})
				}
			} else {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "database error",
				})
			}
		} else {
			// Update profile fields that may have changed
			db.Model(&user).Updates(map[string]interface{}{
				"first_name": tgUser.FirstName,
				"last_name":  tgUser.LastName,
				"username":   tgUser.Username,
				"photo_url":  tgUser.PhotoURL,
			})
		}

		c.Locals(contextKeyUser, &user)
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
