package timezone

import (
	"testing"
	"time"
)

func TestLoadOrUTC_KnownZone(t *testing.T) {
	loc := LoadOrUTC("Europe/Moscow")
	if loc == nil {
		t.Fatal("LoadOrUTC returned nil for a valid zone")
	}
	if loc.String() != "Europe/Moscow" {
		t.Errorf("LoadOrUTC(\"Europe/Moscow\") = %q, want \"Europe/Moscow\"", loc.String())
	}
}

func TestLoadOrUTC_EmptyAndUTC(t *testing.T) {
	for _, name := range []string{"", "UTC"} {
		loc := LoadOrUTC(name)
		if loc != time.UTC {
			t.Errorf("LoadOrUTC(%q) = %v, want time.UTC", name, loc)
		}
	}
}

func TestLoadOrUTC_GarbageFallsBack(t *testing.T) {
	loc := LoadOrUTC("Bogus/Not_A_Zone")
	if loc != time.UTC {
		t.Errorf("LoadOrUTC garbage = %v, want time.UTC", loc)
	}
	// Second call must hit cache and still return UTC — regression guard
	// against the cache storing nil and breaking subsequent lookups.
	if loc2 := LoadOrUTC("Bogus/Not_A_Zone"); loc2 != time.UTC {
		t.Errorf("LoadOrUTC garbage (cached) = %v, want time.UTC", loc2)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"Europe/Moscow", true},
		{"Asia/Tokyo", true},
		{"UTC", true},
		{"", false},
		{"Not/A/Zone", false},
		{"random nonsense", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Validate(tt.name); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
