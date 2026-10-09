//go:build e2e

package e2e_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/api"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type e2eFixture struct {
	db          *gorm.DB
	server      *httptest.Server
	installDir  string
	nssmLogPath string
	cancel      context.CancelFunc
	agentDone   <-chan struct{}
}

type releaseFixture struct {
	server           *httptest.Server
	metadataURL      string
	metadataRequests atomic.Int32
	artifactRequests atomic.Int32
}

func newReleaseFixture(t *testing.T, serviceName, version string) *releaseFixture {
	t.Helper()
	artifact := zipArtifact(t, map[string]string{"app.exe": "e2e-binary"})
	fixture := &releaseFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			fixture.metadataRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"services": map[string]any{
					serviceName: map[string]any{
						"version":      version,
						"artifact":     "app.zip",
						"artifact_url": fixture.server.URL + "/app.zip",
						"published_at": "2026-08-25T00:00:00Z",
					},
				},
			})
		case "/app.zip":
			fixture.artifactRequests.Add(1)
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	fixture.metadataURL = fixture.server.URL + "/version.json"
	t.Cleanup(fixture.server.Close)
	return fixture
}

func zipArtifact(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func newE2EFixture(t *testing.T, failStart bool) *e2eFixture {
	t.Helper()
	root := t.TempDir()
	installDir := filepath.Join(root, "application")
	nssmPath, nssmLogPath := writeFakeNSSM(t, root, failStart)
	db, err := database.NewDB(filepath.Join(root, "watcher.db"))
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	db.Logger = db.Logger.LogMode(logger.Silent)

	cfg := &config.AppConfig{
		Environment:             "e2e",
		GitHubDeployEnabled:     false,
		LogDir:                  filepath.Join(root, "logs"),
		LogLevel:                "error",
		LogMaxSizeMB:            10,
		NssmPath:                nssmPath,
		DBPath:                  filepath.Join(root, "watcher.db"),
		APIPort:                 "0",
		WatcherRepoURL:          "https://github.com/fanboykun/watcher",
		WatcherServiceName:      "watcher-e2e",
		WebhookTimeoutSec:       1,
		WebhookRetryScheduleSec: "0",
	}
	checkTrigger := make(chan agent.CheckTrigger, 10)
	syncTrigger := make(chan struct{}, 10)
	events := agent.NewWatcherEventBus()
	log := agent.NewLoggerWithWriter("e2e", io.Discard, "error")
	router := api.NewRouter(
		db,
		nssmPath,
		cfg.LogDir,
		"e2e",
		"",
		filepath.Join(root, ".env"),
		cfg,
		log,
		events,
		checkTrigger,
		syncTrigger,
		nil,
		make(chan struct{}, 1),
	)
	server := httptest.NewServer(router)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	runtimeAgent := agent.NewAgent(db, cfg, log, events, checkTrigger, syncTrigger, nil)
	go func() {
		defer close(done)
		runtimeAgent.Run(ctx)
	}()

	fixture := &e2eFixture{
		db:          db,
		server:      server,
		installDir:  installDir,
		nssmLogPath: nssmLogPath,
		cancel:      cancel,
		agentDone:   done,
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("agent did not stop after cancellation")
		}
		server.Close()
	})
	return fixture
}

func writeFakeNSSM(t *testing.T, root string, failStart bool) (string, string) {
	t.Helper()
	stateDir := filepath.Join(root, "nssm-state")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatalf("create NSSM state dir: %v", err)
	}
	logPath := filepath.Join(root, "nssm-calls.log")
	failValue := "0"
	if failStart {
		failValue = "1"
	}
	script := fmt.Sprintf(`#!/bin/sh
set -eu
state_dir=%q
log_file=%q
fail_start=%q
printf '%%s\n' "$*" >> "$log_file"
action="${1:-}"
service="${2:-unknown}"
state_file="$state_dir/$service.state"
case "$action" in
  status)
    if [ ! -f "$state_file" ]; then
      echo "SERVICE_DOES_NOT_EXIST"
      exit 3
    fi
    cat "$state_file"
    ;;
  install)
    echo "SERVICE_STOPPED" > "$state_file"
    echo "installed"
    ;;
  set)
    echo "configured"
    ;;
  start|continue)
    if [ "$fail_start" = "1" ]; then
      echo "The service cannot be started"
      exit 1
    fi
    echo "SERVICE_RUNNING" > "$state_file"
    echo "SERVICE_RUNNING"
    ;;
  stop)
    echo "SERVICE_STOPPED" > "$state_file"
    echo "SERVICE_STOPPED"
    ;;
  remove)
    rm -f "$state_file"
    echo "removed"
    ;;
  *)
    echo "unsupported fake NSSM action: $action"
    exit 2
    ;;
esac
`, stateDir, logPath, failValue)
	path := filepath.Join(root, "fake-nssm")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write fake NSSM: %v", err)
	}
	return path, logPath
}

func (f *e2eFixture) createWatcher(t *testing.T, metadataURL, healthURL string) database.Watcher {
	t.Helper()
	payload := map[string]any{
		"name":               "e2e-app",
		"service_name":       "app",
		"metadata_url":       metadataURL,
		"release_ref":        "latest",
		"check_interval_sec": 3600,
		"download_retries":   1,
		"install_dir":        f.installDir,
		"hc_enabled":         true,
		"hc_retries":         1,
		"hc_interval_sec":    1,
		"hc_timeout_sec":     1,
		"max_kept_versions":  3,
		"services": []map[string]any{
			{
				"service_type":         "nssm",
				"windows_service_name": "e2e-app-service",
				"binary_name":          "app.exe",
				"start_arguments":      "--serve",
				"env_file":             ".env",
				"env_content":          "APP_MODE=e2e\n",
				"health_check_url":     healthURL,
				"public_url":           healthURL,
				"config_files": []map[string]string{
					{"file_path": "config/app.txt", "target": "app_dir", "content": "app-config"},
					{"file_path": "config/release.txt", "target": "release_dir", "content": "release-config"},
				},
			},
		},
	}
	var watcher database.Watcher
	f.doJSON(t, http.MethodPost, "/api/watchers", payload, http.StatusCreated, &watcher)
	if watcher.ID == 0 {
		t.Fatal("create watcher returned zero ID")
	}
	return watcher
}

func (f *e2eFixture) doJSON(t *testing.T, method, path string, body any, wantStatus int, target any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+database.DefaultAuthPassword)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, path, resp.StatusCode, wantStatus, data)
	}
	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("decode response %s: %v", data, err)
		}
	}
}

func (f *e2eFixture) waitForWatcher(t *testing.T, watcherID uint, predicate func(database.Watcher) bool) database.Watcher {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var watcher database.Watcher
	for time.Now().Before(deadline) {
		err := f.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, watcherID).Error
		if err == nil && predicate(watcher) {
			return watcher
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("watcher %d did not reach expected state; last status=%q version=%q error=%q", watcherID, watcher.Status, watcher.CurrentVersion, watcher.LastError)
	return database.Watcher{}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func assertContainsAll(t *testing.T, value string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			t.Errorf("value does not contain %q:\n%s", fragment, value)
		}
	}
}
