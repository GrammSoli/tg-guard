// Package timezone is a process-wide cache around time.LoadLocation.
//
// time.LoadLocation parses the binary tzdata file on every call. Workers
// iterate over thousands of rows belonging to ~30 distinct zones in
// practice, so the cache is keyed by zone NAME (not user/room ID):
// 1M users sharing 30 zones cost 30 cache entries total.
package timezone

import (
	"sync"
	"time"
)

var cache sync.Map // map[string]*time.Location

// LoadOrUTC returns the *time.Location for an IANA name, or time.UTC for
// empty / unknown / malformed values. Never returns nil — callers can
// use the result directly. The UTC fallback is cached under the bogus
// name too, so a permanently-broken zone string costs one syscall total,
// not one per worker tick.
func LoadOrUTC(name string) *time.Location {
	if name == "" || name == "UTC" {
		return time.UTC
	}
	if v, ok := cache.Load(name); ok {
		return v.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil || loc == nil {
		cache.Store(name, time.UTC)
		return time.UTC
	}
	cache.Store(name, loc)
	return loc
}

// Validate reports whether name is a non-empty, OS-recognised IANA zone.
// Used by handler validation; the cache is populated as a side-effect so
// the next worker tick reuses the parsed Location without an OS read.
func Validate(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := cache.Load(name); ok {
		return true
	}
	loc, err := time.LoadLocation(name)
	if err != nil || loc == nil {
		return false
	}
	cache.Store(name, loc)
	return true
}
