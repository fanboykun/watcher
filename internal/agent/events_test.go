package agent

import (
	"testing"
	"time"
)

func TestWatcherEventBusSubscribePublish(t *testing.T) {
	bus := NewWatcherEventBus()

	ch, unsubscribe := bus.Subscribe(42)
	defer unsubscribe()

	bus.Publish(42, WatcherEvent{Type: EventStatusChanged, Data: map[string]any{"status": "healthy"}})

	select {
	case ev := <-ch:
		if ev.Type != EventStatusChanged {
			t.Fatalf("unexpected event type: got %q", ev.Type)
		}
		if ev.WatcherID != 42 {
			t.Fatalf("unexpected watcher id: got %d", ev.WatcherID)
		}
		if ev.Timestamp.IsZero() {
			t.Fatalf("expected timestamp to be populated")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for published event")
	}
}

func TestWatcherEventBusWatcherIsolation(t *testing.T) {
	bus := NewWatcherEventBus()

	chA, unsubA := bus.Subscribe(1)
	defer unsubA()
	chB, unsubB := bus.Subscribe(2)
	defer unsubB()

	bus.Publish(1, WatcherEvent{Type: EventPollEvent})

	select {
	case <-chA:
		// expected
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("watcher 1 subscriber did not receive event")
	}

	select {
	case ev := <-chB:
		t.Fatalf("watcher 2 subscriber should not receive watcher 1 event, got %+v", ev)
	case <-time.After(100 * time.Millisecond):
		// expected no delivery
	}
}

func TestWatcherEventBusUnsubscribeStopsDelivery(t *testing.T) {
	bus := NewWatcherEventBus()
	ch, unsubscribe := bus.Subscribe(7)

	unsubscribe()
	bus.Publish(7, WatcherEvent{Type: EventVersionChanged})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("channel should be closed after unsubscribe")
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatalf("expected closed channel after unsubscribe")
	}
}

func TestWatcherEventBusBurstKeepsRecentEvents(t *testing.T) {
	bus := NewWatcherEventBus()
	ch, unsubscribe := bus.Subscribe(9)
	defer unsubscribe()

	// Publish more than channel capacity to exercise non-blocking drop path.
	for i := 0; i < 200; i++ {
		bus.Publish(9, WatcherEvent{
			Type: EventDeployStarted,
			Data: map[string]any{"seq": i},
		})
	}

	lastSeq := -1
	read := 0
	for {
		select {
		case ev := <-ch:
			if ev.Data == nil {
				t.Fatalf("event data should not be nil")
			}
			v, ok := ev.Data["seq"]
			if !ok {
				t.Fatalf("missing seq field")
			}
			seq, ok := v.(int)
			if !ok {
				t.Fatalf("seq should be int, got %T", v)
			}
			lastSeq = seq
			read++
		default:
			goto done
		}
	}

done:
	if read == 0 {
		t.Fatalf("expected buffered events after burst publish")
	}
	if lastSeq < 150 {
		t.Fatalf("expected to retain recent events, got last seq=%d", lastSeq)
	}
}
