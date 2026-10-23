package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigSnapshotIncludesEveryCapturedFile(t *testing.T) {
	root := t.TempDir()
	wcfg := &WatcherConfig{InstallDir: root, Services: []ServiceConfig{
		{WindowsServiceName: "api", EnvFile: "old/.env", EnvContent: "OLD=env", ConfigFiles: []ConfigFile{
			{FilePath: "config/app.json", Target: "app_dir", Content: "old app"},
			{FilePath: "web.config", Target: "release_dir", Content: "old release"},
		}},
		{WindowsServiceName: "web", ConfigFiles: []ConfigFile{{FilePath: "empty.txt", Target: "release_dir", Content: ""}}},
	}}
	if err := CaptureConfigSnapshot(wcfg, "release/v2", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	// Current service definitions are irrelevant to historical inspection.
	wcfg.Services = nil
	snapshot, err := ReadConfigSnapshot(root, "release/v2")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != "release/v2" || snapshot.Source != SnapshotSourceDeployment || snapshot.CapturedAt.IsZero() || len(snapshot.Services) != 2 {
		t.Fatalf("snapshot metadata/inventory: %+v", snapshot)
	}
	files := make(map[string]string)
	for _, service := range snapshot.Services {
		for _, file := range service.Files {
			files[service.ServiceName+"/"+file.Target+"/"+file.FilePath] = file.Content
		}
	}
	want := map[string]string{"api/env/old/.env": "OLD=env", "api/app_dir/config/app.json": "old app", "api/release_dir/web.config": "old release", "web/release_dir/empty.txt": ""}
	if len(files) != len(want) {
		t.Fatalf("captured files: %+v", files)
	}
	for key, content := range want {
		if got, ok := files[key]; !ok || got != content {
			t.Fatalf("%s=%q present=%v, want %q", key, got, ok, content)
		}
	}
}

func TestReadConfigSnapshotHandlesEmptyAndEnvOnlyHistory(t *testing.T) {
	for _, envOnly := range []bool{false, true} {
		root := t.TempDir()
		wcfg := &WatcherConfig{InstallDir: root}
		if envOnly {
			wcfg.Services = []ServiceConfig{{WindowsServiceName: "legacy", EnvFile: ".env", EnvContent: "OLD=true"}}
		}
		if err := CaptureConfigSnapshot(wcfg, "v1", SnapshotSourceDeployment); err != nil {
			t.Fatal(err)
		}
		snapshot, err := ReadConfigSnapshot(root, "v1")
		if err != nil || snapshot.Services == nil {
			t.Fatalf("empty/legacy snapshot: %+v %v", snapshot, err)
		}
		if !envOnly && len(snapshot.Services) != 0 {
			t.Fatal("empty snapshot invented service files")
		}
		if envOnly && (len(snapshot.Services) != 1 || len(snapshot.Services[0].Files) != 1 || snapshot.Services[0].Files[0].Target != "env") {
			t.Fatalf("env-only historical tree: %+v", snapshot)
		}
	}
}

func TestReadConfigSnapshotRejectsSymlinkCategories(t *testing.T) {
	root := t.TempDir()
	wcfg := &WatcherConfig{InstallDir: root, Services: []ServiceConfig{{WindowsServiceName: "api", EnvFile: ".env", EnvContent: "original"}}}
	if err := CaptureConfigSnapshot(wcfg, "v1", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.json"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ConfigSnapshotPath(root, "v1"), "services", "api", "app")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ReadConfigSnapshot(root, "v1"); err == nil {
		t.Fatal("snapshot inspection followed a symlink outside historical storage")
	}
}
