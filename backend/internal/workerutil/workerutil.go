// Package workerutil hosts cross-cutting helpers shared by every long-running
// background goroutine in this codebase: supervisor-style panic recovery
// and Telegram rate-limit response parsing.
//
// The goals are pragmatic:
//
//   - A single nil-pointer panic inside any worker must NOT crash the whole
//     backend binary. The supervisor catches the panic, logs it with a stack
//     trace, waits a short cool-off, and restarts the worker loop.
//
//   - Telegram's `Too Many Requests; retry after N` reply must be parsed
//     consistently across the notification worker and the admin broadcast
//     goroutine. Both used to embed bespoke string-matching; this package
//     replaces those with a single source of truth.
package workerutil

import (
	"log"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// supervisorCooldown is the pause between a worker panic and the restart
// attempt. Long enough to keep panic loops from drowning the log; short
// enough that operators don't notice an outage in normal monitoring.
const supervisorCooldown = 5 * time.Second

// PanicHook, when set, is invoked on every recovered worker panic with
// the worker name, the recovered value, and the captured stack. main.go
// wires this to observability.CapturePanic so panics reach Sentry.
//
// It's a package-level var rather than a Supervise parameter so this
// leaf package keeps zero non-stdlib imports (the Sentry SDK is heavy
// and we don't want it pulled into workerutil's test binary). nil hook
// = log-only behaviour, exactly as before.
var PanicHook func(source string, recovered interface{}, stack []byte)

// Supervise runs fn in the current goroutine, recovers from panics, logs the
// stack trace under [name], and restarts fn after a cool-off. It returns
// only when fn returns normally (which workers typically do only on
// graceful ctx cancellation).
//
// Usage from main.go:
//
//	go workerutil.Supervise("notification-worker", func() {
//	    notifWorker.Start(ctx)
//	})
//
// The wrapped fn should be the worker's Start method or equivalent loop —
// NOT a single tick. Supervise is intended for top-level goroutines.
func Supervise(name string, fn func()) {
	for {
		exited := func() (panicked bool) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					log.Printf("[%s] PANIC: %v\n%s", name, r, stack)
					if PanicHook != nil {
						PanicHook(name, r, stack)
					}
					panicked = true
				}
			}()
			fn()
			return false
		}()

		if !exited {
			// fn returned normally — propagate the exit. This is the
			// graceful-shutdown path: the worker saw ctx.Done() and
			// returned, the parent goroutine is done.
			return
		}
		log.Printf("[%s] restarting after cooldown of %s", name, supervisorCooldown)
		time.Sleep(supervisorCooldown)
	}
}

// retryAfterPattern matches the "retry after N" fragment Telegram embeds
// in 429 error messages. Both the go-telegram client and Telegram itself
// surface this as plain text inside the error string; we accept a few
// surrounding forms with a lenient regex.
var retryAfterPattern = regexp.MustCompile(`retry after (\d+)`)

// ParseRetryAfter extracts the "retry after N seconds" hint from a
// Telegram 429 error. Returns the parsed duration and ok=true on a
// match; ok=false (with a zero duration) when the error doesn't look
// like a Telegram rate-limit. Callers should check the err is non-nil
// before calling — this function does not gate on the underlying error
// type.
//
// Examples Telegram returns (lower-cased before matching):
//
//	"too many requests: retry after 17"
//	"telegram api error: Too Many Requests: retry after 30"
//
// The seconds value is capped at 5 minutes — anything longer is almost
// certainly a parsing error or a runaway server-side issue, and we'd
// rather give up and retry on the next worker tick than block the
// goroutine for an hour.
func ParseRetryAfter(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	msg := strings.ToLower(err.Error())
	m := retryAfterPattern.FindStringSubmatch(msg)
	if len(m) < 2 {
		return 0, false
	}
	secs, parseErr := strconv.Atoi(m[1])
	if parseErr != nil || secs <= 0 {
		return 0, false
	}
	if secs > 300 {
		secs = 300
	}
	return time.Duration(secs) * time.Second, true
}

// IsRateLimit returns true when err carries Telegram's "too many requests"
// signature. Used by callers that just need to detect a 429 without
// caring about the retry_after value (e.g. to log a metric).
func IsRateLimit(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "too many requests")
}
