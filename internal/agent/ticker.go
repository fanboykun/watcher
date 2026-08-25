package agent

import "time"

// newTicker creates a ticker with a safe minimum interval.
func newTicker(intervalSec int) *time.Ticker {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	return time.NewTicker(time.Duration(intervalSec) * time.Second)
}
