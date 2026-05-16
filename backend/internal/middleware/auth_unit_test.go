package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

const testBotToken = "1234567890:TEST_TOKEN_FOR_UNIT_TESTS_ONLY"

// signInitData builds a valid Telegram WebApp initData payload signed with
// botToken. Mirrors the algorithm in validateInitData so the two can be
// kept in lockstep; if Telegram ever changes the signature scheme, only
// the production code path moves, the helper tests still hit the same
// derivation and a real failure surfaces immediately.
func signInitData(t *testing.T, botToken string, fields map[string]string) string {
	t.Helper()
	values := url.Values{}
	for k, v := range fields {
		values.Set(k, v)
	}

	var pairs []string
	for k, vs := range values {
		if k == "hash" {
			continue
		}
		pairs = append(pairs, k+"="+vs[0])
	}
	sort.Strings(pairs)
	dataCheckString := strings.Join(pairs, "\n")

	secretMAC := hmac.New(sha256.New, []byte("WebAppData"))
	secretMAC.Write([]byte(botToken))
	secretKey := secretMAC.Sum(nil)

	dataMAC := hmac.New(sha256.New, secretKey)
	dataMAC.Write([]byte(dataCheckString))
	hash := hex.EncodeToString(dataMAC.Sum(nil))

	values.Set("hash", hash)
	return values.Encode()
}

func validUserJSON() string {
	return `{"id":12345,"first_name":"Test","last_name":"User","username":"testuser","language_code":"en"}`
}

func TestValidateInitData_HappyPath(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      validUserJSON(),
	})

	u, err := validateInitData(initData, testBotToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != 12345 {
		t.Errorf("ID = %d, want 12345", u.ID)
	}
	if u.Username != "testuser" {
		t.Errorf("Username = %q, want testuser", u.Username)
	}
}

func TestValidateInitData_InvalidSignature(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      validUserJSON(),
	})
	// Verify with a DIFFERENT token — signature must be rejected.
	_, err := validateInitData(initData, "wrong_token")
	if err == nil {
		t.Fatal("expected signature error, got nil")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature error, got: %v", err)
	}
}

func TestValidateInitData_MissingHash(t *testing.T) {
	// Hand-craft initData without the hash field — should fail.
	values := url.Values{}
	values.Set("auth_date", fmt.Sprintf("%d", time.Now().Unix()))
	values.Set("user", validUserJSON())
	_, err := validateInitData(values.Encode(), testBotToken)
	if err == nil {
		t.Fatal("expected error for missing hash")
	}
	if !strings.Contains(err.Error(), "hash") {
		t.Errorf("expected hash error, got: %v", err)
	}
}

func TestValidateInitData_MissingAuthDate(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"user": validUserJSON(),
	})
	_, err := validateInitData(initData, testBotToken)
	if err == nil {
		t.Fatal("expected error for missing auth_date")
	}
	if !strings.Contains(err.Error(), "auth_date") {
		t.Errorf("expected auth_date error, got: %v", err)
	}
}

func TestValidateInitData_InvalidAuthDate(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": "not-a-number",
		"user":      validUserJSON(),
	})
	_, err := validateInitData(initData, testBotToken)
	if err == nil {
		t.Fatal("expected error for non-numeric auth_date")
	}
}

func TestValidateInitData_ExpiredAuthDate(t *testing.T) {
	// 2 hours ago — past the 60-minute window.
	old := time.Now().Add(-2 * time.Hour).Unix()
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", old),
		"user":      validUserJSON(),
	})
	_, err := validateInitData(initData, testBotToken)
	if err == nil {
		t.Fatal("expected expiry error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected expired error, got: %v", err)
	}
}

func TestValidateInitData_MissingUser(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
	})
	_, err := validateInitData(initData, testBotToken)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
	if !strings.Contains(err.Error(), "user") {
		t.Errorf("expected user error, got: %v", err)
	}
}

func TestValidateInitData_InvalidUserJSON(t *testing.T) {
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      "{not_valid_json",
	})
	_, err := validateInitData(initData, testBotToken)
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
}

func TestValidateInitData_EmptyString(t *testing.T) {
	// url.ParseQuery accepts the empty string and returns an empty
	// url.Values map without error, so the first failure point is the
	// missing-hash check.
	_, err := validateInitData("", testBotToken)
	if err == nil {
		t.Fatal("expected error for empty initData")
	}
}

func TestValidateInitData_AuthDateAtBoundary(t *testing.T) {
	// Exactly at the edge of maxInitDataAge — should still be accepted.
	// We back off a couple of seconds to give the test predictable margin
	// against time.Since drift.
	atEdge := time.Now().Add(-(maxInitDataAge - 5*time.Second)).Unix()
	initData := signInitData(t, testBotToken, map[string]string{
		"auth_date": fmt.Sprintf("%d", atEdge),
		"user":      validUserJSON(),
	})
	if _, err := validateInitData(initData, testBotToken); err != nil {
		t.Errorf("expected near-edge auth_date to pass, got: %v", err)
	}
}

func TestLocaleFromCode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"ru", "ru"},
		{"ru-RU", "ru"},
		{"ru-UA", "ru"},
		{"en", "en"},
		{"en-US", "en"},
		{"de", "en"},
		{"", "en"},
		{"zh-CN", "en"},
	}
	for _, c := range cases {
		if got := localeFromCode(c.in); got != c.want {
			t.Errorf("localeFromCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
