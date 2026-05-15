package workerutil

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupervise_RestartsOnPanic(t *testing.T) {
	var callCount int32
	// Force Supervise to call fn three times: panic, panic, then return.
	done := make(chan struct{})
	go func() {
		Supervise("test", func() {
			n := atomic.AddInt32(&callCount, 1)
			if n < 3 {
				panic("boom")
			}
			// third call returns normally → Supervise exits
		})
		close(done)
	}()

	// supervisorCooldown is 5s between restarts; allow up to 15s.
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("Supervise did not finish after 3 attempts: got %d calls", atomic.LoadInt32(&callCount))
	}
	if got := atomic.LoadInt32(&callCount); got != 3 {
		t.Fatalf("expected 3 calls (2 panics + 1 normal exit), got %d", got)
	}
}

func TestSupervise_NormalReturnExits(t *testing.T) {
	var called int32
	done := make(chan struct{})
	go func() {
		Supervise("test", func() {
			atomic.AddInt32(&called, 1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Supervise blocked after normal return")
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("expected 1 call, got %d", atomic.LoadInt32(&called))
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantSecs int
		wantOK   bool
	}{
		{"telegram-style", errors.New("Too Many Requests: retry after 17"), 17, true},
		{"go-client style", errors.New("telegram api error: retry after 30"), 30, true},
		{"no match", errors.New("forbidden"), 0, false},
		{"nil err", nil, 0, false},
		{"capped at 300", errors.New("retry after 9999"), 300, true},
		{"zero rejected", errors.New("retry after 0"), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRetryAfter(tc.err)
			if ok != tc.wantOK {
				t.Fatalf("ok mismatch: got %v want %v", ok, tc.wantOK)
			}
			if tc.wantOK && got != time.Duration(tc.wantSecs)*time.Second {
				t.Fatalf("duration mismatch: got %s want %ds", got, tc.wantSecs)
			}
		})
	}
}

func TestIsRateLimit(t *testing.T) {
	if !IsRateLimit(errors.New("Too Many Requests: retry after 1")) {
		t.Fatal("expected true")
	}
	if IsRateLimit(errors.New("forbidden")) {
		t.Fatal("expected false")
	}
	if IsRateLimit(nil) {
		t.Fatal("expected false for nil")
	}
}

func TestIsPermanentSendFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// True positives — these phrases ARE in the Telegram Bot API
		// error stream and DO mean "give up on this recipient".
		{"blocked by user", errors.New("Forbidden: bot was blocked by the user"), true},
		{"deactivated", errors.New("Forbidden: user is deactivated"), true},
		{"chat not found", errors.New("Bad Request: chat not found"), true},
		{"never started", errors.New("Forbidden: bot can't initiate conversation with a user"), true},
		{"kicked from group", errors.New("Forbidden: bot was kicked from the supergroup chat"), true},
		{"write forbidden", errors.New("Bad Request: CHAT_WRITE_FORBIDDEN"), true},
		{"case-insensitive", errors.New("FORBIDDEN: BOT WAS BLOCKED BY THE USER"), true},

		// True negatives — DON'T skip the recipient on these.
		// This is the regression we're guarding: a bare "forbidden"
		// from a transient global issue (token rotation, edge proxy)
		// must NOT be treated as a per-user permanent failure.
		{"bare forbidden", errors.New("403 Forbidden"), false},
		{"rate limit", errors.New("Too Many Requests: retry after 5"), false},
		{"network", errors.New("connection refused"), false},
		{"timeout", errors.New("context deadline exceeded"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPermanentSendFailure(tc.err); got != tc.want {
				t.Fatalf("IsPermanentSendFailure(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}
