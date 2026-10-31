package agent

import (
	"math/rand/v2"
	"time"
)

// nextPollDelay returns the check interval spread by ±10%, so watchers started
// together drift apart instead of hitting GitHub in lockstep.
func nextPollDelay(intervalSec int) time.Duration {
	if intervalSec <= 0 {
		intervalSec = 60
	}
	interval := time.Duration(intervalSec) * time.Second
	spread := interval / 5
	return interval - spread/2 + rand.N(spread+1)
}
