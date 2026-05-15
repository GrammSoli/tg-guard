// Package observability wires the Sentry error-reporting SDK into the
// backend. Every helper here is a safe no-op when SENTRY_DSN is unset —
// dev and test runs need no Sentry account and stay completely offline.
//
// Routing of captured events to Telegram / Slack is NOT done here: the
// code only ships events to Sentry. The Telegram/Slack fan-out is a
// Sentry dashboard setting (Project → Alerts → Integrations).
package observability

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
)

// UserInfo carries the identity attached to a captured event so triage
// can answer "which user hit this?". Built from primitives — this
// package deliberately doesn't import internal/model, keeping its
// dependency surface (and the worker packages that import it) thin.
//
// A nil *UserInfo means "no user context" — used for worker panics and
// unauthenticated request failures.
type UserInfo struct {
	InternalID uint   // our users.id
	TelegramID int64  // Telegram user id
	Username   string // Telegram @username, may be empty
}

// sentryUser converts a UserInfo into the SDK's user struct. The
// internal id is the primary key (stable, never changes); the Telegram
// id + username land in Data so the dashboard search can find
// "@Ivanov" directly.
func sentryUser(u *UserInfo) sentry.User {
	if u == nil {
		return sentry.User{}
	}
	return sentry.User{
		ID:       strconv.FormatUint(uint64(u.InternalID), 10),
		Username: u.Username,
		Data: map[string]string{
			"telegram_id": strconv.FormatInt(u.TelegramID, 10),
		},
	}
}

// flushTimeout bounds how long Flush blocks while draining the event
// buffer — on shutdown and after a panic we don't want to hang the
// process waiting on a slow Sentry ingest endpoint.
const flushTimeout = 2 * time.Second

// enabled is set true once Init succeeds with a non-empty DSN. The
// Capture* helpers below are still safe to call when it's false (the
// sentry-go SDK no-ops without a configured client) — the flag just
// lets us skip the extra work of building scopes.
var enabled bool

// Init configures the global Sentry client. Returns a flush function
// the caller should defer in main() so buffered events are drained on
// graceful shutdown.
//
// release identifies the running build in the Sentry UI — pass a git
// SHA or semver. Empty is fine; Sentry just shows "unknown".
func Init(release string) func() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		log.Println("[sentry] SENTRY_DSN not set — error reporting disabled")
		return func() {}
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "production"
	}

	if err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: env,
		Release:     release,
		// AttachStacktrace adds a stack to plain CaptureMessage/error
		// calls that don't already carry one.
		AttachStacktrace: true,
		// We report errors only — performance tracing multiplies event
		// volume and Sentry quota. Flip this up if you later want spans.
		TracesSampleRate: 0.0,
		// Drop the noisy expected-error classes before they leave the
		// process so they don't burn quota or page anyone.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			for _, ex := range event.Exception {
				switch ex.Value {
				case "context canceled", "context deadline exceeded":
					return nil
				}
			}
			return event
		},
	}); err != nil {
		log.Printf("[sentry] init failed: %v — error reporting disabled", err)
		return func() {}
	}

	enabled = true
	log.Printf("[sentry] initialised (env=%s release=%q)", env, release)
	return func() { sentry.Flush(flushTimeout) }
}

// Enabled reports whether Sentry was successfully configured. Handy for
// callers that want to skip building expensive context when it's off.
func Enabled() bool { return enabled }

// CaptureException ships an error to Sentry. No-op when Sentry is
// disabled. Safe to call from any goroutine.
func CaptureException(err error) {
	if err == nil {
		return
	}
	sentry.CaptureException(err)
}

// CaptureHTTPError reports a 5xx response. Method/path are passed as
// primitives so this package never imports the web framework — keeps
// the dependency graph (and the worker packages that import this) thin.
//
// user, when non-nil, is attached to the event so triage can see which
// user hit the failure. Per-event (not a global hub user) because the
// backend is multi-tenant — concurrent requests carry different users.
func CaptureHTTPError(method, path string, status int, user *UserInfo, err error) {
	if err == nil {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("http.method", method)
		scope.SetTag("http.path", path)
		scope.SetLevel(sentry.LevelError)
		scope.SetContext("http", map[string]interface{}{
			"method":      method,
			"path":        path,
			"status_code": status,
		})
		if user != nil {
			scope.SetUser(sentryUser(user))
		}
		sentry.CaptureException(err)
	})
}

// CapturePanic reports a recovered panic with its captured stack. Used
// by workerutil.Supervise (via the PanicHook indirection) — worker
// panics have no user context. It Flushes synchronously because a panic
// often precedes a restart or crash and we can't rely on the deferred
// Init flush running in time.
func CapturePanic(source string, recovered interface{}, stack []byte) {
	capturePanic(source, nil, recovered, stack)
}

// CapturePanicWithUser is the HTTP variant — used by the Fiber recover
// middleware, which can pull the authenticated user out of the request
// context and attach it to the panic event.
func CapturePanicWithUser(source string, user *UserInfo, recovered interface{}, stack []byte) {
	capturePanic(source, user, recovered, stack)
}

func capturePanic(source string, user *UserInfo, recovered interface{}, stack []byte) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("panic.source", source)
		scope.SetLevel(sentry.LevelFatal)
		if len(stack) > 0 {
			scope.SetContext("panic", map[string]interface{}{
				"stacktrace": string(stack),
			})
		}
		if user != nil {
			scope.SetUser(sentryUser(user))
		}
		sentry.CurrentHub().Recover(recovered)
	})
	sentry.Flush(flushTimeout)
}

// FlushWithContext drains buffered events, bounded by ctx. Convenience
// for shutdown paths that already have a deadline context.
func FlushWithContext(ctx context.Context) {
	deadline, ok := ctx.Deadline()
	if !ok {
		sentry.Flush(flushTimeout)
		return
	}
	if d := time.Until(deadline); d > 0 {
		sentry.Flush(d)
	}
}
