package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newRateLimitedWatcher points a version.json watcher at server and returns its runtime.
func newRateLimitedWatcher(t *testing.T, server *httptest.Server) (*RepoWatcher, *gorm.DB, uint) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}, &database.PollEvent{}); err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{
		Name: "alpha", ServiceName: "api", MetadataURL: server.URL + "/version.json", InstallDir: t.TempDir(),
		CheckIntervalSec: 60, DownloadRetries: 3, MaxKeptVersions: 3,
		CurrentVersion: "v1", Status: string(StatusHealthy), AutoDeploy: true,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	rw := NewRepoWatcher(&watcher, db, &config.AppConfig{}, newTestLogger(), nil, nil)
	rw.github = NewGitHubPool() // keep this test's rate-limit blocks out of the process-wide pool
	return rw, db, watcher.ID
}

func TestDeploy_RateLimitedDownloadOpensNoGitHubDeployment(t *testing.T) {
	var deployments atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deployments") {
			deployments.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42})
			return
		}
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	rw, _, _ := newRateLimitedWatcher(t, server)
	rw.global = &config.AppConfig{Environment: "production", GitHubToken: "configured-token", GitHubDeployEnabled: true}
	rw.wcfg.MetadataURL = "https://github.com/example/app" // resolvable repo, so GitHub deployments stay enabled
	gh, _ := newRateLimitTestClient("", server)

	err := rw.deploy(context.Background(), gh, ServiceMeta{Version: "v2", ArtifactURL: server.URL + "/artifact.zip"}, "v2", "v1")
	var deferred *deployDeferredError
	if !errors.As(err, &deferred) {
		t.Fatalf("expected deployDeferredError, got %v", err)
	}
	if got := deployments.Load(); got != 0 {
		t.Fatalf("GitHub deployment API called %d times for a deferred download", got)
	}
}

func TestRepoWatcherRun_RateLimitedMetadataSkipsPoll(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	rw, db, watcherID := newRateLimitedWatcher(t, server)

	for range 3 {
		if err := rw.Run(context.Background()); err != nil {
			t.Fatalf("rate-limited poll should not fail, got %v", err)
		}
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("polls after the block should not reach GitHub: hits=%d", got)
	}

	var watcher database.Watcher
	if err := db.First(&watcher, watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.LastPollStatus != "rate_limited" || watcher.Status != string(StatusHealthy) {
		t.Fatalf("last_poll_status=%q status=%q", watcher.LastPollStatus, watcher.Status)
	}
	var deploys int64
	db.Model(&database.DeployLog{}).Count(&deploys)
	if deploys != 0 {
		t.Fatalf("expected no deploy attempts, got %d", deploys)
	}
}

func TestRepoWatcherRun_LowQuotaSkipsDeployBeforeOpeningAttempt(t *testing.T) {
	var downloads atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "3")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{
				"api": {Version: "v2", ArtifactURL: server.URL + "/artifact.zip"},
			}})
		default:
			downloads.Add(1)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	rw, db, watcherID := newRateLimitedWatcher(t, server)

	if err := rw.Run(context.Background()); err != nil {
		t.Fatalf("low quota should not fail the poll, got %v", err)
	}
	var deploys int64
	db.Model(&database.DeployLog{}).Where("watcher_id = ?", watcherID).Count(&deploys)
	if deploys != 0 || downloads.Load() != 0 {
		t.Fatalf("expected no attempt and no download, got deploys=%d downloads=%d", deploys, downloads.Load())
	}
	var watcher database.Watcher
	if err := db.First(&watcher, watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.LastPollStatus != "rate_limited" || watcher.Status != string(StatusHealthy) {
		t.Fatalf("last_poll_status=%q status=%q", watcher.LastPollStatus, watcher.Status)
	}
}

func TestRepoWatcherRun_RateLimitedDownloadDefersWithoutFailure(t *testing.T) {
	var downloads atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{
				"api": {Version: "v2", ArtifactURL: server.URL + "/artifact.zip"},
			}})
		case "/artifact.zip":
			downloads.Add(1)
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	rw, db, watcherID := newRateLimitedWatcher(t, server)
	clock := time.Unix(1_700_000_000, 0)
	rw.github.bucket("").limiter.now = func() time.Time { return clock }

	// One more poll than maxDeployRetries: deferrals must never suspend the release.
	for i := range 4 {
		if err := rw.Run(context.Background()); err != nil {
			t.Fatalf("poll %d: rate-limited deploy should not fail, got %v", i+1, err)
		}
		clock = clock.Add(2 * time.Hour) // let the block expire so the next poll reaches GitHub
	}

	if got := downloads.Load(); got != 4 {
		t.Fatalf("expected every poll to retry the download, got %d", got)
	}
	var logs []database.DeployLog
	if err := db.Where("watcher_id = ?", watcherID).Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 4 {
		t.Fatalf("expected 4 deploy attempts, got %d", len(logs))
	}
	for _, l := range logs {
		if l.Kind != "deploy" || l.Status != "deferred" || l.CompletedAt == nil || !strings.Contains(l.Error, "rate limit") {
			t.Fatalf("unexpected attempt: kind=%q status=%q completed=%v error=%q", l.Kind, l.Status, l.CompletedAt, l.Error)
		}
	}
	if failures := rw.state.ConsecutiveFailuresForVersion("v2"); failures != 0 {
		t.Fatalf("deferrals counted as failures: %d", failures)
	}
	var watcher database.Watcher
	if err := db.First(&watcher, watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.Status != string(StatusHealthy) || watcher.LastError != "" || watcher.CurrentVersion != "v1" || watcher.LastPollStatus != "rate_limited" {
		t.Fatalf("watcher status=%q last_error=%q current=%q last_poll=%q", watcher.Status, watcher.LastError, watcher.CurrentVersion, watcher.LastPollStatus)
	}
	var suspended int64
	db.Model(&database.PollEvent{}).Where("watcher_id = ? AND status = ?", watcherID, "deploy_suspended").Count(&suspended)
	if suspended != 0 {
		t.Fatalf("release was suspended after deferrals")
	}
}
