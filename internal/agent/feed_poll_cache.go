package agent

import (
	"context"
	"sync"
	"time"
)

// feedPollCache remembers the last manifest a watcher read from its feed, so
// an unchanged feed costs a conditional request (HTTP) or a stat (file share)
// instead of a download. It lives with the watcher's goroutine, which restarts
// on any configuration change, so validators never outlive the source they
// describe. A process restart costs one full fetch.
type feedPollCache struct {
	mu    sync.Mutex
	entry *feedPollEntry
}

type feedPollEntry struct {
	key          string // manifest location
	etag         string
	lastModified string
	modTime      time.Time // file share feeds
	size         int64
	meta         *VersionMetadata // artifact locations already resolved
}

func newFeedPollCache() *feedPollCache {
	return &feedPollCache{}
}

// lookup returns the cached entry for key, or nil when there is none or the
// poll asked for a fresh read.
func (c *feedPollCache) lookup(ctx context.Context, key string) *feedPollEntry {
	if c == nil || freshFetchRequested(ctx) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entry == nil || c.entry.key != key {
		return nil
	}
	entry := *c.entry
	return &entry
}

func (c *feedPollCache) store(entry feedPollEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entry = &entry
}

// unchanged returns a copy of the cached manifest marked as not modified.
func (e *feedPollEntry) unchanged() *VersionMetadata {
	meta := cloneMetadata(e.meta)
	meta.notModified = true
	return meta
}

// cloneMetadata copies a manifest so the cache never shares a map with a poll.
func cloneMetadata(meta *VersionMetadata) *VersionMetadata {
	out := &VersionMetadata{Services: make(map[string]ServiceMeta, len(meta.Services))}
	for name, svc := range meta.Services {
		out.Services[name] = svc
	}
	return out
}

// freshFetchRequested reports a manual check, which always rereads the feed so
// an operator can force re-evaluation.
func freshFetchRequested(ctx context.Context) bool {
	return TraceFromContext(ctx).TriggeredBy == "manual"
}
