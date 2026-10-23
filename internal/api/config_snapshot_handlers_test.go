package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func TestWatcherConfigSnapshotReturnsHistoricalManagedFiles(t *testing.T) {
	router, db, watcher, svc := candidateAPIFixture(t)
	version := "release/v2"
	wcfg := agent.WatcherConfigFromDB(&watcher)
	wcfg.Services = []agent.ServiceConfig{{WindowsServiceName: "old-service", EnvFile: "old.env", EnvContent: "OLD=true", ConfigFiles: []agent.ConfigFile{
		{FilePath: "config/old.json", Target: "app_dir", Content: "old app"},
		{FilePath: "web.config", Target: "release_dir", Content: "old release"},
	}}}
	if err := agent.CaptureConfigSnapshot(wcfg, version, agent.SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(agent.ConfigSnapshotPath(watcher.InstallDir, version), "manifest.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&svc).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/watcher/api/watchers/%d/snapshots/%s/config", watcher.ID, url.PathEscape(version))
	rec := authRequest(router, http.MethodGet, path, "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot: %d %s", rec.Code, rec.Body.String())
	}
	var got agent.ConfigSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != version || len(got.Services) != 1 || got.Services[0].ServiceName != "old-service" || len(got.Services[0].Files) != 3 {
		t.Fatalf("missing historical service/files: %+v", got)
	}
	want := map[string]string{"env/old.env": "OLD=true", "app_dir/config/old.json": "old app", "release_dir/web.config": "old release"}
	for _, file := range got.Services[0].Files {
		if content, ok := want[file.Target+"/"+file.FilePath]; !ok || content != file.Content {
			t.Fatalf("unexpected historical file: %+v", file)
		}
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("historical inspection changed snapshot: %v", err)
	}
	var serviceCount int64
	if err := db.Model(&database.Service{}).Where("watcher_id = ?", watcher.ID).Count(&serviceCount).Error; err != nil || serviceCount != 0 {
		t.Fatalf("historical read changed active services: %d %v", serviceCount, err)
	}
}

func TestWatcherConfigSnapshotMissingIsNotActiveConfig(t *testing.T) {
	router, _, watcher, _ := candidateAPIFixture(t)
	path := fmt.Sprintf("/api/watchers/%d/snapshots/v99/config", watcher.ID)
	rec := authRequest(router, http.MethodGet, path, "", "watcher")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing snapshot substituted active configuration: %d %s", rec.Code, rec.Body.String())
	}
}
