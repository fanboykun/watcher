package agent

import (
	"sync"
	"time"
)

// PollingMonitor tracks live loops. Configuration alone cannot prove a loop is running.
type PollingMonitor struct {
	mu    sync.RWMutex
	next  uint64
	loops map[uint]pollingLoop
}

type pollingLoop struct {
	generation uint64
	checking   bool
	startedAt  *time.Time
}

func NewPollingMonitor() *PollingMonitor { return &PollingMonitor{loops: make(map[uint]pollingLoop)} }

func (m *PollingMonitor) register(id uint) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	m.loops[id] = pollingLoop{generation: m.next}
	return m.next
}

func (m *PollingMonitor) checking(id uint, generation uint64, checking bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	loop, ok := m.loops[id]
	if !ok || loop.generation != generation {
		return
	}
	loop.checking = checking
	loop.startedAt = nil
	if checking {
		now := time.Now().UTC()
		loop.startedAt = &now
	}
	m.loops[id] = loop
}

func (m *PollingMonitor) stop(id uint, generation uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loops[id].generation == generation {
		delete(m.loops, id)
	}
}

func (m *PollingMonitor) Snapshot(id uint, paused bool) (string, *time.Time) {
	if m == nil {
		if paused {
			return "paused", nil
		}
		return "unknown", nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	loop, exists := m.loops[id]
	if !exists {
		return "stopped", nil
	}
	if loop.checking {
		return "checking", loop.startedAt
	}
	if paused {
		return "paused", nil
	}
	return "active", nil
}
