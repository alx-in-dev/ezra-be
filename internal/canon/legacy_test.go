package canon

import (
	"testing"
	"time"
)

func TestLegacyState(t *testing.T) {
	h := time.Hour
	// L1: stable < 24h, fading 24–48h, offline ≥ 48h.
	if got := LegacyState(1, 1*h); got != LegacyStable {
		t.Fatalf("L1 @1h = %s, want stable", got)
	}
	if got := LegacyState(1, 23*h); got != LegacyStable {
		t.Fatalf("L1 @23h = %s, want stable", got)
	}
	if got := LegacyState(1, 30*h); got != LegacyFading {
		t.Fatalf("L1 @30h = %s, want fading", got)
	}
	if got := LegacyState(1, 49*h); got != LegacyOffline {
		t.Fatalf("L1 @49h = %s, want offline", got)
	}
	// L5: stable window is longer (48h) → still stable at 40h where L1 is offline.
	if got := LegacyState(5, 40*h); got != LegacyStable {
		t.Fatalf("L5 @40h = %s, want stable", got)
	}
	if got := LegacyState(5, 60*h); got != LegacyFading {
		t.Fatalf("L5 @60h = %s, want fading (stable 48h, fading 48–72h)", got)
	}
	if got := LegacyState(5, 80*h); got != LegacyOffline {
		t.Fatalf("L5 @80h = %s, want offline", got)
	}
}
