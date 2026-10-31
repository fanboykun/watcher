package agent

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const (
	maxCachedResponses    = 256
	maxCachedResponseSize = 2 << 20
	maxCachedAssets       = 64
	maxCachedAssetSize    = 1 << 20
)

// githubBucket is the state shared by every client using one token.
type githubBucket struct {
	limiter   *rateLimiter
	responses *lruCache[cachedResponse]
	assets    *lruCache[[]byte]
	flights   flightGroup
}

func newGitHubBucket() *githubBucket {
	return &githubBucket{
		limiter:   newRateLimiter(),
		responses: newLRUCache[cachedResponse](maxCachedResponses),
		assets:    newLRUCache[[]byte](maxCachedAssets),
	}
}

// cachedResponse is a fully buffered response that can be replayed to many callers.
type cachedResponse struct {
	status int
	header http.Header
	body   []byte
}

func (c cachedResponse) response(req *http.Request) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", c.status, http.StatusText(c.status)),
		StatusCode:    c.status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        c.header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(c.body)),
		ContentLength: int64(len(c.body)),
		Request:       req,
	}
}

// cachingTransport revalidates GitHub API JSON with ETags — an authenticated 304
// does not count against the rate limit — and merges identical in-flight GETs.
// Binary asset downloads (Accept: application/octet-stream) bypass it.
type cachingTransport struct {
	base   http.RoundTripper
	bucket *githubBucket
}

func (t cachingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	accept := req.Header.Get("Accept")
	if req.Method != http.MethodGet || !strings.Contains(accept, "application/vnd.github") {
		return t.base.RoundTrip(req)
	}
	key := req.URL.String() + "\n" + accept
	for {
		result, err := t.bucket.flights.do(req.Context(), key, func() (cachedResponse, error) {
			return t.fetch(req, key)
		})
		// A merged caller must not inherit another caller's cancellation.
		if err != nil && req.Context().Err() == nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return result.response(req), nil
	}
}

func (t cachingTransport) fetch(req *http.Request, key string) (cachedResponse, error) {
	entry, cached := t.bucket.responses.get(key)
	out := req
	if cached {
		out = req.Clone(req.Context())
		out.Header.Set("If-None-Match", entry.header.Get("ETag"))
	}
	resp, err := t.base.RoundTrip(out)
	if err != nil {
		return cachedResponse{}, err
	}
	defer resp.Body.Close()
	if cached && resp.StatusCode == http.StatusNotModified {
		return entry, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return cachedResponse{}, err
	}
	result := cachedResponse{status: resp.StatusCode, header: resp.Header.Clone(), body: body}
	if resp.StatusCode == http.StatusOK && resp.Header.Get("ETag") != "" && len(body) <= maxCachedResponseSize {
		t.bucket.responses.put(key, result)
	}
	return result, nil
}

// flightGroup runs one call per key at a time; concurrent callers share its result.
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	val  cachedResponse
	err  error
}

func (g *flightGroup) do(ctx context.Context, key string, fn func() (cachedResponse, error)) (cachedResponse, error) {
	g.mu.Lock()
	if call, ok := g.calls[key]; ok {
		g.mu.Unlock()
		select {
		case <-call.done:
			return call.val, call.err
		case <-ctx.Done():
			return cachedResponse{}, ctx.Err()
		}
	}
	if g.calls == nil {
		g.calls = make(map[string]*flightCall)
	}
	call := &flightCall{done: make(chan struct{})}
	g.calls[key] = call
	g.mu.Unlock()

	call.val, call.err = fn()
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	close(call.done)
	return call.val, call.err
}

// lruCache is a small, size-bounded, concurrency-safe LRU map.
type lruCache[V any] struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[string]*list.Element
}

type lruEntry[V any] struct {
	key   string
	value V
}

func newLRUCache[V any](max int) *lruCache[V] {
	return &lruCache[V]{max: max, order: list.New(), items: make(map[string]*list.Element)}
}

func (c *lruCache[V]) get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*lruEntry[V]).value, true
	}
	var zero V
	return zero, false
}

func (c *lruCache[V]) put(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*lruEntry[V]).value = value
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&lruEntry[V]{key: key, value: value})
	if c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry[V]).key)
	}
}
