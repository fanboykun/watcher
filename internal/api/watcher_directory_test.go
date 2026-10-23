package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func TestWatcherDirectoryEditRejectsRelocationWithoutChangingPool(t *testing.T) {
	for _, phase := range []string{"empty", "downloading", "downloaded", "pending_approval", "deploying", "rollback", "paused"} {
		t.Run(phase, func(t *testing.T) {
			router, db, watcher, service := candidateAPIFixture(t)
			watcher.PendingVersion = ""
			watcher.Status = "unknown"
			watcher.Paused = phase == "paused"
			if phase == "empty" {
				if err := db.Delete(&service).Error; err != nil {
					t.Fatal(err)
				}
			}
			if phase == "pending_approval" {
				watcher.PendingVersion = "v2"
				watcher.Status = "pending_approval"
			}
			if phase == "deploying" || phase == "rollback" {
				watcher.Status = "deploying"
				attempt := database.DeployLog{WatcherID: watcher.ID, Kind: "deploy", Status: "in_progress", Version: "v2"}
				if phase == "rollback" {
					attempt.Kind = "rollback"
				}
				if err := db.Create(&attempt).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Save(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			var artifact database.CatalogArtifact
			if phase == "downloading" || phase == "downloaded" {
				artifact = database.CatalogArtifact{WatcherID: watcher.ID, ServiceName: watcher.ServiceName, InstallDir: watcher.InstallDir, Version: "v2"}
				if phase == "downloaded" {
					now := time.Now().UTC()
					artifact.DownloadedAt = &now
				}
				if err := db.Create(&artifact).Error; err != nil {
					t.Fatal(err)
				}
				watcher.PendingCatalogID = artifact.ID
				if err := db.Model(&watcher).UpdateColumn("pending_catalog_id", artifact.ID).Error; err != nil {
					t.Fatal(err)
				}
			}

			oldFile := filepath.Join(watcher.InstallDir, ".watcher", "snapshots", "v1", "manifest.json")
			if artifact.ID != 0 {
				oldFile = agent.CatalogArtifactPath(&artifact)
			}
			if phase != "empty" {
				if err := os.MkdirAll(filepath.Dir(oldFile), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(oldFile, []byte("original pool data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			newDir := filepath.Join(t.TempDir(), "relocated")
			payload, err := json.Marshal(map[string]any{"install_dir": newDir, "name": "must not be saved", "paused": true})
			if err != nil {
				t.Fatal(err)
			}
			rec := authRequest(router, http.MethodPut, fmt.Sprintf("/api/watchers/%d", watcher.ID), string(payload), "watcher")
			if rec.Code != http.StatusConflict {
				t.Fatalf("directory edit during %s: %d %s", phase, rec.Code, rec.Body.String())
			}
			var stored database.Watcher
			if err := db.First(&stored, watcher.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.InstallDir != watcher.InstallDir || stored.Name != watcher.Name || stored.Paused != watcher.Paused || stored.Status != watcher.Status || stored.PendingCatalogID != watcher.PendingCatalogID || !stored.UpdatedAt.Equal(watcher.UpdatedAt) {
				t.Fatalf("rejected relocation changed pool state: %+v", stored)
			}
			if artifact.ID != 0 {
				var storedArtifact database.CatalogArtifact
				if err := db.First(&storedArtifact, artifact.ID).Error; err != nil || storedArtifact.InstallDir != artifact.InstallDir {
					t.Fatalf("cached release changed: %+v %v", storedArtifact, err)
				}
			}
			if phase != "empty" {
				content, err := os.ReadFile(oldFile)
				if err != nil || string(content) != "original pool data" {
					t.Fatalf("old pool data changed: %q %v", content, err)
				}
			}
			if _, err := os.Stat(newDir); !os.IsNotExist(err) {
				t.Fatalf("rejected edit touched destination: %v", err)
			}
		})
	}
}

func TestWatcherSettingsAcceptOriginalOrOmittedDirectory(t *testing.T) {
	router, db, watcher, _ := candidateAPIFixture(t)
	for _, includeDirectory := range []bool{false, true} {
		payload := map[string]any{"name": "saved normally"}
		if includeDirectory {
			payload["install_dir"] = watcher.InstallDir
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		rec := authRequest(router, http.MethodPut, fmt.Sprintf("/api/watchers/%d", watcher.ID), string(body), "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("save settings: %d %s", rec.Code, rec.Body.String())
		}
		var stored database.Watcher
		if err := db.First(&stored, watcher.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.InstallDir != watcher.InstallDir || stored.Name != "saved normally" {
			t.Fatalf("normal settings save failed: %+v", stored)
		}
	}
}
