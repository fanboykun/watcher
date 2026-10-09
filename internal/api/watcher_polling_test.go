package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestWatcherPollingStatusIndependentOfDeployment(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "polling.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	watcher := database.Watcher{Name: "poller", ServiceName: "app", Status: "healthy", LastPollStatus: "error", LastPollError: "metadata unavailable", LastPollAt: &now, LastPollID: "previous-poll"}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, nil, nil, nil, nil, agent.NewPollingMonitor())
	for _, path := range []string{"/api/watchers", "/api/watchers/1"} {
		recorder := authRequest(router, http.MethodGet, path, "", database.DefaultAuthPassword)
		if recorder.Code != 200 {
			t.Fatalf("%s: %d %s", path, recorder.Code, recorder.Body)
		}
		var result database.Watcher
		if path == "/api/watchers" {
			var results []database.Watcher
			if err := json.Unmarshal(recorder.Body.Bytes(), &results); err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 {
				t.Fatal(results)
			}
			result = results[0]
		} else if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "healthy" || result.LastPollStatus != "error" || result.LastPollError != watcher.LastPollError || result.PollingActivity != "stopped" || result.LastPollAt == nil {
			t.Fatalf("polling conflated with deployment: %+v", result)
		}
	}
	if err := db.Model(&watcher).UpdateColumns(map[string]any{"last_poll_at": nil, "last_poll_status": "", "last_poll_error": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.PollEvent{WatcherID: watcher.ID, Status: "no_update", CheckedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	recorder := authRequest(router, http.MethodGet, "/api/watchers/1", "", database.DefaultAuthPassword)
	var historical database.Watcher
	if err := json.Unmarshal(recorder.Body.Bytes(), &historical); err != nil {
		t.Fatal(err)
	}
	if historical.LastPollStatus != "no_update" || historical.LastPollAt == nil {
		t.Fatalf("historical poll missing: %+v", historical)
	}

}
