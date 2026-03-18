package internal

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

// ============================================================
// parseGitHubURL
// ============================================================

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantOwner   string
		wantRepo    string
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
			name:            "missing https scheme",
			url:             "github.com/my-org/my-repo/releases/latest/download/version.json",
			wantErrContains: "unexpected URL format",
		},
		{
			name:            "only owner no repo",
			url:             "https://github.com/my-org",
			wantErrContains: "cannot extract owner/repo",
		},
		{
			name:            "empty string",
			url:             "",
			wantErrContains: "unexpected URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseGitHubURL(tt.url)
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
	if !strings.Contains(err.Error(), "github_token") {
		t.Errorf("expected hint about github_token in error, got: %v", err)
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