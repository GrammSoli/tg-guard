package bot

import "testing"

// TestParseStatsCallback covers the stats-dashboard callback parsing,
// including the default section/period fallbacks for missing parts.
func TestParseStatsCallback(t *testing.T) {
	tests := []struct {
		data        string
		wantSection string
		wantPeriod  string
	}{
		{"admin_stats", "aud", "7d"},          // bare → defaults
		{"admin_stats:mon", "mon", "7d"},      // section only
		{"admin_stats:mon:30d", "mon", "30d"}, // fully specified
		{"admin_stats:svc:1d", "svc", "1d"},
		{"admin_stats::30d", "aud", "30d"}, // empty section → default
		{"admin_stats:act:", "act", "7d"},  // empty period → default
	}
	for _, tt := range tests {
		section, period := parseStatsCallback(tt.data)
		if section != tt.wantSection || period != tt.wantPeriod {
			t.Errorf("parseStatsCallback(%q) = (%q, %q), want (%q, %q)",
				tt.data, section, period, tt.wantSection, tt.wantPeriod)
		}
	}
}
