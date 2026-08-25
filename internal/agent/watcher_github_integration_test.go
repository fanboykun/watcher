package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestWatcherGitHubDeploymentStatusIntegration(t *testing.T) {
	tests := []struct {
		name         string
		startFailure bool
		wantStatuses []string
		wantErr      bool
	}{
		{name: "pending NSSM transition reports GitHub success", wantStatuses: []string{"in_progress", "success"}},
		{name: "genuine NSSM failure reports GitHub failure", startFailure: true, wantStatuses: []string{"in_progress", "failure"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installNSSMCommandMock(t)
			root := t.TempDir()
			artifactPath := filepath.Join(root, "artifact.zip")
			createTestZipFile(t, artifactPath, map[string]string{"api.exe": "target"})
			artifact, err := os.ReadFile(artifactPath)
			if err != nil {
				t.Fatal(err)
			}

			var statusMu sync.Mutex
			var statuses []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/artifact.zip":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(artifact)
				case r.Method == http.MethodPost && r.URL.Path == "/repos/example/app/deployments":
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{"id": 42})
				case r.Method == http.MethodPost && r.URL.Path == "/repos/example/app/deployments/42/statuses":
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					statusMu.Lock()
					statuses = append(statuses, body["state"])
					statusCount := len(statuses)
					statusMu.Unlock()
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{"id": statusCount})
				default:
					t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}); err != nil {
				t.Fatal(err)
			}
			dbWatcher := database.Watcher{Name: "alpha", ServiceName: "api", MetadataURL: "https://github.com/example/app", InstallDir: root}
			if err := db.Create(&dbWatcher).Error; err != nil {
				t.Fatal(err)
			}
			log := newTestLogger()
			wcfg := &WatcherConfig{
				Name: "alpha", ServiceName: "api", MetadataURL: dbWatcher.MetadataURL,
				InstallDir: root, DownloadRetries: 1, MaxKeptVersions: 3,
				Services: []ServiceConfig{{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"}},
			}
			state := NewStateManager(db, dbWatcher.ID, log, nil, nil)
			deployer := NewDeployer(wcfg, "nssm.exe", log, state.AppendDeployLog)
			steps := []nssmCommandStep{
				{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
				{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
				stateStep("api", ServiceStateStopped),
			}
			if tt.startFailure {
				steps = append(steps,
					nssmCommandStep{args: "start api", output: "cannot start", err: errors.New("exit status 1")},
					stateStep("api", ServiceStateStopped),
				)
			} else {
				steps = append(steps,
					nssmCommandStep{args: "start api", output: string(ServiceStateStartPending)},
					stateStep("api", ServiceStateStartPending),
					stateStep("api", ServiceStateRunning),
				)
			}
			runner := newScriptedNSSMRunner(t, steps...)
			deployer.serviceManager = newTestServiceManager(runner.run)
			runtime := &RepoWatcher{
				wcfg: wcfg,
				global: &config.AppConfig{
					Environment: "production", GitHubToken: "configured-token", GitHubDeployEnabled: true,
				},
				log: log, state: state, deployer: deployer, db: db, watcherID: dbWatcher.ID,
			}
			gh := NewGitHubClient("", log)
			gh.apiBase = server.URL
			gh.client = server.Client()
			svcMeta := ServiceMeta{Version: "v1", ArtifactURL: server.URL + "/artifact.zip"}

			err = runtime.deploy(context.Background(), gh, svcMeta, "v1", "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("deploy error=%v, wantErr=%v", err, tt.wantErr)
			}
			statusMu.Lock()
			gotStatuses := strings.Join(statuses, ",")
			statusMu.Unlock()
			if got := gotStatuses; got != strings.Join(tt.wantStatuses, ",") {
				t.Fatalf("GitHub statuses=%q, want %q", got, strings.Join(tt.wantStatuses, ","))
			}
			runner.assertDone()
		})
	}
}
