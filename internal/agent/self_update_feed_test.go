package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelfUpdateSourceDetection(t *testing.T) {
	tests := []struct {
		url      string
		wantFeed string // "" means GitHub
		wantErr  bool
	}{
		{url: "https://github.com/fanboykun/watcher"},
		{url: "https://releases.internal/watcher", wantFeed: "*agent.httpFeedSource"},
		{url: `\\fileserver\releases\watcher`, wantFeed: "*agent.fsFeedSource"},
		{url: "watcher", wantErr: true},
	}
	for _, tt := range tests {
		feed, err := SelfUpdateSource{RepoURL: tt.url}.feed(newTestLogger())
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v", tt.url, err)
			continue
		}
		got := ""
		if feed != nil {
			got = fmt.Sprintf("%T", feed)
		}
		if got != tt.wantFeed {
			t.Errorf("%s: source = %q, want %q", tt.url, got, tt.wantFeed)
		}
	}
}

func TestSelfUpdateFromHTTPFeed(t *testing.T) {
	payload := []byte("watcher-release-zip")
	sum, err := writeTempAndHash(t, payload)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "ops" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/watcher/version.json":
			fmt.Fprintf(w, `{"services":{"watcher":{"version":"v9.1.0","artifact_url":"releases/v9.1.0/watcher.zip","sha256":%q}}}`, sum)
		case "/watcher/releases/v9.1.0/watcher.zip":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	src := SelfUpdateSource{RepoURL: server.URL + "/watcher", Token: "github-token", FeedAuth: FeedAuth{Type: FeedAuthBasic, Username: "ops", Secret: "pw"}}

	info, err := CheckForUpdate(context.Background(), "v9.0.0", src)
	if err != nil {
		t.Fatal(err)
	}
	if !info.UpdateAvailable || info.LatestVersion != "v9.1.0" || info.DownloadURL != server.URL+"/watcher/releases/v9.1.0/watcher.zip" || info.SHA256 != sum {
		t.Fatalf("info = %+v", info)
	}
	dest := filepath.Join(t.TempDir(), "update.zip")
	if err := downloadSelfUpdate(context.Background(), info, src, dest, newTestLogger()); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != string(payload) {
		t.Fatalf("downloaded %q", got)
	}

	tampered := *info
	tampered.SHA256 = strings.Repeat("0", 64)
	if err := downloadSelfUpdate(context.Background(), &tampered, src, dest, newTestLogger()); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("tampered download error = %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("rejected update left on disk: %v", err)
	}
}

func TestSelfUpdateFromFSFeedRequiresWatcherEntry(t *testing.T) {
	root := t.TempDir()
	writeFeedManifest(t, root, map[string]ServiceMeta{"api": {Version: "v1", ArtifactURL: "releases/v1/api.zip"}})
	_, err := CheckForUpdate(context.Background(), "v1.0.0", SelfUpdateSource{RepoURL: root})
	if err == nil || !strings.Contains(err.Error(), `no "watcher" service entry`) {
		t.Fatalf("error = %v", err)
	}

	writeFeedManifest(t, root, map[string]ServiceMeta{"watcher": {Version: "v1.2.0", ArtifactURL: "releases/v1.2.0/watcher.zip"}})
	info, err := CheckForUpdate(context.Background(), "v1.0.0", SelfUpdateSource{RepoURL: root})
	if err != nil || !info.UpdateAvailable || info.DownloadURL != filepath.Join(root, "releases", "v1.2.0", "watcher.zip") {
		t.Fatalf("info = %+v err = %v", info, err)
	}
}

func writeTempAndHash(t *testing.T, data []byte) (string, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return fileSHA256(path)
}
