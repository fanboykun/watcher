package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestLogger returns a logger that discards output during tests
func newTestLogger() *Logger {
	return NewLogger("test")
}

// newTestClient returns a GitHubClient pointing at the test server for both
// direct HTTP calls and GitHub API calls.
func newTestClient(token string, server *httptest.Server) *GitHubClient {
	c := NewGitHubClient(token, newTestLogger())
	c.client = server.Client()
	c.apiBase = server.URL // redirect API calls to test server
	return c
}

func serviceMetaKeys(m map[string]ServiceMeta) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ============================================================
// ParseGitHubURL
// ============================================================

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		wantOwner       string
		wantRepo        string
		wantErrContains string
	}{
		{
			name:      "valid metadata URL",
			url:       "https://github.com/my-org/my-repo/releases/latest/download/version.json",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
		},
		{
			name:      "valid short URL",
			url:       "https://github.com/fanboykun/simple/releases/latest/download/version.json",
			wantOwner: "fanboykun",
			wantRepo:  "simple",
		},
		{
			name:      "missing https scheme",
			url:       "github.com/my-org/my-repo/releases/latest/download/version.json",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
		},
		{
			name:      "http scheme is accepted",
			url:       "http://github.com/fanboykun/watcher",
			wantOwner: "fanboykun",
			wantRepo:  "watcher",
		},
		{
			name:      "repo dot git suffix is normalized",
			url:       "https://github.com/fanboykun/watcher.git",
			wantOwner: "fanboykun",
			wantRepo:  "watcher",
		},
		{
			name:      "ssh URL is accepted",
			url:       "git@github.com:fanboykun/watcher.git",
			wantOwner: "fanboykun",
			wantRepo:  "watcher",
		},
		{
			name:            "only owner no repo",
			url:             "https://github.com/my-org",
			wantErrContains: "cannot extract owner/repo",
		},
		{
			name:            "empty string",
			url:             "",
			wantErrContains: "empty GitHub URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubURL(tt.url)
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

// ============================================================
// parseArtifactURL
// ============================================================

func TestParseArtifactURL(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		wantOwner       string
		wantRepo        string
		wantAsset       string
		wantErrContains string
	}{
		{
			name:      "valid artifact URL",
			url:       "https://github.com/my-org/my-repo/releases/download/v1.2.3/my-repo-v1.2.3.zip",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
			wantAsset: "my-repo-v1.2.3.zip",
		},
		{
			name:      "valid artifact URL fanboykun",
			url:       "https://github.com/fanboykun/simple/releases/download/v0.1.0/simple-v0.1.0.zip",
			wantOwner: "fanboykun",
			wantRepo:  "simple",
			wantAsset: "simple-v0.1.0.zip",
		},
		{
			name:            "missing filename",
			url:             "https://github.com/my-org/my-repo/releases/download/v1.2.3",
			wantErrContains: "cannot extract asset name",
		},
		{
			name:            "wrong scheme",
			url:             "http://github.com/my-org/my-repo/releases/download/v1.2.3/file.zip",
			wantErrContains: "unexpected URL format",
		},
		{
			name:            "empty string",
			url:             "",
			wantErrContains: "unexpected URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, asset, err := parseArtifactURL(tt.url)
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if asset != tt.wantAsset {
				t.Errorf("asset = %q, want %q", asset, tt.wantAsset)
			}
		})
	}
}

func TestParseReleaseDownloadURL(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		wantOwner       string
		wantRepo        string
		wantTag         string
		wantAsset       string
		wantErrContains string
	}{
		{
			name:      "valid release download URL",
			url:       "https://github.com/my-org/my-repo/releases/download/v1.2.3/my-repo-v1.2.3.zip",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
			wantTag:   "v1.2.3",
			wantAsset: "my-repo-v1.2.3.zip",
		},
		{
			name:      "release tag with slash",
			url:       "https://github.com/my-org/my-repo/releases/download/alpha-api/v1.2.3/alpha-api-v1.2.3.zip",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
			wantTag:   "alpha-api/v1.2.3",
			wantAsset: "alpha-api-v1.2.3.zip",
		},
		{
			name:      "release tag with encoded slash",
			url:       "https://github.com/my-org/my-repo/releases/download/alpha-api%2Fv1.2.3/alpha-api-v1.2.3.zip",
			wantOwner: "my-org",
			wantRepo:  "my-repo",
			wantTag:   "alpha-api/v1.2.3",
			wantAsset: "alpha-api-v1.2.3.zip",
		},
		{
			name:            "missing tag segment",
			url:             "https://github.com/my-org/my-repo/releases/download//my-repo-v1.2.3.zip",
			wantErrContains: "unexpected release download URL format",
		},
		{
			name:            "not a release download URL",
			url:             "https://github.com/my-org/my-repo/archive/refs/heads/main.zip",
			wantErrContains: "unexpected release download URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, tag, asset, err := parseReleaseDownloadURL(tt.url)
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErrContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tt.wantOwner {
				t.Errorf("owner = %q, want %q", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
			if asset != tt.wantAsset {
				t.Errorf("asset = %q, want %q", asset, tt.wantAsset)
			}
		})
	}
}

// ============================================================
// FetchMetadata -- public repo (no token, direct fetch)
// ============================================================

func TestFetchMetadata_PublicRepo(t *testing.T) {
	meta := VersionMetadata{
		Services: map[string]ServiceMeta{
			"simple": {
				Version:     "v1.0.0",
				Artifact:    "simple-v1.0.0.zip",
				ArtifactURL: "https://github.com/fanboykun/simple/releases/download/v1.0.0/simple-v1.0.0.zip",
				PublishedAt: "2024-01-01T00:00:00Z",
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should have no Authorization header for public repo
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected Authorization header for public repo")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meta)
	}))
	defer server.Close()

	client := newTestClient("", server) // no token = public repo
	result, err := client.fetchDirect(context.Background(), server.URL+"/version.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got VersionMetadata
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	svc, ok := got.Services["simple"]
	if !ok {
		t.Fatal("expected service 'simple' in metadata")
	}
	if svc.Version != "v1.0.0" {
		t.Errorf("version = %q, want %q", svc.Version, "v1.0.0")
	}
}

func TestFetchMetadata_PublicRepo_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient("", server)
	_, err := client.fetchDirect(context.Background(), server.URL+"/version.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestFetchMetadata_PublicRepo_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient("", server)
	_, err := client.fetchDirect(context.Background(), server.URL+"/version.json")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("expected hint about GITHUB_TOKEN in error, got: %v", err)
	}
}

func TestFetchMetadata_PublicRepo_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "not valid json {{{")
	}))
	defer server.Close()

	client := NewGitHubClient("", newTestLogger())
	client.client = server.Client()
	_, err := client.FetchMetadata(context.Background(), server.URL+"/version.json")
	if err == nil {
		t.Fatal("expected JSON decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode version.json") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

// ============================================================
// FetchMetadata -- private repo (token, API flow)
// ============================================================

func TestFetchMetadata_PrivateRepo_Success(t *testing.T) {
	const token = "ghp_testtoken"

	versionJSON := `{
		"services": {
			"my-service": {
				"version": "v2.0.0",
				"artifact": "my-service-v2.0.0.zip",
				"artifact_url": "https://github.com/my-org/my-service/releases/download/v2.0.0/my-service-v2.0.0.zip",
				"published_at": "2024-06-01T00:00:00Z"
			}
		}
	}`

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		// Authorization header must always be present
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			t.Errorf("call %d: expected Authorization 'Bearer %s', got %q", callCount, token, auth)
		}

		switch callCount {
		case 1:
			// Step 1: GET /repos/my-org/my-service/releases/latest
			if !strings.Contains(r.URL.Path, "releases/latest") {
				t.Errorf("call 1: expected releases/latest path, got %s", r.URL.Path)
			}
			release := githubRelease{
				Assets: []githubAsset{
					{ID: 101, Name: "my-service-v2.0.0.zip", URL: "http://" + r.Host + "/asset/101"},
					{ID: 102, Name: "version.json", URL: "http://" + r.Host + "/asset/102"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(release)

		case 2:
			// Step 2: GET /asset/102 — download version.json content
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("call 2: expected Accept: application/octet-stream, got %q", r.Header.Get("Accept"))
			}
			fmt.Fprint(w, versionJSON)

		default:
			t.Errorf("unexpected call %d to %s", callCount, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := newTestClient(token, server)
	result, err := client.FetchMetadata(
		context.Background(),
		"https://github.com/my-org/my-service/releases/latest/download/version.json",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected exactly 2 API calls, got %d", callCount)
	}

	svc, ok := result.Services["my-service"]
	if !ok {
		t.Fatal("expected service 'my-service' in metadata")
	}
	if svc.Version != "v2.0.0" {
		t.Errorf("version = %q, want %q", svc.Version, "v2.0.0")
	}
}

func TestFetchMetadata_PrivateRepo_NoReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	_, err := client.FetchMetadata(
		context.Background(),
		"https://github.com/my-org/my-repo/releases/latest/download/version.json",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestFetchMetadata_PrivateRepo_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient("ghp_bad_token", server)
	_, err := client.FetchMetadata(
		context.Background(),
		"https://github.com/my-org/my-repo/releases/latest/download/version.json",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "repo scope") {
		t.Errorf("expected repo scope hint in error, got: %v", err)
	}
}

func TestFetchMetadata_PrivateRepo_AssetNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a release with no version.json asset
		release := githubRelease{
			Assets: []githubAsset{
				{ID: 101, Name: "app-v1.0.0.zip", URL: "http://" + r.Host + "/asset/101"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	_, err := client.FetchMetadata(
		context.Background(),
		"https://github.com/my-org/my-repo/releases/latest/download/version.json",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "version.json") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected asset not found error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "app-v1.0.0.zip") {
		t.Errorf("expected available assets listed in error, got: %v", err)
	}
}

func TestFetchServiceMetadataForRelease_RepoLatestFindsMatchingServiceRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/fanboykun/multi-release/releases/latest":
			release := githubRelease{
				TagName:     "alpha-api/v0.2.0",
				PublishedAt: "2026-04-02T10:00:00Z",
				Assets: []githubAsset{
					{
						Name:               "alpha-api-v0.2.0-windows-amd64.zip",
						BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/alpha-api%2Fv0.2.0/alpha-api-v0.2.0-windows-amd64.zip",
					},
					{
						Name:               "alpha-api.version.json",
						BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/alpha-api%2Fv0.2.0/alpha-api.version.json",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		case "/repos/fanboykun/multi-release/releases":
			releases := []githubRelease{
				{
					TagName:     "alpha-api/v0.2.0",
					PublishedAt: "2026-04-02T10:00:00Z",
					Assets: []githubAsset{
						{
							Name:               "alpha-api-v0.2.0-windows-amd64.zip",
							BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/alpha-api%2Fv0.2.0/alpha-api-v0.2.0-windows-amd64.zip",
						},
						{
							Name:               "alpha-api.version.json",
							BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/alpha-api%2Fv0.2.0/alpha-api.version.json",
						},
					},
				},
				{
					TagName:     "beta-api/v0.1.0",
					PublishedAt: "2026-04-01T12:00:00Z",
					Assets: []githubAsset{
						{
							Name:               "beta-api-v0.1.0-windows-amd64.zip",
							BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/beta-api%2Fv0.1.0/beta-api-v0.1.0-windows-amd64.zip",
						},
						{
							Name:               "beta-api.version.json",
							BrowserDownloadURL: "https://github.com/fanboykun/multi-release/releases/download/beta-api%2Fv0.1.0/beta-api.version.json",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(releases)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient("", server)
	meta, err := client.FetchServiceMetadataForRelease(
		context.Background(),
		"https://github.com/fanboykun/multi-release",
		"latest",
		"beta-api",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(meta.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(meta.Services))
	}
	svc, ok := meta.Services["beta-api"]
	if !ok {
		t.Fatalf("expected beta-api metadata, got keys: %v", serviceMetaKeys(meta.Services))
	}
	if svc.Version != "beta-api/v0.1.0" {
		t.Fatalf("version = %q, want %q", svc.Version, "beta-api/v0.1.0")
	}
	if strings.Contains(svc.Artifact, "version.json") {
		t.Fatalf("expected deployable artifact, got metadata asset %q", svc.Artifact)
	}
	if _, exists := meta.Services["beta-api.version.json"]; exists {
		t.Fatalf("metadata assets should not appear as services")
	}
}

// ============================================================
// DownloadArtifact -- public repo
// ============================================================

func TestDownloadArtifact_PublicRepo_Success(t *testing.T) {
	zipContent := makeTestZip(t, map[string]string{
		"web.exe":    "fake web binary",
		"worker.exe": "fake worker binary",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(zipContent)
	}))
	defer server.Close()

	client := newTestClient("", server)

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "app.zip")

	err := client.downloadDirect(context.Background(), server.URL+"/download/app.zip", destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Fatal("expected zip file to exist at dest path")
	}

	// Verify no .tmp file left behind
	if _, err := os.Stat(destPath + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file should have been cleaned up")
	}
}

func TestDownloadArtifact_PublicRepo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient("", server)
	destDir := t.TempDir()

	err := client.DownloadArtifact(
		context.Background(),
		server.URL+"/download/app.zip",
		filepath.Join(destDir, "app.zip"),
		2, // 2 retries to keep test fast
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "2 attempts") {
		t.Errorf("expected retry count in error, got: %v", err)
	}
}

func TestDownloadArtifact_PublicRepo_RetrySucceeds(t *testing.T) {
	callCount := 0
	zipContent := makeTestZip(t, map[string]string{"web.exe": "binary"})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			// First attempt fails
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Second attempt succeeds
		w.WriteHeader(http.StatusOK)
		w.Write(zipContent)
	}))
	defer server.Close()

	client := newTestClient("", server)
	destDir := t.TempDir()

	err := client.DownloadArtifact(
		context.Background(),
		server.URL+"/download/app.zip",
		filepath.Join(destDir, "app.zip"),
		3,
	)
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", callCount)
	}
}

func TestDownloadArtifact_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestClient("", server)
	destDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := client.DownloadArtifact(
		ctx,
		server.URL+"/download/app.zip",
		filepath.Join(destDir, "app.zip"),
		5,
	)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ============================================================
// Authorization header behaviour
// ============================================================

func TestNewRequest_WithToken(t *testing.T) {
	client := NewGitHubClient("ghp_mytoken", newTestLogger())
	req, err := client.newRequest(context.Background(), http.MethodGet, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := req.Header.Get("Authorization")
	want := "Bearer ghp_mytoken"
	if got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestNewRequest_WithoutToken(t *testing.T) {
	client := NewGitHubClient("", newTestLogger())
	req, err := client.newRequest(context.Background(), http.MethodGet, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("expected no Authorization header without token, got %q", got)
	}
}

// ============================================================
// Helpers
// ============================================================

// makeTestZip creates an in-memory zip with the given filename->content map
func makeTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// ============================================================
// CreateDeployment
// ============================================================

func TestCreateDeployment_Success(t *testing.T) {
	const token = "ghp_testtoken"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and path
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repos/my-org/my-repo/deployments") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Verify headers
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("missing or wrong Authorization header")
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing Accept header")
		}
		if r.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Errorf("missing X-GitHub-Api-Version header")
		}

		// Verify body
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["ref"] != "v1.0.0" {
			t.Errorf("expected ref=v1.0.0, got %v", body["ref"])
		}
		if body["environment"] != "production" {
			t.Errorf("expected environment=production, got %v", body["environment"])
		}
		if body["auto_merge"] != false {
			t.Errorf("expected auto_merge=false")
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 12345})
	}))
	defer server.Close()

	client := newTestClient(token, server)
	id, err := client.CreateDeployment(context.Background(), "my-org", "my-repo", "v1.0.0", "production", "Deploying v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 12345 {
		t.Errorf("expected deployment ID 12345, got %d", id)
	}
}

func TestCreateDeployment_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message": "Bad credentials"}`)
	}))
	defer server.Close()

	client := newTestClient("bad_token", server)
	_, err := client.CreateDeployment(context.Background(), "my-org", "my-repo", "v1.0.0", "production", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestCreateDeployment_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	_, err := client.CreateDeployment(context.Background(), "org", "repo", "v1.0.0", "prod", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 in error, got: %v", err)
	}
}

// ============================================================
// UpdateDeploymentStatus
// ============================================================

func TestUpdateDeploymentStatus_Success(t *testing.T) {
	const token = "ghp_testtoken"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		expectedPath := "/repos/my-org/my-repo/deployments/12345/statuses"
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q, want %q", r.URL.Path, expectedPath)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["state"] != "success" {
			t.Errorf("expected state=success, got %v", body["state"])
		}
		if body["log_url"] != "http://example.com/deploys/1" {
			t.Errorf("expected log_url, got %v", body["log_url"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 99})
	}))
	defer server.Close()

	client := newTestClient(token, server)
	err := client.UpdateDeploymentStatus(context.Background(), "my-org", "my-repo", 12345, "success", "http://example.com/deploys/1", "Deployed successfully")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateDeploymentStatus_NoLogURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if _, ok := body["log_url"]; ok {
			t.Error("log_url should not be present when empty")
		}
		if body["state"] != "in_progress" {
			t.Errorf("expected state=in_progress, got %v", body["state"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 100})
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	err := client.UpdateDeploymentStatus(context.Background(), "org", "repo", 1, "in_progress", "", "Starting deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateDeploymentStatus_TruncatesLongDescription(t *testing.T) {
	longDescription := strings.Repeat("deploy failed because configuration is invalid; ", 5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if got := body["description"]; len([]rune(got)) > githubDeploymentStatusDescriptionLimit {
			t.Fatalf("description length = %d, want <= %d", len([]rune(got)), githubDeploymentStatusDescriptionLimit)
		}
		if got := body["description"]; !strings.HasSuffix(got, "...") {
			t.Fatalf("description = %q, want truncated suffix", got)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"id": 101})
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	err := client.UpdateDeploymentStatus(context.Background(), "org", "repo", 1, "failure", "", longDescription)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateDeploymentStatus_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Not Found"}`)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	err := client.UpdateDeploymentStatus(context.Background(), "org", "repo", 999, "success", "", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

// ============================================================
// FetchLatestRelease
// ============================================================

func TestFetchLatestRelease_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/repos/fanboykun/watcher/releases/latest"
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q, want %q", r.URL.Path, expectedPath)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing Accept header")
		}

		release := githubRelease{
			TagName:     "v2.0.0",
			PublishedAt: "2024-06-15T10:00:00Z",
			Assets: []githubAsset{
				{ID: 1, Name: "watcher-v2.0.0.zip", BrowserDownloadURL: "https://github.com/fanboykun/watcher/releases/download/v2.0.0/watcher-v2.0.0.zip"},
				{ID: 2, Name: "checksums.txt", BrowserDownloadURL: "https://github.com/fanboykun/watcher/releases/download/v2.0.0/checksums.txt"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	// Use real GitHub URL format — ParseGitHubURL needs this.
	// apiBase is already overridden to point at the test server.
	release, err := client.FetchLatestRelease(context.Background(), "https://github.com/fanboykun/watcher")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release.TagName != "v2.0.0" {
		t.Errorf("tag = %q, want %q", release.TagName, "v2.0.0")
	}
	if len(release.Assets) != 2 {
		t.Errorf("expected 2 assets, got %d", len(release.Assets))
	}
	if release.Assets[0].Name != "watcher-v2.0.0.zip" {
		t.Errorf("first asset = %q, want %q", release.Assets[0].Name, "watcher-v2.0.0.zip")
	}
}

func TestFetchLatestRelease_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient("ghp_token", server)
	_, err := client.FetchLatestRelease(context.Background(), "https://github.com/fanboykun/watcher")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestFetchLatestRelease_InvalidRepoURL(t *testing.T) {
	client := NewGitHubClient("ghp_token", newTestLogger())
	_, err := client.FetchLatestRelease(context.Background(), "not-a-url")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestDeriveServiceNameFromAsset(t *testing.T) {
	tests := []struct {
		asset string
		want  string
	}{
		{asset: "fktool-v0.1.1-windows-amd64.zip", want: "fktool"},
		{asset: "fktool-v0.1.1-windows-amd64", want: "fktool"},
		{asset: "fktool-windows-amd64-v0.1.1.zip", want: "fktool"},
		{asset: "my-api-service-v1.2.3-linux-arm64.tar.gz", want: "my-api-service"},
		{asset: "watcher.exe", want: "watcher"},
		{asset: "checksums.txt", want: "checksums.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			if got := deriveServiceNameFromAsset(tt.asset); got != tt.want {
				t.Fatalf("deriveServiceNameFromAsset(%q) = %q, want %q", tt.asset, got, tt.want)
			}
		})
	}
}

func TestFetchMetadataFromRepo_DerivesServiceNamesFromAssets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/fanboykun/fktool/releases/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		release := githubRelease{
			TagName:     "v0.1.1",
			PublishedAt: "2026-03-27T09:00:00Z",
			Assets: []githubAsset{
				{
					ID:                 1,
					Name:               "fktool-v0.1.1-windows-amd64.zip",
					BrowserDownloadURL: "https://github.com/fanboykun/fktool/releases/download/v0.1.1/fktool-v0.1.1-windows-amd64.zip",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(release)
	}))
	defer server.Close()

	client := newTestClient("", server)
	meta, err := client.FetchMetadataFromRepo(context.Background(), "https://github.com/fanboykun/fktool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc, ok := meta.Services["fktool"]
	if !ok {
		t.Fatalf("expected service key fktool, got keys: %v", keys(meta.Services))
	}
	if svc.Version != "v0.1.1" {
		t.Fatalf("version = %q, want %q", svc.Version, "v0.1.1")
	}
	if svc.Artifact != "fktool-v0.1.1-windows-amd64.zip" {
		t.Fatalf("artifact = %q", svc.Artifact)
	}
}
