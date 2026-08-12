package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================
// CleanOldReleases
// ============================================================

func TestCleanOldReleases_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create 5 version directories with staggered mod times
	versions := []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0"}
	for i, v := range versions {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
		os.WriteFile(filepath.Join(vDir, "app.exe"), []byte("binary"), 0644)
		// Set mod times with increasing timestamps
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(vDir, modTime, modTime)
	}

	// Keep only 3
	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only 3 remain
	entries, _ := os.ReadDir(relDir)
	if len(entries) != 3 {
		t.Errorf("expected 3 directories, got %d", len(entries))
	}

	// The 3 newest should survive
	for _, v := range []string{"v1.2.0", "v1.3.0", "v1.4.0"} {
		if _, err := os.Stat(filepath.Join(relDir, v)); os.IsNotExist(err) {
			t.Errorf("expected %s to survive cleanup", v)
		}
	}

	// The 2 oldest should be gone
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		if _, err := os.Stat(filepath.Join(relDir, v)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", v)
		}
	}
}

func TestCleanOldReleases_FewerThanKeep(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Only 2 versions, keep=3 → nothing should be deleted
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
	}

	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(relDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 directories, got %d", len(entries))
	}
}

func TestCleanOldReleases_EmptyReleasesDir(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanOldReleases_NoReleasesDir(t *testing.T) {
	dir := t.TempDir()

	// No releases/ directory at all — should not error
	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================
// ListAvailableVersions
// ============================================================

func TestListAvailableVersions_SortedByModTime(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create versions with specific mod times (oldest to newest)
	versions := []string{"v1.0.0", "v1.1.0", "v1.2.0"}
	for i, v := range versions {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
		os.WriteFile(filepath.Join(vDir, "app.exe"), []byte("binary content"), 0644)
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(vDir, modTime, modTime)
	}

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(result))
	}

	// Should be sorted newest first
	if result[0].Version != "v1.2.0" {
		t.Errorf("first version = %q, want v1.2.0", result[0].Version)
	}
	if result[2].Version != "v1.0.0" {
		t.Errorf("last version = %q, want v1.0.0", result[2].Version)
	}

	// Verify size is computed
	for _, v := range result {
		if v.SizeBytes <= 0 {
			t.Errorf("version %s has no size", v.Version)
		}
	}
}

func TestListAvailableVersions_IgnoresFiles(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create a version directory and a stray file
	os.MkdirAll(filepath.Join(relDir, "v1.0.0"), 0755)
	os.WriteFile(filepath.Join(relDir, "leftover.zip"), []byte("zip"), 0644)

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 version (ignoring files), got %d", len(result))
	}
}

func TestListAvailableVersions_NoReleasesDir(t *testing.T) {
	dir := t.TempDir()

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for missing releases dir, got %v", result)
	}
}

func TestReleaseStorageName_RoundTrip(t *testing.T) {
	version := "alpha-api/v0.1.0"
	storage := releaseStorageName(version)
	if storage == version {
		t.Fatalf("expected storage name to escape path separators, got %q", storage)
	}
	if restored := restoreReleaseVersion(storage); restored != version {
		t.Fatalf("restored version = %q, want %q", restored, version)
	}
}

func TestListAvailableVersions_DecodesStoredVersionNames(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatalf("mkdir releases: %v", err)
	}

	rawVersion := "alpha-api/v0.1.0"
	storedDir := filepath.Join(relDir, releaseStorageName(rawVersion))
	if err := os.MkdirAll(storedDir, 0755); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storedDir, "app.exe"), []byte("binary"), 0644); err != nil {
		t.Fatalf("write app file: %v", err)
	}

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result))
	}
	if result[0].Version != rawVersion {
		t.Fatalf("listed version = %q, want %q", result[0].Version, rawVersion)
	}
	if result[0].Path != storedDir {
		t.Fatalf("listed path = %q, want %q", result[0].Path, storedDir)
	}
}

func TestListAvailableVersions_HasSnapshotFlag(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// v1.0.0 with snapshot
	v1Dir := filepath.Join(relDir, "v1.0.0")
	os.MkdirAll(filepath.Join(v1Dir, snapshotDir), 0755)

	// v1.1.0 without snapshot
	v2Dir := filepath.Join(relDir, "v1.1.0")
	os.MkdirAll(v2Dir, 0755)

	// v1.2.0 with .watcher-snapshot as a file (not a dir)
	v3Dir := filepath.Join(relDir, "v1.2.0")
	os.MkdirAll(v3Dir, 0755)
	os.WriteFile(filepath.Join(v3Dir, snapshotDir), []byte("not a dir"), 0600)

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(result))
	}

	// Build a map for easy lookup
	m := make(map[string]bool)
	for _, v := range result {
		m[v.Version] = v.HasSnapshot
	}

	if !m["v1.0.0"] {
		t.Error("v1.0.0 should have HasSnapshot=true")
	}
	if m["v1.1.0"] {
		t.Error("v1.1.0 should have HasSnapshot=false")
	}
	if m["v1.2.0"] {
		t.Error("v1.2.0 should have HasSnapshot=false (file, not dir)")
	}
}

func TestCurrentVersionFromCurrentDir_ReadsReleaseSymlink(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "releases", releaseStorageName("v1.0.0"))
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatalf("mkdir release dir: %v", err)
	}

	currentDir := filepath.Join(dir, "current")
	if err := os.Symlink(releaseDir, currentDir); err != nil {
		t.Fatalf("symlink current dir: %v", err)
	}

	got, err := currentVersionFromCurrentDir(dir)
	if err != nil {
		t.Fatalf("currentVersionFromCurrentDir returned error: %v", err)
	}
	if got != "v1.0.0" {
		t.Fatalf("currentVersionFromCurrentDir = %q, want %q", got, "v1.0.0")
	}
}

func TestResolveRollbackVersionFallsBackToCurrentDirWhenDBVersionMissing(t *testing.T) {
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "releases", releaseStorageName("v1.0.0"))
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatalf("mkdir release dir: %v", err)
	}

	currentDir := filepath.Join(dir, "current")
	if err := os.Symlink(releaseDir, currentDir); err != nil {
		t.Fatalf("symlink current dir: %v", err)
	}

	d := NewDeployer(&WatcherConfig{InstallDir: dir}, "nssm.exe", newTestLogger(), func(string) {})
	got := d.resolveRollbackVersion("v1.1.0", "")
	if got != "v1.0.0" {
		t.Fatalf("resolveRollbackVersion = %q, want %q", got, "v1.0.0")
	}
}

func TestResolveRollbackVersionFallsBackToAvailableReleaseList(t *testing.T) {
	dir := t.TempDir()
	releasesDir := filepath.Join(dir, "releases")
	if err := os.MkdirAll(releasesDir, 0755); err != nil {
		t.Fatalf("mkdir releases dir: %v", err)
	}

	olderDir := filepath.Join(releasesDir, releaseStorageName("v1.0.0"))
	if err := os.MkdirAll(olderDir, 0755); err != nil {
		t.Fatalf("mkdir older release dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(olderDir, "app.exe"), []byte("older"), 0644); err != nil {
		t.Fatalf("write older app: %v", err)
	}
	olderTime := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(olderDir, olderTime, olderTime); err != nil {
		t.Fatalf("chtimes older release: %v", err)
	}

	newerDir := filepath.Join(releasesDir, releaseStorageName("v1.1.0"))
	if err := os.MkdirAll(newerDir, 0755); err != nil {
		t.Fatalf("mkdir newer release dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(newerDir, "app.exe"), []byte("newer"), 0644); err != nil {
		t.Fatalf("write newer app: %v", err)
	}
	newerTime := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(newerDir, newerTime, newerTime); err != nil {
		t.Fatalf("chtimes newer release: %v", err)
	}

	d := NewDeployer(&WatcherConfig{InstallDir: dir}, "nssm.exe", newTestLogger(), func(string) {})
	got := d.resolveRollbackVersion("v1.2.0", "")
	if got != "v1.1.0" {
		t.Fatalf("resolveRollbackVersion = %q, want %q", got, "v1.1.0")
	}
}

func TestIISBindingFromPublicURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "http default port", raw: "http://example.com", want: "http/*:80:example.com"},
		{name: "http custom port", raw: "http://example.com:8080", want: "http/*:8080:example.com"},
		{name: "https default port", raw: "https://example.com", want: "https/*:443:example.com"},
		{name: "empty host allowed", raw: "http://:8080", want: "http/*:8080:"},
		{name: "missing scheme", raw: "example.com", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := iisBindingFromPublicURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got binding %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("binding = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureServiceByType_IISCreatesRegistration(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var calls []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		call := name
		if len(args) > 0 {
			call += " " + strings.Join(args, " ")
		}
		calls = append(calls, call)
		cmd := strings.Join(args, " ")
		switch cmd {
		case "list apppool my-pool":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		case "add apppool /name:my-pool":
			return []byte("APPPOOL object \"my-pool\" added"), nil
		case "set apppool my-pool /managedRuntimeVersion:v4.0":
			return []byte("APPPOOL object changed"), nil
		case "list site my-site":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		case "add site /name:my-site /bindings:http/*:8080:example.com /physicalPath:C:/apps/current":
			return []byte("SITE object \"my-site\" added"), nil
		case "set vdir my-site/ /physicalPath:C:/apps/current":
			return []byte("VDIR object changed"), nil
		case "set app my-site/ /applicationPool:my-pool":
			return []byte("APP object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		WindowsServiceName: "frontend",
		IISAppKind:         "aspnet_classic",
		IISSiteName:        "my-site",
		IISAppPool:         "my-pool",
		IISManagedRuntime:  "v4.0",
		PublicURL:          "http://example.com:8080",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}

	want := []string{
		appcmdPath() + " list apppool my-pool",
		appcmdPath() + " add apppool /name:my-pool",
		appcmdPath() + " set apppool my-pool /managedRuntimeVersion:v4.0",
		appcmdPath() + " list site my-site",
		appcmdPath() + " add site /name:my-site /bindings:http/*:8080:example.com /physicalPath:C:/apps/current",
		appcmdPath() + " set vdir my-site/ /physicalPath:C:/apps/current",
		appcmdPath() + " set app my-site/ /applicationPool:my-pool",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestEnsureServiceByType_IISStaticDefaultsAppPoolToNoManagedCode(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var sawRuntime bool
	runCommand = func(name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "list apppool my-pool":
			return []byte("APPPOOL \"my-pool\""), nil
		case "set apppool my-pool /managedRuntimeVersion:":
			sawRuntime = true
			return []byte("APPPOOL object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", strings.Join(args, " "))
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:       "iis",
		IISAppKind:        "static",
		IISAppPool:        "my-pool",
		IISManagedRuntime: "",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}
	if !sawRuntime {
		t.Fatal("expected app pool runtime to be configured")
	}
}

func TestEnsureServiceByType_IISRequiresPublicURLWhenCreatingSite(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	runCommand = func(name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "list site my-site":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		default:
			return []byte(""), nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		IISAppKind:         "php",
		WindowsServiceName: "frontend",
		IISSiteName:        "my-site",
	}, "C:/apps/current")
	if err == nil {
		t.Fatal("expected error when public URL is missing for a new IIS site")
	}
}

func TestEnsureServiceByType_IISUpdatesExistingSiteWithoutPublicURL(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var calls []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		call := name
		if len(args) > 0 {
			call += " " + strings.Join(args, " ")
		}
		calls = append(calls, call)
		switch strings.Join(args, " ") {
		case "list apppool frontend":
			return []byte("APPPOOL \"frontend\""), nil
		case "set apppool frontend /managedRuntimeVersion:":
			return []byte("APPPOOL object changed"), nil
		case "list site my-site":
			return []byte("SITE \"my-site\""), nil
		case "set vdir my-site/ /physicalPath:C:/apps/current":
			return []byte("VDIR object changed"), nil
		case "set app my-site/ /applicationPool:frontend":
			return []byte("APP object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", strings.Join(args, " "))
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		IISAppKind:         "static",
		WindowsServiceName: "frontend",
		IISSiteName:        "my-site",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}

	want := []string{
		appcmdPath() + " list apppool frontend",
		appcmdPath() + " set apppool frontend /managedRuntimeVersion:",
		appcmdPath() + " list site my-site",
		appcmdPath() + " set vdir my-site/ /physicalPath:C:/apps/current",
		appcmdPath() + " set app my-site/ /applicationPool:frontend",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestEnsureIISServiceDefaultsAppPoolAndSiteFromServiceName(t *testing.T) {
	oldRunCommand := runCommand
	defer func() { runCommand = oldRunCommand }()

	var calls []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if len(args) >= 3 && args[0] == "list" && (args[1] == "apppool" || args[1] == "site") {
			return []byte("object was not found"), errors.New("missing")
		}
		return []byte("ok"), nil
	}

	d := NewDeployer(&WatcherConfig{InstallDir: t.TempDir()}, "nssm.exe", newTestLogger(), func(string) {})
	svc := ServiceConfig{
		ServiceType:        "iis",
		WindowsServiceName: "admin-fe",
		IISAppKind:         "static",
		PublicURL:          "http://admin.example.test",
	}

	if err := d.ensureIISService(svc, filepath.Join(t.TempDir(), "current")); err != nil {
		t.Fatalf("ensureIISService returned error: %v", err)
	}

	joined := strings.Join(calls, "\n")
	for _, want := range []string{
		"add apppool /name:admin-fe",
		"set apppool admin-fe /managedRuntimeVersion:",
		"add site /name:admin-fe /bindings:http/*:80:admin.example.test",
		"set app admin-fe/ /applicationPool:admin-fe",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected command containing %q, got calls:\n%s", want, joined)
		}
	}
}

func TestWriteReleaseConfigFilesWritesOnlyReleaseDirTargets(t *testing.T) {
	currentDir := t.TempDir()
	d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
		{
			WindowsServiceName: "admin-fe",
			ConfigFiles: []ConfigFile{
				{FilePath: "web.config", Target: "release_dir", Content: "<configuration />"},
				{FilePath: "settings/app.json", Target: "app_dir", Content: "{}"},
			},
		},
	}}, "nssm.exe", newTestLogger(), func(string) {})

	if err := d.writeReleaseConfigFiles(currentDir); err != nil {
		t.Fatalf("writeReleaseConfigFiles returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(currentDir, "web.config"))
	if err != nil {
		t.Fatalf("expected web.config in current dir: %v", err)
	}
	if string(got) != "<configuration />" {
		t.Fatalf("web.config content = %q", string(got))
	}
	if _, err := os.Stat(filepath.Join(currentDir, "settings", "app.json")); !os.IsNotExist(err) {
		t.Fatalf("app_dir config should not be written into current dir, stat err=%v", err)
	}
}

func TestCaptureConfigSnapshot(t *testing.T) {
	t.Run("single service with all config categories", func(t *testing.T) {
		releaseDir := t.TempDir()
		d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api-svc",
				EnvFile:            ".env",
				EnvContent:         "PORT=3000\nDB=prod",
				ConfigFiles: []ConfigFile{
					{FilePath: "config/app.json", Target: "app_dir", Content: `{"key":"val"}`},
					{FilePath: "web.config", Target: "release_dir", Content: "<configuration />"},
				},
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})

		if err := d.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatalf("captureConfigSnapshot returned error: %v", err)
		}

		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "env", "api-svc", ".env"), "PORT=3000\nDB=prod")
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "app", "api-svc", "config", "app.json"), `{"key":"val"}`)
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "release", "api-svc", "web.config"), "<configuration />")
	})

	t.Run("multiple services with per-service namespacing", func(t *testing.T) {
		releaseDir := t.TempDir()
		d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "svc-a",
				EnvFile:            ".env",
				EnvContent:         "SVC=a",
				ConfigFiles: []ConfigFile{
					{FilePath: "settings.json", Target: "app_dir", Content: `{"svc":"a"}`},
				},
			},
			{
				WindowsServiceName: "svc-b",
				EnvFile:            ".env",
				EnvContent:         "SVC=b",
				ConfigFiles: []ConfigFile{
					{FilePath: "settings.json", Target: "app_dir", Content: `{"svc":"b"}`},
				},
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})

		if err := d.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatalf("captureConfigSnapshot returned error: %v", err)
		}

		// Both services have their own namespaced files even with same relative path
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "env", "svc-a", ".env"), "SVC=a")
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "env", "svc-b", ".env"), "SVC=b")
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "app", "svc-a", "settings.json"), `{"svc":"a"}`)
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "app", "svc-b", "settings.json"), `{"svc":"b"}`)
	})

	t.Run("service with no env content skips env snapshot", func(t *testing.T) {
		releaseDir := t.TempDir()
		d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "web-fe",
				EnvFile:            ".env",
				EnvContent:         "", // empty
				ConfigFiles: []ConfigFile{
					{FilePath: "web.config", Target: "release_dir", Content: "<cfg/>"},
				},
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})

		if err := d.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatalf("captureConfigSnapshot returned error: %v", err)
		}

		// env dir should not exist for this service
		envDir := filepath.Join(releaseDir, snapshotDir, "env", "web-fe")
		if _, err := os.Stat(envDir); !os.IsNotExist(err) {
			t.Fatalf("expected no env dir for service with empty EnvContent, stat err=%v", err)
		}
		// release config should still exist
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "release", "web-fe", "web.config"), "<cfg/>")
	})

	t.Run("service with no config files produces no snapshot dirs", func(t *testing.T) {
		releaseDir := t.TempDir()
		d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "bare-svc",
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})

		if err := d.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatalf("captureConfigSnapshot returned error: %v", err)
		}

		snapRoot := filepath.Join(releaseDir, snapshotDir)
		// snapshot root should not exist since nothing was written
		if _, err := os.Stat(snapRoot); !os.IsNotExist(err) {
			t.Fatalf("expected no snapshot dir for service with no config, stat err=%v", err)
		}
	})

	t.Run("redeploy clears old snapshot", func(t *testing.T) {
		releaseDir := t.TempDir()

		// First deploy with one config
		d1 := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api",
				EnvFile:            ".env",
				EnvContent:         "V=1",
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})
		if err := d1.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "env", "api", ".env"), "V=1")

		// Second deploy with different config — should overwrite
		d2 := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api",
				EnvFile:            ".env",
				EnvContent:         "V=2",
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})
		if err := d2.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "env", "api", ".env"), "V=2")
	})

	t.Run("config files with empty content are skipped", func(t *testing.T) {
		releaseDir := t.TempDir()
		d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "svc",
				ConfigFiles: []ConfigFile{
					{FilePath: "empty.txt", Target: "app_dir", Content: ""},
					{FilePath: "ok.txt", Target: "app_dir", Content: "data"},
				},
			},
		}}, "nssm.exe", newTestLogger(), func(string) {})

		if err := d.captureConfigSnapshot(releaseDir); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Stat(filepath.Join(releaseDir, snapshotDir, "app", "svc", "empty.txt")); !os.IsNotExist(err) {
			t.Fatal("empty content config file should not be snapshotted")
		}
		assertFileContent(t, filepath.Join(releaseDir, snapshotDir, "app", "svc", "ok.txt"), "data")
	})
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, string(got), want)
	}
}

func TestBackfillConfigSnapshots(t *testing.T) {
	t.Run("backfills missing snapshots", func(t *testing.T) {
		installDir := t.TempDir()
		releasesDir := filepath.Join(installDir, "releases")
		os.MkdirAll(filepath.Join(releasesDir, "v1.0.0"), 0755)
		os.MkdirAll(filepath.Join(releasesDir, "v1.1.0"), 0755)

		wcfg := &WatcherConfig{
			Name:       "test-watcher",
			InstallDir: installDir,
			Services: []ServiceConfig{
				{
					WindowsServiceName: "api",
					EnvFile:            ".env",
					EnvContent:         "PORT=8080",
				},
			},
		}

		BackfillConfigSnapshots(wcfg, newTestLogger())

		// Both versions should have snapshots
		assertFileContent(t, filepath.Join(releasesDir, "v1.0.0", snapshotDir, "env", "api", ".env"), "PORT=8080")
		assertFileContent(t, filepath.Join(releasesDir, "v1.1.0", snapshotDir, "env", "api", ".env"), "PORT=8080")
	})

	t.Run("skips versions that already have snapshots", func(t *testing.T) {
		installDir := t.TempDir()
		releasesDir := filepath.Join(installDir, "releases")
		os.MkdirAll(filepath.Join(releasesDir, "v1.0.0"), 0755)
		os.MkdirAll(filepath.Join(releasesDir, "v1.1.0"), 0755)

		// Pre-create a snapshot for v1.0.0 with different content
		snapDir := filepath.Join(releasesDir, "v1.0.0", snapshotDir, "env", "api")
		os.MkdirAll(snapDir, 0755)
		os.WriteFile(filepath.Join(snapDir, ".env"), []byte("ORIGINAL=true"), 0600)

		wcfg := &WatcherConfig{
			Name:       "test-watcher",
			InstallDir: installDir,
			Services: []ServiceConfig{
				{
					WindowsServiceName: "api",
					EnvFile:            ".env",
					EnvContent:         "NEW=true",
				},
			},
		}

		BackfillConfigSnapshots(wcfg, newTestLogger())

		// v1.0.0 should keep its original snapshot
		assertFileContent(t, filepath.Join(releasesDir, "v1.0.0", snapshotDir, "env", "api", ".env"), "ORIGINAL=true")
		// v1.1.0 should get a new snapshot
		assertFileContent(t, filepath.Join(releasesDir, "v1.1.0", snapshotDir, "env", "api", ".env"), "NEW=true")
	})

	t.Run("handles missing releases dir gracefully", func(t *testing.T) {
		wcfg := &WatcherConfig{
			Name:       "test-watcher",
			InstallDir: t.TempDir(), // no releases/ subdir
			Services: []ServiceConfig{
				{
					WindowsServiceName: "api",
					EnvFile:            ".env",
					EnvContent:         "PORT=8080",
				},
			},
		}

		// Should not panic or error — just silently skip
		BackfillConfigSnapshots(wcfg, newTestLogger())
	})

	t.Run("idempotent backfill does not overwrite", func(t *testing.T) {
		installDir := t.TempDir()
		releasesDir := filepath.Join(installDir, "releases")
		os.MkdirAll(filepath.Join(releasesDir, "v1.0.0"), 0755)

		wcfg := &WatcherConfig{
			Name:       "test-watcher",
			InstallDir: installDir,
			Services: []ServiceConfig{
				{
					WindowsServiceName: "svc",
					EnvFile:            ".env",
					EnvContent:         "A=1",
				},
			},
		}

		BackfillConfigSnapshots(wcfg, newTestLogger())
		assertFileContent(t, filepath.Join(releasesDir, "v1.0.0", snapshotDir, "env", "svc", ".env"), "A=1")

		// Change config, run backfill again — should NOT overwrite
		wcfg.Services[0].EnvContent = "A=2"
		BackfillConfigSnapshots(wcfg, newTestLogger())
		assertFileContent(t, filepath.Join(releasesDir, "v1.0.0", snapshotDir, "env", "svc", ".env"), "A=1")
	})
}

func TestHasConfigSnapshot(t *testing.T) {
	t.Run("returns true when snapshot dir exists", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, snapshotDir), 0755)
		if !HasConfigSnapshot(dir) {
			t.Fatal("expected HasConfigSnapshot to return true")
		}
	})

	t.Run("returns false when snapshot dir is missing", func(t *testing.T) {
		dir := t.TempDir()
		if HasConfigSnapshot(dir) {
			t.Fatal("expected HasConfigSnapshot to return false")
		}
	})

	t.Run("returns false when snapshot is a file not a dir", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, snapshotDir), []byte("not a dir"), 0600)
		if HasConfigSnapshot(dir) {
			t.Fatal("expected HasConfigSnapshot to return false for file")
		}
	})
}

func TestRestoreConfigSnapshot(t *testing.T) {
	t.Run("restores all three categories to correct paths", func(t *testing.T) {
		releaseDir := t.TempDir()
		installDir := t.TempDir()
		currentDir := t.TempDir()

		// Create a snapshot manually
		wcfg := &WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api",
				EnvFile:            ".env",
				EnvContent:         "PORT=3000",
				ConfigFiles: []ConfigFile{
					{FilePath: "config/db.json", Target: "app_dir", Content: `{"host":"db1"}`},
					{FilePath: "web.config", Target: "release_dir", Content: "<cfg/>"},
				},
			},
		}}
		if err := CaptureConfigSnapshot(wcfg, releaseDir); err != nil {
			t.Fatal(err)
		}

		// Restore
		if err := RestoreConfigSnapshot(releaseDir, installDir, currentDir); err != nil {
			t.Fatalf("RestoreConfigSnapshot returned error: %v", err)
		}

		assertFileContent(t, filepath.Join(installDir, ".env"), "PORT=3000")
		assertFileContent(t, filepath.Join(installDir, "config", "db.json"), `{"host":"db1"}`)
		assertFileContent(t, filepath.Join(currentDir, "web.config"), "<cfg/>")
	})

	t.Run("multiple services restore without collision", func(t *testing.T) {
		releaseDir := t.TempDir()
		installDir := t.TempDir()
		currentDir := t.TempDir()

		wcfg := &WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "svc-a",
				EnvFile:            "a.env",
				EnvContent:         "SVC=a",
			},
			{
				WindowsServiceName: "svc-b",
				EnvFile:            "b.env",
				EnvContent:         "SVC=b",
			},
		}}
		if err := CaptureConfigSnapshot(wcfg, releaseDir); err != nil {
			t.Fatal(err)
		}

		if err := RestoreConfigSnapshot(releaseDir, installDir, currentDir); err != nil {
			t.Fatalf("RestoreConfigSnapshot returned error: %v", err)
		}

		assertFileContent(t, filepath.Join(installDir, "a.env"), "SVC=a")
		assertFileContent(t, filepath.Join(installDir, "b.env"), "SVC=b")
	})

	t.Run("no snapshot dir degrades gracefully", func(t *testing.T) {
		releaseDir := t.TempDir()
		installDir := t.TempDir()
		currentDir := t.TempDir()

		// No snapshot exists — should return nil (not error)
		err := RestoreConfigSnapshot(releaseDir, installDir, currentDir)
		if err != nil {
			t.Fatalf("expected nil error for missing snapshot, got: %v", err)
		}
	})

	t.Run("restores overwrite drifted config", func(t *testing.T) {
		releaseDir := t.TempDir()
		installDir := t.TempDir()
		currentDir := t.TempDir()

		// Capture a snapshot
		wcfg := &WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api",
				EnvFile:            ".env",
				EnvContent:         "PORT=3000",
			},
		}}
		if err := CaptureConfigSnapshot(wcfg, releaseDir); err != nil {
			t.Fatal(err)
		}

		// Simulate config drift — someone changed the env file
		os.MkdirAll(installDir, 0755)
		os.WriteFile(filepath.Join(installDir, ".env"), []byte("PORT=9999"), 0600)

		// Restore should bring back the snapshot content
		if err := RestoreConfigSnapshot(releaseDir, installDir, currentDir); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, filepath.Join(installDir, ".env"), "PORT=3000")
	})

	t.Run("nested config file paths are preserved", func(t *testing.T) {
		releaseDir := t.TempDir()
		installDir := t.TempDir()
		currentDir := t.TempDir()

		wcfg := &WatcherConfig{Services: []ServiceConfig{
			{
				WindowsServiceName: "api",
				ConfigFiles: []ConfigFile{
					{FilePath: "deep/nested/config.yaml", Target: "app_dir", Content: "key: value"},
				},
			},
		}}
		if err := CaptureConfigSnapshot(wcfg, releaseDir); err != nil {
			t.Fatal(err)
		}

		if err := RestoreConfigSnapshot(releaseDir, installDir, currentDir); err != nil {
			t.Fatal(err)
		}
		assertFileContent(t, filepath.Join(installDir, "deep", "nested", "config.yaml"), "key: value")
	})
}
