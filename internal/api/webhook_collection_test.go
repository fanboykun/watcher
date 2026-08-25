package api

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestWebhookCollectionAPIsReturnEmptyArrays(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	watcher := database.Watcher{
		Name:             "empty-webhook-watcher",
		ServiceName:      "empty-webhook-service",
		MetadataURL:      "https://example.com/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       t.TempDir(),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}

	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(
		db,
		"nssm",
		cfg.LogDir,
		"test",
		"",
		".env",
		cfg,
		agent.NewLoggerWithWriter("api", io.Discard, "error"),
		nil,
		make(chan uint, 1),
		make(chan struct{}, 1),
		nil,
		make(chan struct{}, 1),
	)

	t.Run("events", func(t *testing.T) {
		rec := authRequest(router, http.MethodGet, "/api/watchers/"+itoa(watcher.ID)+"/webhook-events", "", database.DefaultAuthPassword)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var response []database.WebhookEvent
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if response == nil {
			t.Fatalf("response decoded to nil; body=%s", rec.Body.String())
		}
	})

	t.Run("deliveries", func(t *testing.T) {
		rec := authRequest(router, http.MethodGet, "/api/watchers/"+itoa(watcher.ID)+"/webhook-deliveries", "", database.DefaultAuthPassword)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var response struct {
			Data []webhookDeliveryListItem `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
		}
		if response.Data == nil {
			t.Fatalf("response data decoded to nil; body=%s", rec.Body.String())
		}
	})
}
