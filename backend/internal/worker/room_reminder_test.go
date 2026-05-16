package worker

import (
	"testing"
	"time"
)

// TestBillsTomorrow checks the day-before-billing trigger, including the
// short-month clamping that must match BillingResetWorker.
func TestBillsTomorrow(t *testing.T) {
	tests := []struct {
		name       string
		billingDay int
		now        time.Time
		want       bool
	}{
		{"eve of a mid-month billing day", 15, date(2026, 5, 14), true},
		{"two days before", 15, date(2026, 5, 13), false},
		{"the billing day itself is not an eve", 15, date(2026, 5, 15), false},
		{"billing_day 1 — eve is the last day of the previous month", 1, date(2026, 5, 31), true},
		{"billing_day 31 in a 31-day month — eve is the 30th", 31, date(2026, 5, 30), true},
		{"billing_day 31 in a 30-day month — clamped, eve is the 29th", 31, date(2026, 6, 29), true},
		{"billing_day 31 in a 30-day month — the 30th itself is not an eve", 31, date(2026, 6, 30), false},
		{"billing_day 31 in February — clamped to the 28th, eve is the 27th", 31, date(2026, 2, 27), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := billsTomorrow(tt.billingDay, tt.now); got != tt.want {
				t.Errorf("billsTomorrow(%d, %s) = %v, want %v",
					tt.billingDay, tt.now.Format("2006-01-02"), got, tt.want)
			}
		})
	}
}

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 9, 0, 0, 0, time.UTC)
}
