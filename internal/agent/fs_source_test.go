package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeFSFeedRoot(t *testing.T) {
	tests := []struct {
		raw, want, wantErr string
	}{
		{raw: `\\fileserver\releases\app`, want: `\\fileserver\releases\app`},
		{raw: ` \\fileserver\releases\app\version.json `, want: `\\fileserver\releases\app`},
		{raw: `D:\releases\app\`, want: `D:\releases\app`},
		{raw: `d:/releases/app/version.json`, want: `d:/releases/app`},
		{raw: `/srv/releases/app/`, want: `/srv/releases/app`},
		{raw: `D:\`, wantErr: "below the drive or share root"},
		{raw: `\\fileserver`, wantErr: "must be absolute"},
		{raw: `releases\app`, wantErr: "must be absolute"},
		{raw: `https://releases.internal/app`, wantErr: "not a URL"},
	}
	for _, tt := range tests {
		if runtime.GOOS == "windows" && strings.HasPrefix(tt.raw, "/") {
			continue // rooted POSIX paths are not absolute on Windows
		}
		got, err := NormalizeFSFeedRoot(tt.raw)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NormalizeFSFeedRoot(%q) error=%v, want %q", tt.raw, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeFSFeedRoot(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
		}
	}
}

func writeFeedManifest(t *testing.T, root string, services map[string]ServiceMeta) {
	t.Helper()
	data, err := json.Marshal(VersionMetadata{Services: services})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, feedManifestName), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFSFeedSource_FetchMetadataResolvesArtifactPaths(t *testing.T) {
	root := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "api.zip")
	writeFeedManifest(t, root, map[string]ServiceMeta{
		"relative": {Version: "v2", ArtifactURL: "releases/v2/api.zip"},
		"slashed":  {Version: "services/prs/v0.1.0", ArtifactURL: "releases/services/prs/v0.1.0/prs.zip"},
		"absolute": {Version: "v2", ArtifactURL: elsewhere},
	})

	meta, err := newFSFeedSource(root+string(filepath.Separator), newTestLogger()).FetchMetadata(context.Background(), "latest", "relative")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"relative": filepath.Join(root, "releases", "v2", "api.zip"),
		"slashed":  filepath.Join(root, "releases", "services", "prs", "v0.1.0", "prs.zip"),
		"absolute": elsewhere,
	}
	for name, path := range want {
		if got := meta.Services[name].ArtifactURL; got != path {
			t.Errorf("%s artifact = %q, want %q", name, got, path)
		}
	}
}

func TestFSFeedSource_FetchMetadataErrors(t *testing.T) {
	withURL := t.TempDir()
	writeFeedManifest(t, withURL, map[string]ServiceMeta{"api": {Version: "v1", ArtifactURL: "https://cdn.internal/api.zip"}})
	unreadable := t.TempDir()
	writeFeedManifest(t, unreadable, map[string]ServiceMeta{"api": {Version: "v1"}})
	if err := os.Chmod(filepath.Join(unreadable, feedManifestName), 0); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, root, ref, wantErr string
	}{
		{name: "ref escaping releases dir", root: withURL, ref: "v1/../../etc", wantErr: "must be a tag"},
		{name: "missing manifest", root: t.TempDir(), wantErr: "not found; check the feed path"},
		{name: "artifact URL", root: withURL, wantErr: "reference artifacts by path, not URL"},
		{name: "access denied", root: unreadable, wantErr: "access denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "access denied" && os.Geteuid() == 0 {
				t.Skip("root ignores file permissions")
			}
			_, err := newFSFeedSource(tt.root, newTestLogger()).FetchMetadata(context.Background(), tt.ref, "api")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestFSFeedSource_DownloadArtifact(t *testing.T) {
	original := sleepContext
	var sleeps int
	sleepContext = func(context.Context, time.Duration) error { sleeps++; return nil }
	t.Cleanup(func() { sleepContext = original })

	src := filepath.Join(t.TempDir(), "api.zip")
	if err := os.WriteFile(src, []byte("zip-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out.zip")
	source := newFSFeedSource(filepath.Dir(src), newTestLogger())
	if err := source.DownloadArtifact(context.Background(), src, dest, 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "zip-bytes" {
		t.Fatalf("copied %q", got)
	}

	err := source.DownloadArtifact(context.Background(), src+".missing", dest+"2", 2)
	if err == nil || !strings.Contains(err.Error(), "copy failed after 2 attempts") || sleeps != 1 {
		t.Fatalf("missing artifact error=%v sleeps=%d", err, sleeps)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := source.DownloadArtifact(ctx, src, dest+"3", 3); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled copy error = %v", err)
	}
	if _, err := os.Stat(dest + "3.tmp"); !os.IsNotExist(err) {
		t.Fatalf("canceled copy left a temp file: %v", err)
	}
}

func TestRepoWatcherRun_FSFeedDeploys(t *testing.T) {
	installNSSMCommandMock(t)
	feed := t.TempDir()
	releaseDir := filepath.Join(feed, "releases", "v2")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(releaseDir, "api.zip")
	createTestZipFile(t, artifactPath, map[string]string{"api.exe": "target"})
	sum, err := fileSHA256(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFeedManifest(t, feed, map[string]ServiceMeta{"api": {Version: "v2", ArtifactURL: "releases/v2/api.zip", SHA256: sum}})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}, &database.PollEvent{}, &database.ServiceConfigRevision{}); err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	dbWatcher := database.Watcher{
		Name: "alpha", ServiceName: "api", SourceType: SourceFS, MetadataURL: filepath.Join(feed, feedManifestName),
		InstallDir: installDir, DownloadRetries: 1, MaxKeptVersions: 3, AutoDeploy: true,
	}
	if err := db.Create(&dbWatcher).Error; err != nil {
		t.Fatal(err)
	}
	rw := NewRepoWatcher(&dbWatcher, db, &config.AppConfig{GitHubToken: "configured-token", GitHubDeployEnabled: true}, newTestLogger(), nil, nil)
	rw.wcfg.Services = []ServiceConfig{{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"}}
	rw.deployer = NewDeployer(rw.wcfg, "nssm.exe", rw.log, rw.state.AppendDeployLog)
	runner := newScriptedNSSMRunner(t,
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{args: "start api", output: string(ServiceStateStartPending)},
		stateStep("api", ServiceStateStartPending),
		stateStep("api", ServiceStateRunning),
	)
	rw.deployer.serviceManager = newTestServiceManager(runner.run)

	if err := rw.Run(context.Background()); err != nil {
		t.Fatalf("fs feed poll failed: %v", err)
	}
	runner.assertDone()
	var got database.Watcher
	if err := db.First(&got, dbWatcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	var attempt database.DeployLog
	if err := db.Where("watcher_id = ?", dbWatcher.ID).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if got.CurrentVersion != "v2" || attempt.Status != "succeeded" || !strings.Contains(attempt.Logs, "github_deployment: skipped for fs release source") || !strings.Contains(attempt.Logs, "sha256 verified") {
		t.Fatalf("version=%q status=%q logs=%q", got.CurrentVersion, attempt.Status, attempt.Logs)
	}
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("deploy must not consume the feed's artifact: %v", err)
	}
}
