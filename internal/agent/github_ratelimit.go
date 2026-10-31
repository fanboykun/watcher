package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrRateLimited matches every GitHub primary or secondary rate-limit refusal.
var ErrRateLimited = errors.New("github rate limited")

const (
	// GitHub asks clients to wait at least a minute after an unannotated secondary limit.
	secondaryRateLimitBackoff    = time.Minute
	maxSecondaryRateLimitBackoff = 15 * time.Minute
	// maxRateLimitWait bounds how long a download waits for quota before deferring.
	maxRateLimitWait  = 15 * time.Minute
	rateLimitBodyPeek = 64 << 10
)

// RateLimitError reports when GitHub will accept requests from this token again.
type RateLimitError struct {
	Resource  string
	Until     time.Time
	Secondary bool
}

func (e *RateLimitError) Error() string {
	kind := "primary"
	if e.Secondary {
		kind = "secondary"
	}
	resource := e.Resource
	if resource == "" {
		resource = "core"
	}
	return fmt.Sprintf("GitHub %s rate limit reached (%s); resumes at %s", kind, resource, e.Until.UTC().Format(time.RFC3339))
}

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// rateLimiter remembers a token's rate-limit block so later requests fail locally
// instead of spending more of the exhausted quota.
type rateLimiter struct {
	mu               sync.Mutex
	now              func() time.Time
	blockedErr       *RateLimitError
	secondaryStrikes int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{now: time.Now}
}

// blocked returns the active block, if any.
func (l *rateLimiter) blocked() *RateLimitError {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.blockedErr == nil || !l.now().Before(l.blockedErr.Until) {
		return nil
	}
	blocked := *l.blockedErr
	return &blocked
}

// classify inspects a response and records a block when it is a rate-limit refusal.
func (l *rateLimiter) classify(resp *http.Response) *RateLimitError {
	l.mu.Lock()
	defer l.mu.Unlock()
	if resp.StatusCode < 400 {
		l.secondaryStrikes = 0
		return nil
	}
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	now := l.now()
	limitErr := &RateLimitError{Resource: resp.Header.Get("X-RateLimit-Resource")}
	exhausted := resp.Header.Get("X-RateLimit-Remaining") == "0"
	if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), now); ok {
		limitErr.Until, limitErr.Secondary = now.Add(wait), !exhausted
	} else if exhausted {
		limitErr.Until = now.Add(secondaryRateLimitBackoff)
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			limitErr.Until = time.Unix(reset, 0)
		}
	} else if resp.StatusCode == http.StatusTooManyRequests || mentionsRateLimit(resp) {
		backoff := min(secondaryRateLimitBackoff<<l.secondaryStrikes, maxSecondaryRateLimitBackoff)
		l.secondaryStrikes++
		limitErr.Until, limitErr.Secondary = now.Add(backoff), true
	} else {
		return nil
	}
	if !limitErr.Until.After(now) {
		limitErr.Until = now.Add(time.Second)
	}
	l.blockedErr = limitErr
	blocked := *limitErr
	return &blocked
}

// parseRetryAfter accepts both delay-seconds and HTTP-date forms.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

// mentionsRateLimit peeks at a 403 body for GitHub's secondary-limit message and
// restores the body so non-rate-limit errors are still readable by callers.
func mentionsRateLimit(resp *http.Response) bool {
	if resp.Body == nil {
		return false
	}
	peek, _ := io.ReadAll(io.LimitReader(resp.Body, rateLimitBodyPeek))
	resp.Body = readCloser{Reader: io.MultiReader(bytes.NewReader(peek), resp.Body), Closer: resp.Body}
	return bytes.Contains(bytes.ToLower(peek), []byte("rate limit"))
}

type readCloser struct {
	io.Reader
	io.Closer
}

// rateLimitTransport turns rate-limit responses into *RateLimitError and refuses
// requests locally while the token is blocked.
type rateLimitTransport struct {
	base    http.RoundTripper
	limiter *rateLimiter
}

func (t rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if limitErr := t.limiter.blocked(); limitErr != nil {
		if req.Body != nil {
			req.Body.Close()
		}
		return nil, limitErr
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if limitErr := t.limiter.classify(resp); limitErr != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, rateLimitBodyPeek))
		resp.Body.Close()
		return nil, limitErr
	}
	return resp, nil
}

// sleepContext waits for d or until ctx ends; tests replace it to avoid real waits.
var sleepContext = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
