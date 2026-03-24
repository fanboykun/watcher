package agent

import (
	"sync"
	"time"
)

const (
	EventDeployStarted  = "deploy_started"
	EventDeployFinished = "deploy_finished"
	EventStatusChanged  = "status_changed"
	EventVersionChanged = "version_changed"
	EventPollEvent      = "poll_event"
)

// WatcherEvent is a compact watcher update payload suitable for SSE.
type WatcherEvent struct {
	Type      string         `json:"type"`
	WatcherID uint           `json:"watcher_id"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// WatcherEventBus provides bounded, non-blocking pub/sub by watcher ID.
type WatcherEventBus struct {
	mu   sync.RWMutex
	subs map[uint]map[chan WatcherEvent]struct{}
}

func NewWatcherEventBus() *WatcherEventBus {
	return &WatcherEventBus{
		subs: make(map[uint]map[chan WatcherEvent]struct{}),
	}
}

func (b *WatcherEventBus) Publish(watcherID uint, ev WatcherEvent) {
	b.mu.RLock()
	wSubs := b.subs[watcherID]
	if len(wSubs) == 0 {
		b.mu.RUnlock()
		return
	}
	if ev.WatcherID == 0 {
		ev.WatcherID = watcherID
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	for ch := range wSubs {
		// Non-blocking delivery. If full, drop one stale event and try once more.
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
	b.mu.RUnlock()
}

func (b *WatcherEventBus) Subscribe(watcherID uint) (<-chan WatcherEvent, func()) {
	ch := make(chan WatcherEvent, 24)
	b.mu.Lock()
	if _, ok := b.subs[watcherID]; !ok {
		b.subs[watcherID] = make(map[chan WatcherEvent]struct{})
	}
	b.subs[watcherID][ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		if wSubs, ok := b.subs[watcherID]; ok {
			if _, exists := wSubs[ch]; exists {
				delete(wSubs, ch)
				close(ch)
				if len(wSubs) == 0 {
					delete(b.subs, watcherID)
				}
			}
		}
		b.mu.Unlock()
	}
	return ch, unsubscribe
}
