package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func TestRollbackConfigShowsDraftsAndRejectsUnsafeDraft(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	base := fmt.Sprintf("/api/watchers/%d", w.ID)
	if err := db.Model(&w).UpdateColumns(map[string]any{"current_version": "v2", "status": "healthy", "pending_version": ""}).Error; err != nil {
		t.Fatal(err)
	}
	w.Services = []database.Service{svc}
	if err := agent.CaptureConfigSnapshot(agent.WatcherConfigFromDB(&w), "v1", agent.SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(w.InstallDir, "releases", "v1"), 0755); err != nil {
		t.Fatal(err)
	}

	// Drafts are saved through the candidate endpoint, targeted at the retained version.
	body := fmt.Sprintf(`{"target_version":"v1","services":[{"service_id":%d,"env_content":"VALUE=edited","config_files":[]}]}`, svc.ID)
	if rec := authRequest(r, http.MethodPut, base+"/candidate", body, "watcher"); rec.Code != http.StatusOK {
		t.Fatalf("save draft: %d %s", rec.Code, rec.Body.String())
	}
	rec := authRequest(r, http.MethodGet, base+"/versions/v1/config", "", "watcher")
	var config agent.RollbackConfig
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &config) != nil {
		t.Fatalf("config: %d %s", rec.Code, rec.Body.String())
	}
	if len(config.Services) != 1 || config.Services[0].SnapshotEnv != "VALUE=active" || !config.Services[0].HasDraft || config.Services[0].DraftEnv != "VALUE=edited" {
		t.Fatalf("unexpected config: %+v", config.Services)
	}
	rec = authRequest(r, http.MethodGet, base+"/versions", "", "watcher")
	var list struct {
		DraftVersions []string `json:"draft_versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.DraftVersions) != 1 || list.DraftVersions[0] != "v1" {
		t.Fatalf("draft_versions = %v (%v)", list.DraftVersions, err)
	}

	// A draft that cannot be applied is rejected before any service is touched.
	unsafe := []database.RevisionConfigFile{{FilePath: "../escape.json", Target: "app_dir", Content: "{}"}}
	var draft database.ServiceConfigRevision
	if err := db.Where("service_id = ? AND target_version = ?", svc.ID, "v1").First(&draft).Error; err != nil {
		t.Fatal(err)
	}
	draft.ConfigFiles = &unsafe
	if err := db.Save(&draft).Error; err != nil {
		t.Fatal(err)
	}
	rec = authRequest(r, http.MethodPost, base+"/rollback", `{"version":"v1","use_draft":true}`, "watcher")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsafe draft rollback: %d %s", rec.Code, rec.Body.String())
	}
	var attempts int64
	db.Model(&database.DeployLog{}).Where("watcher_id = ?", w.ID).Count(&attempts)
	if attempts != 0 {
		t.Fatalf("rejected rollback left %d attempts", attempts)
	}
	rec = authRequest(r, http.MethodGet, base+"/versions/v1/config", "", "watcher")
	if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil || config.Services[0].SnapshotEnv != "VALUE=active" {
		t.Fatalf("rejected rollback changed the snapshot: %+v (%v)", config.Services, err)
	}
}
