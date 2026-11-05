package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func TestBlockedReleaseCannotBeApprovedUntilUnblocked(t *testing.T) {
	r, db, w, _ := candidateAPIFixture(t)
	base := fmt.Sprintf("/api/watchers/%d", w.ID)

	rec := authRequest(r, http.MethodPut, base+"/versions/v2/block", `{"reason":"  "}`, "watcher")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("blank reason: %d %s", rec.Code, rec.Body.String())
	}
	rec = authRequest(r, http.MethodPut, base+"/versions/v2/block", `{"reason":"breaks checkout"}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}

	rec = authRequest(r, http.MethodPost, base+"/approve", `{"version":"v2"}`, "watcher")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "breaks checkout") {
		t.Fatalf("approve blocked: %d %s", rec.Code, rec.Body.String())
	}
	if err := db.First(&w, w.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w.ApprovedVersion != "" || w.Status != "pending_approval" {
		t.Fatalf("blocked release was approved: %+v", w)
	}

	rec = authRequest(r, http.MethodGet, base+"/versions", "", "watcher")
	var list struct {
		Marks []database.ReleaseMark `json:"marks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Marks) != 1 || list.Marks[0].Version != "v2" || list.Marks[0].BlockedAt == nil || list.Marks[0].BlockReason != "breaks checkout" {
		t.Fatalf("versions did not list the block: %+v", list.Marks)
	}

	rec = authRequest(r, http.MethodDelete, base+"/versions/v2/block", "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("unblock: %d %s", rec.Code, rec.Body.String())
	}
	rec = authRequest(r, http.MethodPost, base+"/approve", `{"version":"v2"}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve after unblock: %d %s", rec.Code, rec.Body.String())
	}
}

func TestBlockedReleaseRejectsRollbackAndRedeploy(t *testing.T) {
	r, db, w, _ := candidateAPIFixture(t)
	base := fmt.Sprintf("/api/watchers/%d", w.ID)
	if err := db.Model(&w).UpdateColumns(map[string]any{"current_version": "v2", "status": "healthy", "pending_version": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(w.InstallDir, "releases", "v1"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"v1", "v2"} {
		rec := authRequest(r, http.MethodPut, base+"/versions/"+version+"/block", `{"reason":"bad build"}`, "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("block %s: %d %s", version, rec.Code, rec.Body.String())
		}
	}

	rec := authRequest(r, http.MethodPost, base+"/rollback", `{"version":"v1"}`, "watcher")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "release v1 is blocked") {
		t.Fatalf("rollback to blocked: %d %s", rec.Code, rec.Body.String())
	}
	rec = authRequest(r, http.MethodPost, base+"/redeploy", "", "watcher")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "release v2 is blocked") {
		t.Fatalf("redeploy of blocked current: %d %s", rec.Code, rec.Body.String())
	}

	var attempts int64
	db.Model(&database.DeployLog{}).Where("watcher_id = ?", w.ID).Count(&attempts)
	if attempts != 0 {
		t.Fatalf("blocked operations opened %d attempts", attempts)
	}
	if err := db.First(&w, w.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w.CurrentVersion != "v2" {
		t.Fatalf("refused redeploy cleared current_version: %q", w.CurrentVersion)
	}
}

func TestBlockedCatalogReleaseCannotBeSelectedOrDeployed(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", InstallDir: t.TempDir(), Paused: true}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db, "nssm", t.TempDir(), "test", "", ".env", &config.AppConfig{}, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), nil, nil, nil)
	h.catalogFactory = func(string, *agent.Logger) catalogClient { return &fakeCatalog{} }
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	registerAPIRoutes(router.Group("/api"), h)
	base := fmt.Sprintf("/api/watchers/%d", watcher.ID)

	rec := authRequest(router, http.MethodPost, base+"/releases/download", `{"release_id":11,"asset_id":22}`, "watcher")
	if rec.Code != http.StatusCreated {
		t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
	}
	var artifact database.CatalogArtifact
	if err := json.Unmarshal(rec.Body.Bytes(), &artifact); err != nil {
		t.Fatal(err)
	}
	rec = authRequest(router, http.MethodPut, base+"/versions/v2/block", `{"reason":"leaks memory"}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}

	for _, action := range []string{"candidate", "deploy"} {
		rec = authRequest(router, http.MethodPost, fmt.Sprintf("%s/releases/%d/%s", base, artifact.ID, action), "", "watcher")
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "leaks memory") {
			t.Fatalf("%s blocked release: %d %s", action, rec.Code, rec.Body.String())
		}
	}
	var live database.Watcher
	if err := db.First(&live, watcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	if live.PendingCatalogID != 0 || live.PendingVersion != "" {
		t.Fatalf("blocked release became a candidate: %+v", live)
	}
	var attempts int64
	db.Model(&database.DeployLog{}).Where("watcher_id = ?", watcher.ID).Count(&attempts)
	if attempts != 0 {
		t.Fatalf("blocked catalog release opened %d attempts", attempts)
	}

	rec = authRequest(router, http.MethodGet, base+"/releases", "", "watcher")
	var catalog struct {
		Marks []database.ReleaseMark `json:"marks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Marks) != 1 || catalog.Marks[0].Tag != "v2" || catalog.Marks[0].BlockedAt == nil {
		t.Fatalf("catalog did not include the block with its tag: %+v", catalog.Marks)
	}
}
