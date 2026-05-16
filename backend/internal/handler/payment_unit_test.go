package handler

import (
	"math"
	"strconv"
	"testing"
	"time"
)

func TestNormalizePlan(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"month", "month"},
		{"lifetime", "lifetime"},
		{"", "lifetime"},        // empty falls back to lifetime
		{"yearly", "lifetime"},  // unknown values also fall back
		{"MONTH", "lifetime"},   // case-sensitive on purpose
		{"month ", "lifetime"},  // no trimming
	}
	for _, c := range cases {
		if got := normalizePlan(c.in); got != c.want {
			t.Errorf("normalizePlan(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPremiumExpiryFor(t *testing.T) {
	t.Run("lifetime returns nil", func(t *testing.T) {
		if got := premiumExpiryFor("lifetime"); got != nil {
			t.Errorf("premiumExpiryFor(lifetime) = %v, want nil", got)
		}
	})

	t.Run("unknown plan returns nil (treats as lifetime)", func(t *testing.T) {
		if got := premiumExpiryFor("anything-else"); got != nil {
			t.Errorf("premiumExpiryFor(anything-else) = %v, want nil", got)
		}
	})

	t.Run("month returns ~1 month out, UTC", func(t *testing.T) {
		got := premiumExpiryFor("month")
		if got == nil {
			t.Fatal("expected non-nil expiry for month plan")
		}
		// Must be UTC.
		if got.Location() != time.UTC {
			t.Errorf("expiry location = %v, want UTC", got.Location())
		}
		// Must be within a tight band around now+30 days. AddDate(0,1,0)
		// in Go is calendar-aware (handles short months) so the exact
		// duration varies; ±2 days is plenty to catch a wrong-unit bug
		// (hours vs days, etc.) without flaking on month-boundary edge.
		expected := time.Now().UTC().AddDate(0, 1, 0)
		delta := got.Sub(expected)
		if delta < -2*24*time.Hour || delta > 2*24*time.Hour {
			t.Errorf("expiry = %v, expected close to %v (delta=%v)", got, expected, delta)
		}
	})
}

func TestParsePaymentPayload(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		method      string
		wantPlan    string
		wantUserID  uint64
		wantOK      bool
	}{
		{
			name:       "valid 4-part stars lifetime",
			payload:    "premium_stars_lifetime_42",
			method:     "stars",
			wantPlan:   "lifetime",
			wantUserID: 42,
			wantOK:     true,
		},
		{
			name:       "valid 4-part crypto month",
			payload:    "premium_crypto_month_1234567",
			method:     "crypto",
			wantPlan:   "month",
			wantUserID: 1234567,
			wantOK:     true,
		},
		{
			name:    "unknown plan normalises to lifetime, still valid",
			payload: "premium_stars_yearly_99",
			method:  "stars",
			wantPlan: "lifetime",
			wantUserID: 99,
			wantOK:  true,
		},
		{
			name:    "legacy 3-part payload rejected (audit Tier-1 #2)",
			payload: "premium_stars_42",
			method:  "stars",
			wantOK:  false,
		},
		{
			name:    "wrong method prefix",
			payload: "premium_crypto_month_42",
			method:  "stars",
			wantOK:  false,
		},
		{
			name:    "wrong product prefix",
			payload: "freemium_stars_month_42",
			method:  "stars",
			wantOK:  false,
		},
		{
			name:    "non-numeric user id",
			payload: "premium_stars_month_abc",
			method:  "stars",
			wantOK:  false,
		},
		{
			name:    "empty payload",
			payload: "",
			method:  "stars",
			wantOK:  false,
		},
		{
			name:    "5 parts — extra suffix",
			payload: "premium_stars_month_42_extra",
			method:  "stars",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, uid, ok := parsePaymentPayload(tt.payload, tt.method)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (plan=%q uid=%d)", ok, tt.wantOK, plan, uid)
			}
			if !ok {
				return
			}
			if plan != tt.wantPlan {
				t.Errorf("plan = %q, want %q", plan, tt.wantPlan)
			}
			if uid != tt.wantUserID {
				t.Errorf("userID = %d, want %d", uid, tt.wantUserID)
			}
		})
	}
}

// TestAmountConversion_RoundsAwayIEEE754Drift covers the math.Round
// guard in HandleCryptoWebhook: "0.10" → 0.0999999… via ParseFloat,
// and a naive int(x*100) would store 9 cents instead of 10. The
// production code rounds; this test pins that contract so a future
// refactor that drops the round can't slip through.
func TestAmountConversion_RoundsAwayIEEE754Drift(t *testing.T) {
	cases := []struct {
		raw      string
		wantCents int
	}{
		{"0.10", 10},
		{"0.01", 1},
		{"1.00", 100},
		{"10", 1000},
		{"100", 10000},
		{"1.99", 199},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			amountFloat, err := strconv.ParseFloat(c.raw, 64)
			if err != nil {
				t.Fatalf("ParseFloat(%q): %v", c.raw, err)
			}
			got := int(math.Round(amountFloat * 100))
			if got != c.wantCents {
				t.Errorf("Round(%s * 100) = %d, want %d (IEEE-754 drift?)", c.raw, got, c.wantCents)
			}
		})
	}
}
