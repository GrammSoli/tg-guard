package worker

import (
	"testing"
	"time"
)

// TestNotificationWindow_AdmitsNearTermSubscriptions is a regression test
// for "added today, due tomorrow — no reminder". The old now+20h lower
// bound dropped any subscription created less than ~20h before its
// payment: it was never observed by a worker tick while it sat 20-30h
// out, so it never became a candidate. The window must admit a payment
// however soon it falls, leaving the precise filtering to shouldSendNow.
func TestNotificationWindow_AdmitsNearTermSubscriptions(t *testing.T) {
	now := time.Date(2026, 5, 16, 14, 0, 0, 0, time.UTC)
	start, end := notificationWindow(now)

	// Every payment from "right now" out to ~48h must be a candidate.
	for _, hoursOut := range []int{0, 1, 4, 10, 20, 24, 30, 47} {
		due := now.Add(time.Duration(hoursOut) * time.Hour)
		if due.Before(start) || due.After(end) {
			t.Errorf("payment %dh out (%s) falls outside window [%s, %s]",
				hoursOut, due.Format(time.RFC3339),
				start.Format(time.RFC3339), end.Format(time.RFC3339))
		}
	}

	// Payments clearly outside any user's "tomorrow" stay excluded.
	if past := now.Add(-time.Hour); !past.Before(start) {
		t.Errorf("a past payment (%s) should be before window start %s",
			past.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	if far := now.Add(60 * time.Hour); !far.After(end) {
		t.Errorf("a payment 60h out (%s) should be after window end %s",
			far.Format(time.RFC3339), end.Format(time.RFC3339))
	}
}
