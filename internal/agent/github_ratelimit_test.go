package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newRateLimitTestClient keeps the production transport stack (unlike newTestClient)
// and pins the limiter clock so blocks can be expired deterministically.
func newRateLimitTestClient(token string, server *httptest.Server) (*GitHubClient, *time.Time) {
	c := NewGitHubClient(token, newTestLogger())
	c.apiBase = server.URL
	now := time.Unix(1_700_000_000, 0)
	c.limiter.now = func() time.Time { return now }
	return c, &now
}

func stubSleep(t *testing.T, now *time.Time) *time.Duration {
	t.Helper()
	var slept time.Duration
	original := sleepContext
	sleepContext = func(ctx context.Context, d time.Duration) error {
		slept += d
		*now = now.Add(d)
		return ctx.Err()
	}
	t.Cleanup(func() { sleepContext = original })
	return &slept
}

func writeRelease(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v1.0.0"})
}

func TestRateLimitTransport_PrimaryLimitBlocksUntilReset(t *testing.T) {
	var hits, limited atomic.Int32
	limited.Store(1)
	var reset int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if limited.Load() == 1 {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
			w.Header().Set("X-RateLimit-Resource", "core")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
			return
		}
		writeRelease(w)
	}))
	defer server.Close()
	client, now := newRateLimitTestClient("token", server)
	reset = now.Add(2 * time.Minute).Unix()

	_, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	var limitErr *RateLimitError
	if !errors.As(err, &limitErr) || limitErr.Secondary || !limitErr.Until.Equal(time.Unix(reset, 0)) || limitErr.Resource != "core" {
		t.Fatalf("unexpected rate-limit details: %+v", limitErr)
	}
	if strings.Contains(err.Error(), "repo scope") {
		t.Fatalf("rate limit must not be reported as a token scope problem: %v", err)
	}

	if _, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected local block, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("blocked request reached the network: hits=%d", got)
	}

	limited.Store(0)
	*now = time.Unix(reset, 0).Add(time.Second)
	if _, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r"); err != nil {
		t.Fatalf("expected request after reset to succeed, got %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("hits=%d, want 2", got)
	}
}

func TestRateLimitTransport_RetryAfterIsSecondary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client, now := newRateLimitTestClient("token", server)

	_, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
	var limitErr *RateLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected RateLimitError, got %v", err)
	}
	if !limitErr.Secondary || !limitErr.Until.Equal(now.Add(30*time.Second)) {
		t.Fatalf("unexpected rate-limit details: %+v", limitErr)
	}
}

func TestRateLimitTransport_SecondaryMessageBacksOffExponentially(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."}`))
	}))
	defer server.Close()
	client, now := newRateLimitTestClient("token", server)

	for _, want := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		_, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
		var limitErr *RateLimitError
		if !errors.As(err, &limitErr) {
			t.Fatalf("expected RateLimitError, got %v", err)
		}
		if !limitErr.Secondary || !limitErr.Until.Equal(now.Add(want)) {
			t.Fatalf("backoff until=%s, want now+%s", limitErr.Until, want)
		}
		*now = limitErr.Until.Add(time.Second)
	}
}

func TestRateLimitTransport_PlainForbiddenIsNotRateLimited(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by personal access token"}`))
	}))
	defer server.Close()
	client, _ := newRateLimitTestClient("token", server)

	for range 2 {
		_, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
		if err == nil || errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected a non-rate-limit error, got %v", err)
		}
		if !strings.Contains(err.Error(), "repo scope") {
			t.Fatalf("expected token scope hint, got %v", err)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("plain 403 must not block later requests: hits=%d", got)
	}
}

func TestDownloadArtifact_WaitsForRateLimitWithoutSpendingAttempt(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer server.Close()
	client, now := newRateLimitTestClient("", server)
	slept := stubSleep(t, now)
	dest := filepath.Join(t.TempDir(), "artifact.zip")

	if err := client.DownloadArtifact(context.Background(), server.URL+"/artifact.zip", dest, 1); err != nil {
		t.Fatalf("expected download to succeed after waiting, got %v", err)
	}
	if *slept != 5*time.Second {
		t.Fatalf("slept %s, want 5s", *slept)
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "zip-bytes" {
		t.Fatalf("unexpected artifact: %q, %v", data, err)
	}
}

func TestDownloadArtifact_ReturnsLongRateLimitImmediately(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client, now := newRateLimitTestClient("", server)
	slept := stubSleep(t, now)

	err := client.DownloadArtifact(context.Background(), server.URL+"/artifact.zip", filepath.Join(t.TempDir(), "a.zip"), 3)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if *slept != 0 || hits.Load() != 1 {
		t.Fatalf("expected no waiting and one request, slept=%s hits=%d", *slept, hits.Load())
	}
}
