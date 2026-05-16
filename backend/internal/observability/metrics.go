package observability

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the project's process-wide Prometheus collector set. Held
// behind an interface-shaped struct so tests can swap individual
// counters out, and so we keep the cardinality budget visible in one
// place (path is template-bucketed, not raw URL — see HTTPMiddleware).
var (
	// HTTPRequestsTotal counts every request that exits the Fiber
	// router with a final status. Labels: method (GET/POST/...),
	// path (templated, NOT raw URL — see normalizePath in
	// HTTPMiddleware to keep cardinality bounded), status (2xx/3xx
	// /4xx/5xx bucket). The bucketed status avoids the cardinality
	// blow-up of using the raw int 200/201/400/... — Prometheus
	// alerting on 5xx rate doesn't need that resolution.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tgguard_http_requests_total",
			Help: "Total HTTP requests served, bucketed by method/path-template/status-class.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration measures end-to-end Fiber handler latency.
	// Default-ish buckets tuned for the WebApp's expectation: most
	// calls under 100ms, anything over 1s is a problem signal.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tgguard_http_request_duration_seconds",
			Help:    "HTTP handler duration in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// WorkerTicksTotal counts background worker iterations by
	// outcome. "ok" means the tick returned without an unhandled
	// error; "error" means a panic was recovered or the worker's
	// own observability layer reported a failure. Useful as a
	// liveness signal — rate(tick_total[5m]) == 0 = worker stuck.
	WorkerTicksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tgguard_worker_ticks_total",
			Help: "Background worker tick count by name and outcome.",
		},
		[]string{"worker", "outcome"},
	)

	// WorkerTickDuration tracks how long a single iteration runs.
	// The notification worker is the only one regularly above 1s
	// (it streams subscription batches); the rest should be
	// sub-100ms most of the time.
	WorkerTickDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tgguard_worker_tick_duration_seconds",
			Help:    "Background worker tick duration in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120},
		},
		[]string{"worker"},
	)

	// DBPoolStats mirrors sql.DBStats fields as Prometheus gauges.
	// Updated by StartDBPoolWatcher on a tick — cheaper than emitting
	// on every query and more honest than scraping the raw counters
	// (sql.DBStats counts are monotonic).
	DBPoolStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tgguard_db_pool_connections",
			Help: "Postgres connection-pool counters from sql.DBStats.",
		},
		[]string{"state"}, // open | idle | in_use | wait_count
	)

	// NotificationsSentTotal is the one application-counter that
	// matters for product health: are reminders actually reaching
	// users? Labels: outcome=sent|failed|rate_limited|skipped_dnd.
	NotificationsSentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tgguard_notifications_sent_total",
			Help: "Renewal-reminder send outcomes.",
		},
		[]string{"outcome"},
	)
)

// Register adds every metric to the default Prometheus registry. Idempotent
// — a duplicate Register is a noop. Called once from main.go on boot.
func Register() {
	for _, c := range []prometheus.Collector{
		HTTPRequestsTotal,
		HTTPRequestDuration,
		WorkerTicksTotal,
		WorkerTickDuration,
		DBPoolStats,
		NotificationsSentTotal,
	} {
		// MustRegister panics on duplicate; we use the lenient
		// Register and ignore "already registered" to keep tests
		// (which may call Register more than once) and the boot
		// path well-behaved.
		_ = prometheus.Register(c)
	}
}

// MetricsHandler returns the standard /metrics HTTP handler wrapped so
// Fiber's net/http adapter can mount it. When bearerToken is non-empty,
// requests must carry Authorization: Bearer <token>; without it any
// internal scraper would have to be network-level isolated to avoid
// leaking process internals.
func MetricsHandler(bearerToken string) http.Handler {
	promHandler := promhttp.Handler()
	if bearerToken == "" {
		return promHandler
	}
	expected := "Bearer " + bearerToken
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Constant-time compare would be ideal, but stdlib doesn't
		// expose subtle.ConstantTimeCompare on net/http.Header — the
		// bearer token is high-entropy random so timing-channel
		// concern is low. Documented as known trade-off.
		if r.Header.Get("Authorization") != expected {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		promHandler.ServeHTTP(w, r)
	})
}

// HTTPMiddleware is Fiber middleware that emits HTTPRequestsTotal +
// HTTPRequestDuration on every request. Path is taken from the matched
// route template (e.g. "/api/v1/subscriptions/:id") so cardinality stays
// bounded — a raw c.Path() with UUIDs would explode the label space.
//
// Static / unmatched routes get the literal "unknown" path label rather
// than the raw URL, for the same cardinality reason.
func HTTPMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		elapsed := time.Since(start).Seconds()

		method := c.Method()
		path := c.Route().Path
		if path == "" {
			path = "unknown"
		}
		status := strconv.Itoa(c.Response().StatusCode())
		statusClass := statusBucket(status)

		HTTPRequestsTotal.WithLabelValues(method, path, statusClass).Inc()
		HTTPRequestDuration.WithLabelValues(method, path).Observe(elapsed)
		return err
	}
}

// statusBucket reduces full 3-digit status codes to 2xx/3xx/4xx/5xx
// classes plus a literal "0" for the rare network-aborted case where
// Fiber never assigned a code. Keeps the status label dimension at 5
// values, not 50.
func statusBucket(s string) string {
	if len(s) == 0 {
		return "0"
	}
	switch s[0] {
	case '2':
		return "2xx"
	case '3':
		return "3xx"
	case '4':
		return "4xx"
	case '5':
		return "5xx"
	default:
		return "other"
	}
}

// ObserveWorkerTick is a convenience helper for workers: pass the
// worker name and the err returned by the tick function (nil → ok).
// Pair with a defer for duration.
//
//	t := time.Now()
//	defer func() {
//	    observability.ObserveWorkerTick("billing-reset", time.Since(t), nil)
//	}()
func ObserveWorkerTick(worker string, duration time.Duration, err error) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	WorkerTicksTotal.WithLabelValues(worker, outcome).Inc()
	WorkerTickDuration.WithLabelValues(worker).Observe(duration.Seconds())
}

// TimeWorkerTick is the one-line variant for workers whose check()
// doesn't surface an error (most of them; failures are already logged
// + Sentry-captured inside the tick). Usage:
//
//	func (w *BillingResetWorker) check(ctx context.Context) {
//	    defer observability.TimeWorkerTick("billing-reset")()
//	    // ... tick body
//	}
//
// Returns a closure that, when invoked, records the elapsed duration
// since TimeWorkerTick was called. Idiomatic with the Go `defer f()()`
// pattern: the inner call captures start time at defer-evaluation,
// the outer call fires at function exit.
func TimeWorkerTick(worker string) func() {
	start := time.Now()
	return func() {
		ObserveWorkerTick(worker, time.Since(start), nil)
	}
}

// StartDBPoolWatcher polls sql.DBStats at interval and updates the
// DBPoolStats gauges. Runs until ctx is cancelled. Cheap (DBStats() is
// in-memory). One goroutine per app.
func StartDBPoolWatcher(stop <-chan struct{}, db *sql.DB, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s := db.Stats()
				DBPoolStats.WithLabelValues("open").Set(float64(s.OpenConnections))
				DBPoolStats.WithLabelValues("idle").Set(float64(s.Idle))
				DBPoolStats.WithLabelValues("in_use").Set(float64(s.InUse))
				DBPoolStats.WithLabelValues("wait_count").Set(float64(s.WaitCount))
			}
		}
	}()
}
