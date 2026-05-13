package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/bot"
	tgbot "github.com/go-telegram/bot"
	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/handler"
	"github.com/subguard/backend/internal/middleware"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/notifier"
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
	sqlDB.SetMaxOpenConns(envInt("DB_MAX_OPEN_CONNS", 25))
	sqlDB.SetMaxIdleConns(envInt("DB_MAX_IDLE_CONNS", 10))
	sqlDB.SetConnMaxLifetime(time.Hour)
	sqlDB.SetConnMaxIdleTime(15 * time.Minute)

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
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("redis connection error: %v", err)
	}
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

	// ── Telegram Bot ───────────────────────────────────
	var tgBot *tgbot.Bot
	if !isTestMode {
		if cfg.WebhookSecret == "" {
			log.Fatal("WEBHOOK_SECRET is required in production")
		}
		tgBot, err = bot.Setup(cfg, db, notifWorker)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var workerWG sync.WaitGroup

	currencyWorker := worker.NewCurrencyWorker(rdb)
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

	// ── Fiber app ──────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "SubGuard API",
		ErrorHandler: globalErrorHandler,
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins(cfg, isTestMode),
		AllowHeaders: []string{"Content-Type", "X-Telegram-Init-Data"},
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
	}))

	// ── Health check (no auth, outside /api/v1 to avoid group collision) ──
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ── Webhook (no auth, secret verified inside) ──────
	if !isTestMode {
		webhookHandler := handler.NewWebhookHandler(tgBot, cfg.WebhookSecret)
		app.Post("/webhook", webhookHandler.Handle)
	}

	// ── Public catalog (no auth needed) ────────────────
	adminH := handler.NewAdminHandler(db, cfg, tgBot, ctx)
	app.Get("/api/v1/catalog", adminH.ListCatalog)

	// ── Authenticated routes ───────────────────────────
	auth := app.Group("/api/v1", middleware.AuthMiddleware(cfg.BotToken, db))

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
	userH := handler.NewUserHandler(cfg, db).WithNotifier(n)
	auth.Get("/me", userH.GetMe)
	auth.Patch("/me", userH.UpdateMe)
	auth.Get("/me/export", userH.ExportMe)
	auth.Delete("/me", userH.DeleteMe)

	// Subscriptions
	subH := handler.NewSubscriptionHandler(db)
	auth.Get("/subscriptions", subH.List)
	auth.Post("/subscriptions", subH.Create)
	auth.Patch("/subscriptions/:id", subH.Update)
	auth.Delete("/subscriptions/:id", subH.Delete)

	// Rooms
	roomH := handler.NewRoomHandler(db, n)
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
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
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
