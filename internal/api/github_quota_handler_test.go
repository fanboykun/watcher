package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func TestGitHubRateLimitsGroupsWatchersByToken(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "secret-override", "secret-override", "secret-global"} {
		watcher := database.Watcher{Name: "w", ServiceName: "api", MetadataURL: "https://github.com/o/r", InstallDir: t.TempDir(), GitHubToken: token}
		if err := db.Create(&watcher).Error; err != nil {
			t.Fatal(err)
		}
	}
	log := agent.NewLoggerWithWriter("api", io.Discard, "error")
	h := NewHandler(db, "nssm", t.TempDir(), "test", "secret-global", ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
	router := gin.New()
	registerAPIRoutes(router.Group("/api"), h)

	rec := authRequest(router, http.MethodGet, "/api/github/rate-limit", "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("response leaked token material: %s", rec.Body.String())
	}
	var entries []githubQuotaEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected global and one override bucket, got %+v", entries)
	}
	if entries[0].Source != "global" || len(entries[0].WatcherIDs) != 2 || entries[0].WatcherIDs[0] != 1 || entries[0].WatcherIDs[1] != 4 {
		t.Fatalf("unexpected global entry: %+v", entries[0])
	}
	if entries[1].Source != "watcher" || len(entries[1].WatcherIDs) != 2 || entries[1].WatcherIDs[0] != 2 || entries[1].WatcherIDs[1] != 3 {
		t.Fatalf("unexpected override entry: %+v", entries[1])
	}
}
