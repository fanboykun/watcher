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
	// deployRequestBudget covers a deploy's GitHub calls: release lookup, asset
	// download, and the Deployment API create/in_progress/success statuses.
	deployRequestBudget = 5
)

// RateLimitError reports when GitHub will accept requests from this token again.
type RateLimitError struct {
	Resource  string
	Until     time.Time
	Secondary bool
	Reserve   bool // refused locally to keep the remaining quota for deployments
}

func (e *RateLimitError) Error() string {
	if e.Reserve {
		return fmt.Sprintf("GitHub quota nearly exhausted; polling paused to keep the rest for deployments until %s", e.Until.UTC().Format(time.RFC3339))
	}
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

// rateLimiter remembers a token's quota and rate-limit block so later requests
// fail locally instead of spending more of an exhausted quota.
type rateLimiter struct {
	mu               sync.Mutex
	now              func() time.Time
	blockedErr       *RateLimitError
	secondaryStrikes int
	// Last core quota GitHub reported; limit is 0 until a response carries it.
	limit, remaining int
	reset            time.Time
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

// snapshot reports the last known quota for display; it never contacts GitHub.
func (l *rateLimiter) snapshot() GitHubQuota {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	quota := GitHubQuota{Limit: l.limit, Remaining: l.remaining, Reserve: l.limit / 10}
	if now.Before(l.reset) {
		reset := l.reset.UTC()
		quota.ResetAt = &reset
	} else if l.limit > 0 {
		quota.Remaining = l.limit // the window rolled over since GitHub last reported
	}
	if l.blockedErr != nil && now.Before(l.blockedErr.Until) {
		until := l.blockedErr.Until.UTC()
		quota.BlockedUntil = &until
	}
	return quota
}

// shortfall reports when fewer than n requests are known to remain before reset.
func (l *rateLimiter) shortfall(n int) *RateLimitError {
	if blocked := l.blocked(); blocked != nil {
		return blocked
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit == 0 || l.remaining >= n || !l.now().Before(l.reset) {
		return nil
	}
	return &RateLimitError{Resource: "core", Until: l.reset}
}

// reserveExhausted reports when background polling should yield the last tenth
// of the quota to deployments and interactive requests.
func (l *rateLimiter) reserveExhausted() *RateLimitError {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit == 0 || l.remaining > l.limit/10 || !l.now().Before(l.reset) {
		return nil
	}
	return &RateLimitError{Resource: "core", Until: l.reset, Reserve: true}
}

// observe records the core quota headers GitHub attaches to API responses.
func (l *rateLimiter) observe(header http.Header) {
	if resource := header.Get("X-RateLimit-Resource"); resource != "" && resource != "core" {
		return
	}
	limit, limitErr := strconv.Atoi(header.Get("X-RateLimit-Limit"))
	remaining, remainingErr := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	reset, resetErr := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64)
	if limitErr != nil || remainingErr != nil || resetErr != nil || limit <= 0 {
		return
	}
	l.limit, l.remaining, l.reset = limit, remaining, time.Unix(reset, 0)
}

// classify inspects a response and records a block when it is a rate-limit refusal.
func (l *rateLimiter) classify(resp *http.Response) *RateLimitError {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.observe(resp.Header)
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
	limitErr := t.limiter.blocked()
	// Conditional polls stay allowed: an unchanged 304 costs no quota.
	if limitErr == nil && isGitHubPoll(req.Context()) && req.Header.Get("If-None-Match") == "" {
		limitErr = t.limiter.reserveExhausted()
	}
	if limitErr != nil {
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

type githubPollKey struct{}

// withGitHubPollPriority marks requests as background polling, which yields the
// last part of the quota to deployments and interactive requests.
func withGitHubPollPriority(ctx context.Context) context.Context {
	return context.WithValue(ctx, githubPollKey{}, true)
}

func isGitHubPoll(ctx context.Context) bool {
	poll, _ := ctx.Value(githubPollKey{}).(bool)
	return poll
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
