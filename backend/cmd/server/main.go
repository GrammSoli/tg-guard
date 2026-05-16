package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	// Embed the IANA tzdata so distroless/scratch containers can resolve
	// every zone name a user might submit. Without this, the slim image
	// at runtime falls back to UTC for anything that needs /usr/share/
	// zoneinfo — the timezone package's silent UTC fallback would mask
	// the failure and reminders would arrive at the wrong wall-clock time.
	_ "time/tzdata"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	tgbot "github.com/go-telegram/bot"
	"github.com/subguard/backend/internal/bot"
	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/handler"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
	"github.com/subguard/backend/internal/observability"
	"github.com/subguard/backend/internal/seed"
	"github.com/subguard/backend/internal/worker"
	"github.com/subguard/backend/internal/workerutil"
)

// workerDrainTimeout caps how long graceful shutdown waits for in-flight
// worker ticks to finish before forcing rdb/db close. 15s is generous for
// a notification batch (typical tick < 5s) and short enough that k8s
// pre-stop deadlines stay inside the default 30s grace period.
const workerDrainTimeout = 15 * time.Second

func main() {
	// ── Config ─────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	isTestMode := os.Getenv("APP_ENV") == "test"
	if isTestMode {
		log.Println("\u26a0\ufe0f  Running in TEST mode")
	}

	// \u2500\u2500 Sentry / observability \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500
	// No-op when SENTRY_DSN is unset. sentryFlush drains buffered events
	// on shutdown; deferred first so it runs last. workerutil.PanicHook
	// routes every recovered worker panic into Sentry as a fatal event.
	sentryFlush := observability.Init(os.Getenv("APP_VERSION"))
	defer sentryFlush()
	workerutil.PanicHook = observability.CapturePanic

	// ── Database ───────────────────────────────────────
	dbURL := cfg.DatabaseURL
	if isTestMode {
		if testDB := os.Getenv("TEST_DATABASE_URL"); testDB != "" {
			dbURL = testDB
			log.Println("using TEST_DATABASE_URL")
		}
	}
	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("database error: %v", err)
	}
	log.Println("database connected")

	// Tune the underlying connection pool. Without limits GORM will hand out
	// connections until Postgres's max_connections is exhausted under load.
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("get sql.DB error: %v", err)
	}
	// Defaults sized for the 1k–10k user soft-launch range. 50 open / 20
	// idle leaves headroom for 6 background workers + the HTTP pool to
	// burst together on a single backend instance, against the standard
	// PG max_connections=100 baseline. Env vars override for either
	// direction (small VPS or larger managed Postgres).
	maxOpen := envInt("DB_MAX_OPEN_CONNS", 50)
	maxIdle := envInt("DB_MAX_IDLE_CONNS", 20)
	if maxIdle > maxOpen {
		// database/sql clamps idle to open silently. Warn the operator
		// so a misconfigured env (e.g. accidentally setting idle larger
		// than open) doesn't look like "everything's fine, just smaller
		// idle pool than I asked for." Audit Low.
		log.Printf("[db] DB_MAX_IDLE_CONNS=%d > DB_MAX_OPEN_CONNS=%d — clamping idle to %d", maxIdle, maxOpen, maxOpen)
		maxIdle = maxOpen
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

	// Always ensure recently-added columns exist. Safe to run repeatedly
	// thanks to IF NOT EXISTS. Required because RUN_MIGRATIONS is off in
	// production and AutoMigrate won't run.
	//
	// Each statement is logged on failure (previously `_, _ = sqlDB.Exec`
	// silently swallowed everything, including legitimate breakage like
	// permission denied or syntax error after a refactor — operators
	// had no signal). IF NOT EXISTS makes legitimate re-runs no-ops, so
	// real errors are signal, not noise.
	if sqlDB, err := db.DB(); err == nil {
		runMigration := func(label, query string) {
			if _, execErr := sqlDB.Exec(query); execErr != nil {
				log.Printf("[migration] %s failed: %v", label, execErr)
			}
		}
		runMigration("users.deleted_at", `ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ`)
		runMigration("idx_users_deleted_at", `CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at)`)
		runMigration("users.is_banned", `ALTER TABLE users ADD COLUMN IF NOT EXISTS is_banned BOOLEAN NOT NULL DEFAULT false`)
		runMigration("users.is_active", `ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true`)
		runMigration("users.traffic_source_id", `ALTER TABLE users ADD COLUMN IF NOT EXISTS traffic_source_id VARCHAR(64) DEFAULT ''`)
		// Paywall columns on app_settings
		runMigration("app_settings.paywall_enabled", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS paywall_enabled BOOLEAN NOT NULL DEFAULT false`)
		runMigration("app_settings.free_subs_limit", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS free_subs_limit INTEGER NOT NULL DEFAULT 6`)
		runMigration("app_settings.free_room_limit", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS free_room_limit INTEGER NOT NULL DEFAULT 1`)
		// Emergency kill-switch columns on app_settings
		runMigration("app_settings.maintenance_mode", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS maintenance_mode BOOLEAN NOT NULL DEFAULT false`)
		runMigration("app_settings.pause_notifications", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS pause_notifications BOOLEAN NOT NULL DEFAULT false`)
		// Plan-split (Month / Lifetime) pricing + premium expiry
		runMigration("users.premium_expires_at", `ALTER TABLE users ADD COLUMN IF NOT EXISTS premium_expires_at TIMESTAMPTZ`)
		runMigration("app_settings.price_stars_month_ru", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_stars_month_ru INTEGER NOT NULL DEFAULT 75`)
		runMigration("app_settings.price_stars_lifetime_ru", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_stars_lifetime_ru INTEGER NOT NULL DEFAULT 500`)
		runMigration("app_settings.price_stars_month_en", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_stars_month_en INTEGER NOT NULL DEFAULT 150`)
		runMigration("app_settings.price_stars_lifetime_en", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_stars_lifetime_en INTEGER NOT NULL DEFAULT 1000`)
		runMigration("app_settings.price_crypto_month_usd_ru", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_crypto_month_usd_ru INTEGER NOT NULL DEFAULT 1`)
		runMigration("app_settings.price_crypto_lifetime_usd_ru", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_crypto_lifetime_usd_ru INTEGER NOT NULL DEFAULT 10`)
		runMigration("app_settings.price_crypto_month_usd_en", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_crypto_month_usd_en INTEGER NOT NULL DEFAULT 2`)
		runMigration("app_settings.price_crypto_lifetime_usd_en", `ALTER TABLE app_settings ADD COLUMN IF NOT EXISTS price_crypto_lifetime_usd_en INTEGER NOT NULL DEFAULT 20`)
		// Idempotent charge-ID column on donations (Stars webhook dedup)
		runMigration("donations.telegram_payment_charge_id", `ALTER TABLE donations ADD COLUMN IF NOT EXISTS telegram_payment_charge_id VARCHAR(512) NOT NULL DEFAULT ''`)
		runMigration("idx_donations_charge_id", `CREATE UNIQUE INDEX IF NOT EXISTS idx_donations_charge_id ON donations (telegram_payment_charge_id) WHERE telegram_payment_charge_id != ''`)
		// Idempotency stamp for the billing-reset worker. Existing rooms
		// get NULL and will be reset on the first eligible tick — that
		// matches the previous behaviour, no surprise resets.
		runMigration("shared_rooms.last_billing_reset_at", `ALTER TABLE shared_rooms ADD COLUMN IF NOT EXISTS last_billing_reset_at TIMESTAMPTZ`)
		// Idempotency stamp for the room-reminder worker (day-before-billing
		// DMs). Existing rooms get NULL and are simply reminded on their
		// next eligible day — no backfill needed.
		runMigration("shared_rooms.last_billing_reminder_at", `ALTER TABLE shared_rooms ADD COLUMN IF NOT EXISTS last_billing_reminder_at TIMESTAMPTZ`)
		// Per-room IANA timezone. Default UTC matches prior worker semantics
		// (billing_day was interpreted in UTC globally); new rooms get the
		// owner's TZ snapshot at CreateRoom time.
		runMigration("shared_rooms.timezone", `ALTER TABLE shared_rooms ADD COLUMN IF NOT EXISTS timezone VARCHAR(64) NOT NULL DEFAULT 'UTC'`)
		// One-shot backfill: copy each owner's stored TZ into rooms still
		// sitting on the default UTC. Idempotent — subsequent runs find
		// nothing to update once every room has a non-default value.
		// Safe because BillingResetWorker is keyed on last_billing_reset_at
		// and the shift in reset moment for an existing room is at most
		// ~12h of local-time drift, which the worker's 2h tolerance window
		// absorbs on the next eligible day.
		runMigration("shared_rooms.timezone.backfill", `
			UPDATE shared_rooms r
			SET timezone = u.timezone
			FROM users u
			WHERE u.id = r.owner_id
			  AND r.timezone = 'UTC'
			  AND u.timezone IS NOT NULL
			  AND u.timezone <> ''
			  AND u.timezone <> 'UTC'`)
		// Performance indexes (audit Tier-3 #1, #2):
		//
		//   idx_sub_due_unsent — covers the notification worker's hot
		//   query `WHERE next_payment_at BETWEEN ? AND ? AND
		//   (notified_at IS NULL OR notified_at < ?)`. The full table
		//   scan it replaces grew linearly with the subs table; on
		//   100k rows the worker tick took >1s per fire and that's
		//   roughly half our scheduling-precision budget. Partial-on-
		//   notified_at-NULL captures the common case (most due rows
		//   haven't been pinged yet) without bloating the index for
		//   already-sent rows.
		runMigration("idx_sub_due_unsent", `CREATE INDEX IF NOT EXISTS idx_sub_due_unsent ON subscriptions (next_payment_at) WHERE notified_at IS NULL`)
		runMigration("idx_sub_notified_at", `CREATE INDEX IF NOT EXISTS idx_sub_notified_at ON subscriptions (notified_at) WHERE notified_at IS NOT NULL`)
		//
		//   idx_sub_brand_name — supports the admin GetPopularServices
		//   GROUP BY (brand, name). The current HashAggregate on a
		//   full scan is fine at thousands of rows; this index keeps
		//   the query sub-100ms as the table grows past ~100k subs.
		runMigration("idx_sub_brand_name", `CREATE INDEX IF NOT EXISTS idx_sub_brand_name ON subscriptions (brand, name)`)
		//
		//   idx_users_traffic_source — partial on traffic_source_id WHERE
		//   non-empty. The admin GetStats today-sources breakdown groups
		//   by COALESCE(NULLIF(traffic_source_id, ''), 'organic') and
		//   filters created_at >= today. Without this index the GROUP BY
		//   degrades to a HashAggregate on a full scan; cheap today
		//   (most rows are organic = empty), grows linearly with paid
		//   acquisition. Partial keeps the index small. Audit Low.
		runMigration("idx_users_traffic_source", `CREATE INDEX IF NOT EXISTS idx_users_traffic_source ON users (traffic_source_id) WHERE traffic_source_id != ''`)
		// NOTE: a previous destructive UPDATE backfill lived here. It set
		// `price_crypto_*` to 1/10/2/20 whenever the existing value was 0
		// — silently overriding a legitimate admin choice of 0 on every
		// restart. Removed under audit #11 once the root cause of the
		// admin-UI price bug was fixed (column-name mismatch in
		// model.AppSettings — see the `gorm:"column:..."` tags there).
		// The ad-hoc UPDATE is no longer needed: ADD COLUMN ... DEFAULT
		// already backfills existing rows on PG 11+, and from this
		// deploy onwards the admin UI is the single source of truth.
		log.Println("ad-hoc migrations applied")

		// Index-coverage diagnostic: print every public-schema index covering
		// the performance-critical hot columns. Read-only, runs once per
		// boot. The output lets the operator confirm at a glance that the
		// expected GORM-tag indexes (users.telegram_id uniqueIndex,
		// subscriptions.user_id, room_members.user_id, donations.user_id)
		// actually exist on prod — they were declared via struct tags and
		// would only have been created by an AutoMigrate run at some past
		// deploy. If a WARNING appears for a missing index, the next deploy
		// should add an explicit CREATE INDEX IF NOT EXISTS for it.
		hotColumns := []struct{ table, column string }{
			{"users", "telegram_id"},
			{"subscriptions", "user_id"},
			{"subscriptions", "next_payment_at"},
			{"room_members", "user_id"},
			{"shared_rooms", "owner_id"},
			{"donations", "user_id"},
		}
		for _, hc := range hotColumns {
			var count int
			err := sqlDB.QueryRow(`
				SELECT COUNT(*)
				FROM pg_indexes
				WHERE schemaname = 'public'
				  AND tablename = $1
				  AND (indexdef ILIKE '%(' || $2 || ')%'
				       OR indexdef ILIKE '%(' || $2 || ',%'
				       OR indexdef ILIKE '%, ' || $2 || ')%'
				       OR indexdef ILIKE '%, ' || $2 || ',%')`,
				hc.table, hc.column).Scan(&count)
			if err != nil {
				log.Printf("[index-check] %s.%s — query error: %v", hc.table, hc.column, err)
				continue
			}
			if count == 0 {
				log.Printf("[index-check] WARNING: no index covers %s.%s — perf risk at scale", hc.table, hc.column)
			} else {
				log.Printf("[index-check] OK: %d index(es) cover %s.%s", count, hc.table, hc.column)
			}
		}
	}

	// Auto-migrate only in test/dev. Production should run a dedicated
	// migration tool (golang-migrate, atlas) out-of-band so rolling deploys
	// don't race on ALTER TABLE.
	if isTestMode || os.Getenv("RUN_MIGRATIONS") == "1" {
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
			log.Fatalf("migration error: %v", err)
		}
		log.Println("migrations applied")
	} else {
		log.Println("skipping AutoMigrate (set RUN_MIGRATIONS=1 to enable)")
	}

	// Seed catalog data
	seed.SeedCatalog(db)

	// Seed test data (only in test mode)
	if isTestMode {
		seed.SeedTestData(db)
	}

	// ── Redis ──────────────────────────────────────────
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis url error: %v", err)
	}
	// Pool tuning — env-overridable for staging/load tests. Defaults sized
	// for ~20 concurrent backend handlers + workers; bump POOL_SIZE if you
	// see "max number of clients reached" in Redis logs.
	// 40 conns covers the 50-DB-pool case where most HTTP handlers touch
	// Redis (FX cache, rate-limit checks) plus the FX worker writes. Less
	// headroom than DB pool because Redis ops are sub-ms — short hold
	// times keep contention low.
	opt.PoolSize = envInt("REDIS_POOL_SIZE", 40)
	opt.MinIdleConns = envInt("REDIS_MIN_IDLE", 10)
	opt.ReadTimeout = 10 * time.Second
	opt.WriteTimeout = 10 * time.Second
	rdb := redis.NewClient(opt)
	// Bound the startup Ping so a hung Redis can't wedge the binary.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		pingCancel()
		log.Fatalf("redis connection error: %v", err)
	}
	pingCancel()
	log.Println("redis connected")

	// ── Notifier (real TG or mock for tests) ─────────
	// In production the real TelegramNotifier is injected after bot.Setup
	// (which needs the worker, which needs the notifier — circular). We
	// start with nil and call SetNotifier once the bot instance exists.
	var n notifier.Notifier
	if isTestMode {
		n = notifier.NewMockNotifier()
		log.Println("using MockNotifier (test mode)")
	}

	// ── Notification Worker (created early so bot.Setup can reference it) ──
	notifWorker := worker.NewNotificationWorker(db, n)

	// Lifecycle ctx + wg created BEFORE bot.Setup so they can be threaded
	// into the bot's background goroutines (broadcast, async export).
	// Previously these were created later and the bot package fell back
	// to context.Background(), causing graceful-shutdown leaks (audit C2).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var workerWG sync.WaitGroup

	// ── Telegram Bot ───────────────────────────────────
	var tgBot *tgbot.Bot
	if !isTestMode {
		if cfg.WebhookSecret == "" {
			log.Fatal("WEBHOOK_SECRET is required in production")
		}
		tgBot, err = bot.Setup(cfg, db, notifWorker, rdb, ctx, &workerWG)
		if err != nil {
			log.Fatalf("bot setup error: %v", err)
		}

		// Now that the bot is ready, create the real notifier and inject it.
		n = notifier.NewTelegramNotifier(tgBot)
		notifWorker.SetNotifier(n)

		if err := bot.SetWebhook(tgBot, cfg); err != nil {
			log.Printf("webhook setup warning: %v", err)
		}
	} else {
		log.Println("skipping Telegram Bot setup (test mode)")
	}

	// ── Workers (background goroutines) ────────────────
	// Each worker runs inside workerutil.Supervise so a panic logs the
	// stack and restarts the loop after a cool-off instead of crashing
	// the whole binary. workerWG lets the shutdown handler wait for all
	// worker ticks to finish before closing DB / Redis pools.

	currencyWorker := worker.NewCurrencyWorker(rdb, cfg.CurrencyAPIURL)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("currency-worker", func() { currencyWorker.Start(ctx) })
	}()

	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("notification-worker", func() { notifWorker.Start(ctx) })
	}()

	billingWorker := worker.NewBillingResetWorker(db)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("billing-reset", func() { billingWorker.Start(ctx) })
	}()

	// Premium-expiration worker — downgrades users whose month-plan
	// Premium has lapsed. tgBot is nil in test mode; the worker skips
	// the expiry DM in that case.
	premiumWorker := worker.NewPremiumWorker(db, tgBot)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("premium-expiration", func() { premiumWorker.Start(ctx) })
	}()

	// Room-reminder worker — DMs every member the day before their room's
	// monthly billing_day. The hourly tick fires globally, but each room
	// only gets a reminder when ITS local clock reads ROOM_REMINDER_HOUR
	// (room.timezone, not UTC).
	roomReminderWorker := worker.NewRoomReminderWorker(db, n, cfg.BaseURL, envInt("ROOM_REMINDER_HOUR", 9))
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("room-reminder", func() { roomReminderWorker.Start(ctx) })
	}()

	// Trial-expiry worker — converts trials whose trial_ends_at has passed
	// into regular subscriptions, so a lapsed trial stops reading as one.
	trialExpiryWorker := worker.NewTrialExpiryWorker(db)
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		workerutil.Supervise("trial-expiry", func() { trialExpiryWorker.Start(ctx) })
	}()

	// ── Fiber app ──────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "SubGuard API",
		ErrorHandler: globalErrorHandler,
	})

	// Global middleware. The recover middleware catches panics in any
	// request handler — including bot callback/command handlers, which
	// run synchronously inside the /webhook route via bot.ProcessUpdate.
	// StackTraceHandler routes the panic to Sentry before recover turns
	// it into a 500.
	app.Use(recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c fiber.Ctx, e any) {
			stack := debug.Stack()
			log.Printf("[recover] panic on %s %s: %v\n%s", c.Method(), c.Path(), e, stack)
			observability.CapturePanicWithUser("http:"+c.Path(), sentryUserFromCtx(c), e, stack)
		},
	}))
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins(cfg, isTestMode),
		AllowHeaders: []string{"Content-Type", "X-Telegram-Init-Data"},
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
	}))

	// ── Health check (no auth, outside /api/v1 to avoid group collision) ──
	app.Get("/health", func(c fiber.Ctx) error {
		// Probe dependencies so k8s stops routing to a pod whose DB or
		// Redis is unreachable. Bounded timeout keeps the probe itself
		// from hanging when a dependency is slow rather than down.
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "up"
		if sqlDB, err := db.DB(); err != nil || sqlDB.PingContext(ctx) != nil {
			dbStatus = "down"
		}
		redisStatus := "up"
		if rdb.Ping(ctx).Err() != nil {
			redisStatus = "down"
		}

		if dbStatus == "down" || redisStatus == "down" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "degraded", "db": dbStatus, "redis": redisStatus,
			})
		}
		return c.JSON(fiber.Map{"status": "ok", "db": dbStatus, "redis": redisStatus})
	})

	// ── Webhook rate limit ─────────────────────────────
	// /webhook routes are public (HMAC/secret verified inside the handler).
	// Telegram and CryptoPay deliver from a small IP set at a modest rate,
	// so 600/min/IP sits far above real delivery volume while still blunting
	// a junk flood that would otherwise burn CPU on signature checks.
	// Telegram retries non-2xx, so a rare 429 self-heals. Prefix match on
	// "/webhook" covers both /webhook and /webhook/crypto.
	app.Use("/webhook", limiter.New(limiter.Config{
		Max:          envInt("WEBHOOK_RATE_LIMIT", 600),
		Expiration:   time.Minute,
		KeyGenerator: func(c fiber.Ctx) string { return "ip:" + c.IP() },
		LimitReached: func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusTooManyRequests)
		},
	}))

	// ── Webhook (no auth, secret verified inside) ──────
	if !isTestMode {
		webhookHandler := handler.NewWebhookHandler(tgBot, cfg.WebhookSecret)
		app.Post("/webhook", webhookHandler.Handle)
	}

	// ── Crypto Pay webhook (no auth, HMAC-SHA256 verified inside) ──
	cryptoWebhookH := handler.NewPaymentHandler(db, cfg, tgBot)
	app.Post("/webhook/crypto", cryptoWebhookH.HandleCryptoWebhook)

	// ── Public catalog (no auth needed) ────────────────
	adminH := handler.NewAdminHandler(db, cfg, tgBot, rdb, ctx).WithLifecycle(&workerWG)
	app.Get("/api/v1/catalog", adminH.ListCatalog)

	// ── Authenticated routes ───────────────────────────
	// MaintenanceGuard is the FIRST middleware in the group — it runs
	// before AuthMiddleware on purpose. During a maintenance window we
	// reject as early and cheaply as possible: no initData HMAC
	// validation, no DB/Redis session work for a request we're going to
	// 503 anyway. It also means a user with a stale token sees the same
	// maintenance stub as everyone else, not a misleading "session
	// expired". The guard skips /admin/ paths so admin endpoints stay
	// reachable; /health and /webhook live outside this group entirely.
	auth := app.Group("/api/v1",
		middleware.MaintenanceGuard(db, cfg),
		middleware.AuthMiddleware(cfg.BotToken, db),
	)

	// Per-user rate limit on authenticated mutations. The bare limiter
	// (`max=120/min`) is intentionally generous — the goal is to stop
	// runaway scripts, not throttle real usage.
	auth.Use(limiter.New(limiter.Config{
		Max:        envInt("RATE_LIMIT_PER_MIN", 120),
		Expiration: time.Minute,
		KeyGenerator: func(c fiber.Ctx) string {
			if u := middleware.UserFromCtx(c); u != nil {
				return "u:" + strconv.FormatUint(uint64(u.ID), 10)
			}
			return "ip:" + c.IP()
		},
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "rate limit"})
		},
	}))

	// User
	userH := handler.NewUserHandler(cfg, db).WithNotifier(n).WithLifecycle(ctx, &workerWG)
	auth.Get("/me", userH.GetMe)
	auth.Patch("/me", userH.UpdateMe)
	auth.Get("/me/export", userH.ExportMe)
	auth.Delete("/me", userH.DeleteMe)

	// Payments (Stars + Crypto)
	paymentH := handler.NewPaymentHandler(db, cfg, tgBot)
	if tgBot != nil {
		auth.Post("/payments/stars", paymentH.CreateStarsInvoice)
	}
	auth.Post("/payments/crypto", paymentH.CreateCryptoInvoice)

	// Public config (paywall limits — available to all authenticated users)
	auth.Get("/config", adminH.GetPublicConfig)

	// FX rates — cached USD-base map written by the currency worker.
	// Read once on app boot by the frontend's fxStore; powers
	// convertCurrency() everywhere. Auth-gated to keep an unauthenticated
	// scraper off the endpoint, even though rates aren't sensitive.
	currencyH := handler.NewCurrencyHandler(rdb)
	auth.Get("/fx", currencyH.GetRates)

	// Subscriptions
	subH := handler.NewSubscriptionHandler(db)
	auth.Get("/subscriptions", subH.List)
	auth.Post("/subscriptions", subH.Create)
	auth.Patch("/subscriptions/:id", subH.Update)
	auth.Delete("/subscriptions/:id", subH.Delete)

	// Recommendations (sponsored offers)
	recsH := handler.NewRecommendationsHandler(db)
	auth.Get("/recommendations", recsH.List)
	auth.Post("/recommendations/track/view", recsH.TrackView)
	auth.Post("/recommendations/:id/track/click", recsH.TrackClick)

	// Rooms
	roomH := handler.NewRoomHandler(db, cfg, n)
	auth.Get("/rooms", roomH.List)
	auth.Post("/rooms", roomH.Create)
	auth.Get("/rooms/:id", roomH.GetDetail)
	auth.Delete("/rooms/:id", roomH.DeleteRoom)
	auth.Post("/rooms/join/:invite", roomH.Join)
	auth.Post("/rooms/:id/remind", roomH.Remind)
	auth.Patch("/rooms/:id/members/:uid/pay", roomH.MarkPaid)
	auth.Patch("/rooms/:id/members/:uid/unpay", roomH.MarkUnpaid)
	auth.Post("/rooms/:id/services", roomH.AddService)
	auth.Delete("/rooms/:id/services/:sid", roomH.RemoveService)
	auth.Delete("/rooms/:id/members/:uid", roomH.RemoveMember)
	auth.Patch("/rooms/:id", roomH.UpdateRoom)

	// Admin (auth + admin-only)
	admin := auth.Group("/admin", adminH.AdminOnly)
	admin.Get("/stats", adminH.GetStats)
	admin.Get("/catalog", adminH.ListCatalog)
	admin.Post("/catalog", adminH.CreateCatalogItem)
	admin.Patch("/catalog/:id", adminH.UpdateCatalogItem)
	admin.Delete("/catalog/:id", adminH.DeleteCatalogItem)
	admin.Get("/settings", adminH.GetSettings)
	admin.Patch("/settings", adminH.UpdateSettings)
	admin.Post("/broadcast", adminH.Broadcast)
	admin.Get("/campaigns", adminH.ListCampaigns)

	// ── Graceful shutdown ──────────────────────────────
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("shutting down...")
		cancel()

		// Stop accepting HTTP traffic first so in-flight handlers drain
		// cleanly while the workers also wind down.
		if err := app.Shutdown(); err != nil {
			log.Printf("fiber shutdown error: %v", err)
		}

		// Wait for all worker goroutines to return. STRICTLY before we
		// close the DB / Redis pools — otherwise a mid-tick worker would
		// error out trying to write notified_at against a closed pool,
		// causing duplicate notifications on the next start.
		drained := make(chan struct{})
		go func() {
			workerWG.Wait()
			close(drained)
		}()
		select {
		case <-drained:
			log.Println("workers drained cleanly")
		case <-time.After(workerDrainTimeout):
			log.Printf("⚠️  workers did not drain within %s — forcing shutdown", workerDrainTimeout)
		}

		if err := rdb.Close(); err != nil {
			log.Printf("redis close error: %v", err)
		}
		if sqlDB, err := db.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				log.Printf("db close error: %v", err)
			}
		}
	}()

	// ── Start ──────────────────────────────────────────
	addr := ":" + cfg.APIPort
	log.Printf("listening on %s", addr)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func globalErrorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	log.Printf("[error] %d: %v", code, err)
	// Only 5xx reaches Sentry — 4xx is client error (bad input, missing
	// auth) and would just be noise. Panics that recover.New() turned
	// into a 500 were already reported by its StackTraceHandler, but
	// re-capturing here is harmless: Sentry dedups by fingerprint.
	if code >= 500 {
		observability.CaptureHTTPError(c.Method(), c.Path(), code, sentryUserFromCtx(c), err)
		// 5xx bodies are exposed to the public mini-app — don't leak
		// stack traces, raw DB errors, or upstream URLs in the body.
		// The full err is in logs + Sentry; the client just needs a
		// stable code it can branch on. Audit Tier-1 #6.
		return c.Status(code).JSON(fiber.Map{"error": "internal_error"})
	}
	// 4xx and 3xx — surface the original message; these are client-
	// actionable (validation failures, not-found, etc.).
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}

// sentryUserFromCtx pulls the authenticated user out of the Fiber
// request context and adapts it to observability.UserInfo. Returns nil
// when the request never authenticated (panic before auth middleware,
// the /webhook route, /health) — the event is still captured, just
// without user attribution.
func sentryUserFromCtx(c fiber.Ctx) *observability.UserInfo {
	u := middleware.UserFromCtx(c)
	if u == nil {
		return nil
	}
	return &observability.UserInfo{
		InternalID: u.ID,
		TelegramID: u.TelegramID,
		Username:   u.Username,
	}
}

// envInt reads an integer env var with a fallback.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// corsOrigins assembles the AllowOrigins list. Production = BaseURL only.
// Test/dev adds localhost. CORS_EXTRA_ORIGINS appends comma-separated extras
// for staging/preview deployments.
func corsOrigins(cfg *config.Config, testMode bool) []string {
	origins := []string{cfg.BaseURL}
	if testMode {
		origins = append(origins, "http://localhost:5173")
	}
	if extra := os.Getenv("CORS_EXTRA_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	return origins
}
