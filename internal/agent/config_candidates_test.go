package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func candidateFixture(t *testing.T) (*RepoWatcher, database.Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.Service{}, &database.ServiceConfigRevision{}, &database.ServiceConfigFile{}, &database.DeployLog{}, &database.PollEvent{}); err != nil {
		t.Fatal(err)
	}
	w := database.Watcher{Name: "test", InstallDir: t.TempDir(), AutoDeploy: true}
	if err := db.Create(&w).Error; err != nil {
		t.Fatal(err)
	}
	svc := database.Service{WatcherID: w.ID, ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe", EnvFile: ".env", EnvContent: "VALUE=active"}
	if err := db.Create(&svc).Error; err != nil {
		t.Fatal(err)
	}
	w.Services = []database.Service{svc}
	r := NewRepoWatcher(&w, db, &config.AppConfig{}, newTestLogger(), nil, nil)
	if err := os.WriteFile(filepath.Join(w.InstallDir, ".env"), []byte(svc.EnvContent), 0600); err != nil {
		t.Fatal(err)
	}
	return r, svc
}

func TestCandidateResolutionAndConsumption(t *testing.T) {
	r, svc := candidateFixture(t)
	for target, content := range map[string]string{"v2": "VALUE=exact", "next": "VALUE=next"} {
		if err := r.db.Create(&database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: target, EnvContent: content}).Error; err != nil {
			t.Fatal(err)
		}
	}
	selected, err := r.prepareCandidateServices("v2")
	if err != nil {
		t.Fatal(err)
	}
	if r.wcfg.Services[0].EnvContent != "VALUE=exact" || len(selected) != 1 {
		t.Fatal("exact revision did not take precedence")
	}
	var active database.Service
	if err := r.db.First(&active, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(r.wcfg.InstallDir, ".env"))
	if err != nil || active.EnvContent != svc.EnvContent || string(disk) != svc.EnvContent {
		t.Fatal("preparation modified active config")
	}
	// Concurrent edits must survive consumption of the configuration deployed.
	if err := r.db.Model(&database.ServiceConfigRevision{}).Where("id = ?", selected[0].ID).Update("env_content", "VALUE=new-edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := r.commitCandidateServices(selected); err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&active, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if active.EnvContent != "VALUE=exact" {
		t.Fatal("active persistence did not match deployed revision")
	}
	var count int64
	if err := r.db.Model(&database.ServiceConfigRevision{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("concurrent edit or unused next revision was consumed")
	}
	selected, err = r.prepareCandidateServices("v3")
	if err != nil || len(selected) != 1 || selected[0].TargetVersion != "next" {
		t.Fatalf("next fallback: %v %v", selected, err)
	}
	if err := r.commitCandidateServices(selected); err != nil {
		t.Fatal(err)
	}
	if err := r.db.Model(&database.ServiceConfigRevision{}).Where("target_version = ?", "next").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("consumed next revision remains: %d %v", count, err)
	}
}

func TestReleaseApprovalIsVersionBoundAndSurvivesFailure(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"auto_deploy": false, "approved_version": "v2", "pending_version": "v2", "status": "failed"}).Error; err != nil {
		t.Fatal(err)
	}
	allowed, err := r.releaseApproved("v2")
	if err != nil || !allowed {
		t.Fatalf("approved retry blocked: %v %v", allowed, err)
	}
	allowed, err = r.releaseApproved("v3")
	if err != nil || allowed {
		t.Fatalf("unapproved newer release allowed: %v %v", allowed, err)
	}
	var w database.Watcher
	if err := r.db.First(&w, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if w.PendingVersion != "v3" || w.ApprovedVersion != "" || w.Status != "pending_approval" {
		t.Fatalf("wrong pending state: %+v", w)
	}
}

func TestCandidateDeploymentFailurePreservesDraftAndLiveEnvironment(t *testing.T) {
	for _, failure := range []string{"missing_binary", "stop", "start", "success"} {
		t.Run(failure, func(t *testing.T) {
			r, svc := candidateFixture(t)
			installNSSMCommandMock(t)
			revision := database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: "next", EnvContent: "VALUE=candidate"}
			if err := r.db.Create(&revision).Error; err != nil {
				t.Fatal(err)
			}
			selected, err := r.prepareCandidateServices("v2")
			if err != nil {
				t.Fatal(err)
			}
			zipPath := filepath.Join(t.TempDir(), "artifact.zip")
			files := map[string]string{"api.exe": "target"}
			if failure == "missing_binary" {
				files = map[string]string{"other.exe": "bad"}
			}
			createTestZipFile(t, zipPath, files)
			manager := &fakeServiceManager{states: map[string]ServiceState{"api": ServiceStateRunning}}
			if failure == "stop" {
				manager.stopErrors = map[string]error{"api": errors.New("stop failed")}
			}
			if failure == "start" {
				manager.startErrors = map[string]error{"api": errors.New("start failed")}
			}
			r.deployer.serviceManager = manager
			err = r.deployer.Deploy(context.Background(), "v2", zipPath, "")
			if failure == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if err := r.commitCandidateServices(selected); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("expected deployment failure")
			}
			var active database.Service
			if err := r.db.First(&active, svc.ID).Error; err != nil {
				t.Fatal(err)
			}
			disk, err := os.ReadFile(filepath.Join(r.wcfg.InstallDir, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			expected := svc.EnvContent
			if failure == "success" {
				expected = revision.EnvContent
			}
			if string(disk) != expected || active.EnvContent != expected {
				t.Fatalf("disk=%q DB=%q, want %q", disk, active.EnvContent, expected)
			}
			var count int64
			if err := r.db.Model(&database.ServiceConfigRevision{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if (failure == "success" && count != 0) || (failure != "success" && count != 1) {
				t.Fatalf("revision count=%d", count)
			}
			if failure == "success" {
				content, err := ReadServiceSnapshotEnv(r.wcfg, "v2", 0)
				if err != nil || content != revision.EnvContent {
					t.Fatalf("snapshot=%q %v", content, err)
				}
			}
		})
	}
}

func TestFailedRedeployPreservesHistoricalSnapshot(t *testing.T) {
	installNSSMCommandMock(t)
	d, root, zipPath := newNSSMDeployFixture(t, "new")
	d.wcfg.Services[0].EnvFile = ".env"
	d.wcfg.Services[0].EnvContent = "VALUE=old"
	if err := CaptureConfigSnapshot(d.wcfg, "v1", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("VALUE=old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "releases", "v1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "releases", "v1", "api.exe"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	d.wcfg.Services[0].EnvContent = "VALUE=new"
	d.serviceManager = &fakeServiceManager{states: map[string]ServiceState{"api": ServiceStateRunning}, startErrors: map[string]error{"api": errors.New("first start failed")}, startFails: map[string]int{"api": 1}}
	if err := d.Deploy(context.Background(), "v1", zipPath, "v1"); err == nil {
		t.Fatal("expected failed redeploy")
	}
	content, err := ReadServiceSnapshotEnv(d.wcfg, "v1", 0)
	if err != nil || content != "VALUE=old" {
		t.Fatalf("old snapshot overwritten: %q %v", content, err)
	}
}

func TestCandidateManagedFileResolutionAndConcurrentEdit(t *testing.T) {
	r, svc := candidateFixture(t)
	r.wcfg.Services[0].ConfigFiles = []ConfigFile{{FilePath: "active.json", Target: "app_dir", Content: "active"}}
	files := []database.RevisionConfigFile{{FilePath: "next.json", Target: "release_dir", Content: "next"}}
	for _, rev := range []database.ServiceConfigRevision{
		{ServiceID: svc.ID, TargetVersion: "next", EnvContent: "next", ConfigFiles: &files},
		{ServiceID: svc.ID, TargetVersion: "v2", EnvContent: "exact"},
	} {
		if err := r.db.Create(&rev).Error; err != nil {
			t.Fatal(err)
		}
	}
	selected, err := r.prepareCandidateServices("v2")
	if err != nil || len(selected) != 1 || r.wcfg.Services[0].ConfigFiles[0].FilePath != "active.json" {
		t.Fatalf("exact env-only must inherit active files: %+v %v", r.wcfg.Services, err)
	}
	selected, err = r.prepareCandidateServices("v3")
	if err != nil || len(selected) != 1 || r.wcfg.Services[0].ConfigFiles[0].FilePath != "next.json" {
		t.Fatalf("next file fallback: %+v %v", r.wcfg.Services, err)
	}
	var current database.ServiceConfigRevision
	if err := r.db.First(&current, selected[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	edited := []database.RevisionConfigFile{{FilePath: "edited.json", Target: "app_dir", Content: "edit"}}
	current.ConfigFiles = &edited
	if err := r.db.Save(&current).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.commitCandidateServices(selected); err != nil {
		t.Fatal(err)
	}
	var active database.Service
	if err := r.db.Preload("ConfigFiles").First(&active, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(active.ConfigFiles) != 1 || active.ConfigFiles[0].FilePath != "next.json" {
		t.Fatalf("committed concurrent edit instead of selected files: %+v", active.ConfigFiles)
	}
	if err := r.db.First(&current, selected[0].ID).Error; err != nil || (*current.ConfigFiles)[0].FilePath != "edited.json" {
		t.Fatalf("concurrent config-only edit lost: %+v %v", current, err)
	}
}

func TestCandidateManagedFilesDeployRecoveryAndRollback(t *testing.T) {
	for _, scenario := range []string{"success", "remove_all", "health", "start", "same_version_start", "preflight"} {
		t.Run(scenario, func(t *testing.T) {
			r, svc := candidateFixture(t)
			installNSSMCommandMock(t)
			oldFiles := []ConfigFile{{FilePath: "config/old.json", Target: "app_dir", Content: "old-app"}, {FilePath: "old.release.json", Target: "release_dir", Content: "old-release"}}
			r.wcfg.Services[0].ConfigFiles = oldFiles
			for _, file := range oldFiles {
				if err := r.db.Create(&database.ServiceConfigFile{ServiceID: svc.ID, FilePath: file.FilePath, Target: file.Target, Content: file.Content}).Error; err != nil {
					t.Fatal(err)
				}
			}
			root := r.wcfg.InstallDir
			oldRelease := filepath.Join(root, "releases", "v1")
			if err := os.MkdirAll(oldRelease, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(oldRelease, "api.exe"), []byte("old-binary"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(oldRelease, filepath.Join(root, "current")); err != nil {
				t.Fatal(err)
			}
			for _, file := range oldFiles {
				target := root
				if file.Target == "release_dir" {
					target = oldRelease
				}
				if err := writeManagedFile(target, file.FilePath, file.Content); err != nil {
					t.Fatal(err)
				}
			}
			if err := CaptureConfigSnapshot(r.wcfg, "v1", SnapshotSourceDeployment); err != nil {
				t.Fatal(err)
			}
			nextFiles := []database.RevisionConfigFile{{FilePath: "config/new.json", Target: "app_dir", Content: "new-app"}, {FilePath: "new.release.json", Target: "release_dir", Content: "new-release"}}
			if scenario == "remove_all" {
				nextFiles = []database.RevisionConfigFile{}
			}
			version := "v2"
			if scenario == "same_version_start" {
				version = "v1"
			}
			revision := database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: version, EnvContent: "VALUE=new", ConfigFiles: &nextFiles}
			if err := r.db.Create(&revision).Error; err != nil {
				t.Fatal(err)
			}
			selected, err := r.prepareCandidateServices(version)
			if err != nil {
				t.Fatal(err)
			}
			assertContent := func(path, want string) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("%s=%q want %q: %v", path, got, want, err)
				}
			}
			assertAbsent := func(path string) {
				t.Helper()
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected %s absent: %v", path, err)
				}
			}
			assertContent(filepath.Join(root, "config", "old.json"), "old-app")
			assertAbsent(filepath.Join(root, "config", "new.json"))
			zipPath := filepath.Join(t.TempDir(), "target.zip")
			artifacts := map[string]string{"api.exe": "new-binary"}
			if scenario == "preflight" {
				artifacts = map[string]string{"wrong.exe": "bad"}
			}
			createTestZipFile(t, zipPath, artifacts)
			manager := &fakeServiceManager{states: map[string]ServiceState{"api": ServiceStateRunning}}
			if scenario == "start" || scenario == "same_version_start" {
				manager.startErrors = map[string]error{"api": errors.New("start failed")}
				manager.startFails = map[string]int{"api": 1}
			}
			if scenario == "health" {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if calls.Add(1) == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
				}))
				t.Cleanup(server.Close)
				r.wcfg.HealthCheck = HealthCheckConfig{Enabled: true, URL: server.URL, Retries: 1, TimeoutSec: 1}
			}
			r.deployer.serviceManager = manager
			err = r.deployer.Deploy(context.Background(), version, zipPath, "v1")
			success := scenario == "success" || scenario == "remove_all"
			if success {
				if err != nil {
					t.Fatal(err)
				}
				if err := r.commitCandidateServices(selected); err != nil {
					t.Fatal(err)
				}
				assertAbsent(filepath.Join(root, "config", "old.json"))
				if scenario == "success" {
					assertContent(filepath.Join(root, "config", "new.json"), "new-app")
					assertContent(filepath.Join(root, "current", "new.release.json"), "new-release")
				}
				var active database.Service
				if err := r.db.Preload("ConfigFiles").First(&active, svc.ID).Error; err != nil {
					t.Fatal(err)
				}
				if len(active.ConfigFiles) != len(nextFiles) {
					t.Fatalf("deployed shape not persisted: %+v", active.ConfigFiles)
				}
				snapshots, err := readConfigSnapshotServices(r.wcfg, version)
				if err != nil || len(snapshots[0].ConfigFiles) != len(nextFiles) {
					t.Fatalf("deployed snapshot inventory: %+v %v", snapshots, err)
				}
				if err := r.deployer.Rollback(context.Background(), "v1"); err != nil {
					t.Fatal(err)
				}
				restored, err := ReconcileConfigSnapshot(r.db, r.watcherID, "v1")
				if err != nil || len(restored[0].ConfigFiles) != 2 {
					t.Fatalf("restore old config inventory: %+v %v", restored, err)
				}
			} else if err == nil {
				t.Fatal("expected deployment failure")
			}
			assertContent(filepath.Join(root, "config", "old.json"), "old-app")
			assertAbsent(filepath.Join(root, "config", "new.json"))
			assertContent(filepath.Join(root, "current", "old.release.json"), "old-release")
			assertContent(filepath.Join(root, ".env"), svc.EnvContent)
			var active database.Service
			if err := r.db.Preload("ConfigFiles").First(&active, svc.ID).Error; err != nil {
				t.Fatal(err)
			}
			if active.EnvContent != svc.EnvContent || len(active.ConfigFiles) != 2 {
				t.Fatalf("active old configuration was not preserved/restored: %+v", active)
			}
			historical, err := readConfigSnapshotServices(r.wcfg, "v1")
			if err != nil || len(historical[0].ConfigFiles) != 2 || historical[0].ConfigFiles[0].FilePath != "config/old.json" {
				t.Fatalf("old snapshot lost: %+v %v", historical, err)
			}
			var count int64
			if err := r.db.Model(&database.ServiceConfigRevision{}).Where("service_id = ?", svc.ID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if success && count != 0 || !success && count != 1 {
				t.Fatalf("draft count=%d success=%v", count, success)
			}
		})
	}
}

func TestManagedConfigCaseOnlyRenameIsNotRetired(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "settings.json")
	if err := os.WriteFile(oldPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDeployer(&WatcherConfig{InstallDir: root, Services: []ServiceConfig{{ConfigFiles: []ConfigFile{{FilePath: "Settings.json", Target: "app_dir", Content: "new"}}}}}, "", newTestLogger(), nil)
	d.previousServices = []ServiceConfig{{ConfigFiles: []ConfigFile{{FilePath: "settings.json", Target: "app_dir", Content: "old"}}}}
	files, err := d.prepareDeploymentEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].remove || files[0].path != filepath.Join(root, "Settings.json") {
		t.Fatalf("case-only rename scheduled a removal: %+v", files)
	}
	if err := applyDeploymentEnv(files); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "Settings.json"))
	if err != nil || string(content) != "new" {
		t.Fatalf("case-only target write=%q %v", content, err)
	}
}
