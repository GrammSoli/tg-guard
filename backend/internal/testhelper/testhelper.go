// Package testhelper contains shared scaffolding for integration tests:
// an ephemeral Postgres container and HMAC initData signing.
//
// Built only under the `integration` tag so the default `go test ./...`
// run stays Docker-free and fast.
//
//go:build integration

package testhelper

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/subguard/backend/internal/model"
)

// NewPostgres spins up an ephemeral Postgres 16 container, runs the
// project's AutoMigrate, and returns a *gorm.DB pointed at it. The
// container is torn down when the test (or its parent suite) finishes
// via t.Cleanup, so callers don't need to manage its lifetime.
//
// The container starts in <5s on a warm Docker host. Tests that need
// row-level isolation should wrap their work in a tx and roll back, or
// truncate after each subtest — the helper does NOT recreate the DB
// per test for speed.
func NewPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("subguard_test"),
		tcpostgres.WithUsername("subguard"),
		tcpostgres.WithPassword("subguard_test_password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("postgres container start: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("postgres container terminate: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Silent — surfaces only real errors; the worker/handler under
		// test still drives its own log statements which is what the
		// test cares about.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Subscription{},
		&model.SharedRoom{},
		&model.RoomService{},
		&model.RoomMember{},
		&model.ServiceCatalog{},
		&model.TrafficCampaign{},
		&model.AppSettings{},
		&model.Donation{},
		&model.SponsoredOffer{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	return db
}

// TruncateAll wipes every project table. Cheaper than rebuilding the
// container between subtests; call from t.Cleanup if your test mutates
// shared state that the next test must not see.
func TruncateAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	tables := []string{
		"sponsored_offers",
		"donations",
		"app_settings",
		"traffic_campaigns",
		"service_catalogs",
		"room_members",
		"room_services",
		"shared_rooms",
		"subscriptions",
		"users",
	}
	for _, table := range tables {
		// RESTART IDENTITY resets serial PKs so subsequent INSERTs start
		// from 1 again — handy when a test asserts on absolute user.id.
		if err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

// SignCryptoPayload returns the hex-encoded signature Crypto Pay's
// webhook sends in the `crypto-pay-api-signature` header for the given
// raw body. Algorithm: HMAC-SHA256(SHA256(token), body). Mirrors the
// production verification path in handler.HandleCryptoWebhook — kept
// in lockstep so a scheme change shows up as a test failure.
func SignCryptoPayload(token string, body []byte) string {
	tokenHash := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, tokenHash[:])
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignInitData builds a Telegram WebApp initData string signed with
// botToken. The fields map is merged into the payload as-is (typically
// "user" + "auth_date" + optional "chat_instance", "chat_type", etc.).
//
// Mirrors validateInitData's HMAC derivation: secret_key = HMAC-SHA256(
// "WebAppData", bot_token), hash = HMAC-SHA256(data_check_string,
// secret_key). If Telegram changes the scheme, only this helper plus
// the prod path move — a divergence shows up as test failures, not as
// silent skew.
func SignInitData(botToken string, fields map[string]string) string {
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
	values.Set("hash", hex.EncodeToString(dataMAC.Sum(nil)))
	return values.Encode()
}
