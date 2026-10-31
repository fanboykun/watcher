package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// serveWithETag answers 304 when the client already holds etag, counting each outcome.
func serveWithETag(w http.ResponseWriter, r *http.Request, etag string, body any, full, notModified *atomic.Int32) {
	if r.Header.Get("If-None-Match") == etag {
		notModified.Add(1)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	full.Add(1)
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

func newCacheTestClient(pool *GitHubPool, token string, server *httptest.Server) *GitHubClient {
	c := pool.Client(token, newTestLogger())
	c.apiBase = server.URL
	return c
}

func TestCachingTransport_RevalidatesWithETag(t *testing.T) {
	var full, notModified atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWithETag(w, r, `"v1"`, githubRelease{TagName: "v1.2.3"}, &full, &notModified)
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	for range 3 {
		release, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
		if err != nil {
			t.Fatal(err)
		}
		if release.TagName != "v1.2.3" {
			t.Fatalf("tag=%q, want v1.2.3", release.TagName)
		}
	}
	if full.Load() != 1 || notModified.Load() != 2 {
		t.Fatalf("full=%d notModified=%d, want 1 and 2", full.Load(), notModified.Load())
	}
}

func TestCachingTransport_DoesNotShareCacheAcrossTokens(t *testing.T) {
	var full, notModified atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWithETag(w, r, `"v1"`, githubRelease{TagName: "v1"}, &full, &notModified)
	}))
	defer server.Close()
	pool := NewGitHubPool()

	for _, token := range []string{"a", "b"} {
		if _, err := newCacheTestClient(pool, token, server).FetchLatestRelease(context.Background(), "https://github.com/o/r"); err != nil {
			t.Fatal(err)
		}
	}
	if full.Load() != 2 || notModified.Load() != 0 {
		t.Fatalf("full=%d notModified=%d, want 2 and 0", full.Load(), notModified.Load())
	}
}

func TestCachingTransport_SkipsBinaryAssetDownloads(t *testing.T) {
	var conditional atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" {
			conditional.Add(1)
		}
		w.Header().Set("ETag", `"zip"`)
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	for range 2 {
		req, _ := client.newRequest(context.Background(), http.MethodGet, server.URL+"/assets/1")
		req.Header.Set("Accept", "application/octet-stream")
		resp, err := client.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if conditional.Load() != 0 {
		t.Fatalf("binary downloads must not be revalidated")
	}
}

func TestCachingTransport_MergesConcurrentRequests(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-release
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v1"})
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			r, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
			if err == nil && r.TagName != "v1" {
				t.Errorf("tag=%q", r.TagName)
			}
			errs <- err
		})
	}
	time.Sleep(200 * time.Millisecond) // let every caller join the in-flight request
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("hits=%d, want 1 merged request", got)
	}
}

func TestCachingTransport_MergedCallerSurvivesLeaderCancellation(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			<-r.Context().Done() // hold the leader until it is canceled
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v1"})
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		_, _ = client.FetchLatestRelease(leaderCtx, "https://github.com/o/r")
	}()
	for hits.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	followerDone := make(chan error, 1)
	go func() {
		_, err := client.FetchLatestRelease(context.Background(), "https://github.com/o/r")
		followerDone <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the follower join the leader's flight
	cancel()
	<-leaderDone
	if err := <-followerDone; err != nil {
		t.Fatalf("follower inherited leader cancellation: %v", err)
	}
}

func TestFetchMetadata_VersionJSONReusesAssetWhenReleaseUnchanged(t *testing.T) {
	var full, notModified, assetDownloads atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			serveWithETag(w, r, `"rel"`, githubRelease{TagName: "v2", Assets: []githubAsset{
				{ID: 7, Name: "version.json", URL: server.URL + "/assets/7", UpdatedAt: "2026-10-01T00:00:00Z", Size: 64},
			}}, &full, &notModified)
		case "/assets/7":
			assetDownloads.Add(1)
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{"api": {Version: "v2"}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	for range 3 {
		meta, err := client.FetchMetadata(context.Background(), "https://github.com/o/r/releases/latest/download/version.json")
		if err != nil {
			t.Fatal(err)
		}
		if meta.Services["api"].Version != "v2" {
			t.Fatalf("unexpected metadata: %+v", meta)
		}
	}
	if assetDownloads.Load() != 1 || full.Load() != 1 || notModified.Load() != 2 {
		t.Fatalf("assetDownloads=%d full=%d notModified=%d, want 1, 1, 2", assetDownloads.Load(), full.Load(), notModified.Load())
	}
}

func TestFetchServiceMetadata_RepoModeSecondPollIsAllRevalidations(t *testing.T) {
	var full, notModified atomic.Int32
	latest := githubRelease{TagName: "alpha-api/v2", Assets: []githubAsset{{Name: "alpha-api-v2-windows-amd64.zip"}}}
	older := githubRelease{TagName: "beta-api/v1", Assets: []githubAsset{{Name: "beta-api-v1-windows-amd64.zip"}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			serveWithETag(w, r, `"latest"`, latest, &full, &notModified)
		case "/repos/o/r/releases":
			serveWithETag(w, r, `"list"`, []githubRelease{latest, older}, &full, &notModified)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newCacheTestClient(NewGitHubPool(), "token", server)

	for range 2 {
		meta, err := client.FetchServiceMetadataForRelease(context.Background(), "https://github.com/o/r", "latest", "beta-api")
		if err != nil {
			t.Fatal(err)
		}
		if meta.Services["beta-api"].Version != "beta-api/v1" {
			t.Fatalf("unexpected metadata: %+v", meta.Services)
		}
	}
	if full.Load() != 2 || notModified.Load() != 2 {
		t.Fatalf("full=%d notModified=%d, want 2 and 2", full.Load(), notModified.Load())
	}
}

func TestLRUCache_EvictsLeastRecentlyUsed(t *testing.T) {
	cache := newLRUCache[int](2)
	cache.put("a", 1)
	cache.put("b", 2)
	cache.get("a")
	cache.put("c", 3)
	if _, ok := cache.get("b"); ok {
		t.Fatal("expected b to be evicted")
	}
	if v, ok := cache.get("a"); !ok || v != 1 {
		t.Fatal("expected a to survive")
	}
}
