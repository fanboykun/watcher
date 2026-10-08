package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
