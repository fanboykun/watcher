package agent

import (
	"testing"
	"time"
)

func TestNextPollDelay_SpreadsWithinTenPercent(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 1000 {
		d := nextPollDelay(100)
		if d < 90*time.Second || d > 110*time.Second {
			t.Fatalf("delay %s outside 100s ±10%%", d)
		}
		seen[d] = true
	}
	if len(seen) < 2 {
		t.Fatal("expected jittered delays")
	}
	if d := nextPollDelay(0); d < 54*time.Second || d > 66*time.Second {
		t.Fatalf("default delay %s outside 60s ±10%%", d)
	}
}
