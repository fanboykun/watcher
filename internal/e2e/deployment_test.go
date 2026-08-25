//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func TestWatcherDeploymentEndToEnd(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)
	release := newReleaseFixture(t, "app", "v1")
	fixture := newE2EFixture(t, false)

	created := fixture.createWatcher(t, release.metadataURL, healthServer.URL)
	watcher := fixture.waitForWatcher(t, created.ID, func(w database.Watcher) bool {
		return w.Status == "healthy" && w.CurrentVersion == "v1"
	})

	if len(watcher.Services) != 1 {
		t.Fatalf("services = %d, want 1", len(watcher.Services))
	}
	service := watcher.Services[0]
	if service.StartArguments != "--serve" || service.EnvContent != "APP_MODE=e2e\n" || service.PublicURL != healthServer.URL {
		t.Errorf("inline service fields were not persisted: %+v", service)
	}
	if len(service.ConfigFiles) != 2 {
		t.Fatalf("config files = %d, want 2", len(service.ConfigFiles))
	}

	if got := readFileString(t, filepath.Join(fixture.installDir, "current", "app.exe")); got != "e2e-binary" {
		t.Errorf("current binary = %q, want e2e-binary", got)
	}
	if got := readFileString(t, filepath.Join(fixture.installDir, ".env")); got != "APP_MODE=e2e\n" {
		t.Errorf("managed env = %q", got)
	}
	if got := readFileString(t, filepath.Join(fixture.installDir, "config", "app.txt")); got != "app-config" {
		t.Errorf("app config = %q", got)
	}
	if got := readFileString(t, filepath.Join(fixture.installDir, "current", "config", "release.txt")); got != "release-config" {
		t.Errorf("release config = %q", got)
	}
	snapshotRoot := agent.ConfigSnapshotPath(fixture.installDir, "v1")
	if got := readFileString(t, filepath.Join(snapshotRoot, "services", "e2e-app-service", "env", ".env")); got != "APP_MODE=e2e\n" {
		t.Errorf("snapshot env = %q", got)
	}
	if got := readFileString(t, filepath.Join(snapshotRoot, "services", "e2e-app-service", "app", "config", "app.txt")); got != "app-config" {
		t.Errorf("snapshot app config = %q", got)
	}
	if got := readFileString(t, filepath.Join(snapshotRoot, "services", "e2e-app-service", "release", "config", "release.txt")); got != "release-config" {
		t.Errorf("snapshot release config = %q", got)
	}
	if _, err := os.Stat(filepath.Join(fixture.installDir, "current", ".watcher-snapshot")); !os.IsNotExist(err) {
		t.Errorf("snapshot should not be exposed through current: %v", err)
	}

	var attempts []database.DeployLog
	if err := fixture.db.Where("watcher_id = ?", watcher.ID).Order("id asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load deploy attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].Kind != "deploy" || attempts[0].Status != "succeeded" {
		t.Fatalf("deploy attempts = %+v, want one succeeded deploy", attempts)
	}
	var pollEvents []database.PollEvent
	if err := fixture.db.Where("watcher_id = ?", watcher.ID).Order("id asc").Find(&pollEvents).Error; err != nil {
		t.Fatalf("load poll events: %v", err)
	}
	if len(pollEvents) == 0 || pollEvents[0].Status != "new_release" {
		t.Fatalf("poll events = %+v, want new_release", pollEvents)
	}

	var apiWatcher database.Watcher
	fixture.doJSON(t, http.MethodGet, fmt.Sprintf("/api/watchers/%d", watcher.ID), nil, http.StatusOK, &apiWatcher)
	if apiWatcher.Status != "healthy" || apiWatcher.CurrentVersion != "v1" {
		t.Errorf("watcher API state = %+v", apiWatcher)
	}
	var versionsResponse map[string]any
	fixture.doJSON(t, http.MethodGet, fmt.Sprintf("/api/watchers/%d/versions", watcher.ID), nil, http.StatusOK, &versionsResponse)
	versionsText := fmt.Sprint(versionsResponse)
	assertContainsAll(t, versionsText, "v1", "true")

	nssmCalls := readFileString(t, fixture.nssmLogPath)
	assertContainsAll(t, nssmCalls,
		"install e2e-app-service",
		"set e2e-app-service AppDirectory",
		"set e2e-app-service AppParameters --serve",
		"start e2e-app-service",
		"status e2e-app-service",
	)
	if release.metadataRequests.Load() == 0 || release.artifactRequests.Load() == 0 {
		t.Errorf("release requests: metadata=%d artifact=%d", release.metadataRequests.Load(), release.artifactRequests.Load())
	}
}

func TestFirstDeploymentFailureEndToEndDoesNotInventRollback(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)
	release := newReleaseFixture(t, "app", "v1")
	fixture := newE2EFixture(t, true)

	created := fixture.createWatcher(t, release.metadataURL, healthServer.URL)
	watcher := fixture.waitForWatcher(t, created.ID, func(w database.Watcher) bool {
		return w.Status == "failed" && strings.TrimSpace(w.LastError) != ""
	})
	if watcher.CurrentVersion != "" {
		t.Errorf("current version = %q, want empty after failed first deploy", watcher.CurrentVersion)
	}
	assertContainsAll(t, watcher.LastError, "start service e2e-app-service", "no previous version to roll back to")

	var attempts []database.DeployLog
	if err := fixture.db.Where("watcher_id = ?", watcher.ID).Order("id asc").Find(&attempts).Error; err != nil {
		t.Fatalf("load deploy attempts: %v", err)
	}
	if len(attempts) != 1 {
		t.Fatalf("deploy attempts = %+v, want exactly one failed deployment", attempts)
	}
	if attempts[0].Kind != "deploy" || attempts[0].Status != "failed" || attempts[0].FailurePhase != "start_services" {
		t.Errorf("failed attempt = %+v", attempts[0])
	}
	var rollbackCount int64
	if err := fixture.db.Model(&database.DeployLog{}).Where("watcher_id = ? AND kind = ?", watcher.ID, "rollback").Count(&rollbackCount).Error; err != nil {
		t.Fatalf("count rollback attempts: %v", err)
	}
	if rollbackCount != 0 {
		t.Errorf("rollback attempts = %d, want 0 on first deployment", rollbackCount)
	}

	var deployResponse struct {
		Data []database.DeployLog `json:"data"`
	}
	fixture.doJSON(t, http.MethodGet, fmt.Sprintf("/api/watchers/%d/deploys", watcher.ID), nil, http.StatusOK, &deployResponse)
	if len(deployResponse.Data) != 1 || deployResponse.Data[0].Status != "failed" {
		t.Errorf("deploy API response = %+v", deployResponse.Data)
	}
}
