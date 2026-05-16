package worker

import (
	"testing"
	"time"

	"github.com/subguard/backend/internal/model"
)

// TestRoomDueForResetNow exercises the per-room timezone gate: the worker
// must fire only when the room's OWN clock is in the 00:00–01:59 window
// of its billing day.
func TestRoomDueForResetNow(t *testing.T) {
	tests := []struct {
		name       string
		tz         string
		billingDay int
		nowUTC     time.Time
		want       bool
	}{
		{
			name:       "Auckland UTC+13 — local 00:30 on billing day fires",
			tz:         "Pacific/Auckland",
			billingDay: 15,
			// 14 May 11:30 UTC = 15 May 00:30 in Auckland (NZST UTC+12 in
			// southern-hemisphere winter; the test uses a winter date to
			// keep DST out of the calculation).
			nowUTC: time.Date(2026, 5, 14, 12, 30, 0, 0, time.UTC),
			want:   true,
		},
		{
			name:       "Auckland UTC+12 (winter) — 10:30 UTC = 22:30 local on billing day, past the 00-01 window",
			tz:         "Pacific/Auckland",
			billingDay: 14,
			nowUTC:     time.Date(2026, 5, 14, 10, 30, 0, 0, time.UTC),
			want:       false,
		},
		{
			name:       "UTC room — 00:30 UTC on billing day fires",
			tz:         "UTC",
			billingDay: 10,
			nowUTC:     time.Date(2026, 6, 10, 0, 30, 0, 0, time.UTC),
			want:       true,
		},
		{
			name:       "UTC room — 02:00 UTC is past the window",
			tz:         "UTC",
			billingDay: 10,
			nowUTC:     time.Date(2026, 6, 10, 2, 0, 0, 0, time.UTC),
			want:       false,
		},
		{
			name:       "billing_day 31 clamps to last day of a 30-day month",
			tz:         "UTC",
			billingDay: 31,
			nowUTC:     time.Date(2026, 6, 30, 0, 30, 0, 0, time.UTC),
			want:       true,
		},
		{
			name:       "Honolulu UTC-10 — local 01:00 fires while UTC clock reads 11:00",
			tz:         "Pacific/Honolulu",
			billingDay: 5,
			nowUTC:     time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC),
			want:       true,
		},
		{
			name:       "garbage timezone falls back to UTC behaviour",
			tz:         "Bogus/Zone",
			billingDay: 10,
			nowUTC:     time.Date(2026, 6, 10, 0, 30, 0, 0, time.UTC),
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			room := &model.SharedRoom{
				Timezone:   tt.tz,
				BillingDay: tt.billingDay,
			}
			if got := roomDueForResetNow(room, tt.nowUTC); got != tt.want {
				t.Errorf("roomDueForResetNow(tz=%s, day=%d, %s UTC) = %v, want %v",
					tt.tz, tt.billingDay, tt.nowUTC.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}
