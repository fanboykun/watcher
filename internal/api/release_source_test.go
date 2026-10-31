package api

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestCreateWatcherValidatesReleaseSource(t *testing.T) {
	router, db, _, _ := candidateAPIFixture(t)
	tests := []struct {
		name       string
		source     string
		url        string
		ref        string
		wantCode   int
		wantSource string
		wantURL    string
		wantRef    string
	}{
		{name: "default is github", url: "https://github.com/org/app", wantCode: http.StatusCreated, wantSource: agent.SourceGitHub, wantURL: "https://github.com/org/app"},
		{name: "http feed root normalized", source: "HTTP", url: "https://releases.internal/app/version.json", wantCode: http.StatusCreated, wantSource: agent.SourceHTTP, wantURL: "https://releases.internal/app"},
		{name: "http feed accepts pinned slash tag", source: "http", url: "https://releases.internal/app", ref: "services/api/v1.0.0", wantCode: http.StatusCreated, wantSource: agent.SourceHTTP, wantURL: "https://releases.internal/app", wantRef: "services/api/v1.0.0"},
		{name: "http feed rejects path-escaping ref", source: "http", url: "https://releases.internal/app", ref: "../v1.0.0", wantCode: http.StatusBadRequest},
		{name: "http feed rejects non-http url", source: "http", url: "\\\\fileserver\\releases", wantCode: http.StatusBadRequest},
		{name: "unknown source rejected", source: "s3", url: "https://releases.internal/app", wantCode: http.StatusBadRequest},
		{name: "fs feed UNC root normalized", source: "fs", url: `\\fileserver\releases\app\version.json`, wantCode: http.StatusCreated, wantSource: agent.SourceFS, wantURL: `\\fileserver\releases\app`},
		{name: "fs feed rejects relative path", source: "fs", url: `releases\app`, wantCode: http.StatusBadRequest},
		{name: "fs feed rejects URL", source: "fs", url: "https://releases.internal/app", wantCode: http.StatusBadRequest},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"name": tt.name, "service_name": "api", "source_type": tt.source, "metadata_url": tt.url,
				"release_ref": tt.ref, "install_dir": filepath.Join(t.TempDir(), fmt.Sprint(i)),
			})
			rec := authRequest(router, http.MethodPost, "/api/watchers", string(body), "watcher")
			if rec.Code != tt.wantCode {
				t.Fatalf("status=%d body=%s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
			if tt.wantCode != http.StatusCreated {
				return
			}
			var created database.Watcher
			if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			var stored database.Watcher
			if err := db.First(&stored, created.ID).Error; err != nil {
				t.Fatal(err)
			}
			wantRef := tt.wantRef
			if wantRef == "" {
				wantRef = "latest"
			}
			if stored.SourceType != tt.wantSource || stored.MetadataURL != tt.wantURL || stored.ReleaseRef != wantRef {
				t.Fatalf("stored source=%q url=%q ref=%q", stored.SourceType, stored.MetadataURL, stored.ReleaseRef)
			}
		})
	}
}

func TestUpdateWatcherSwitchesToFeedSource(t *testing.T) {
	router, db, watcher, _ := candidateAPIFixture(t)
	path := fmt.Sprintf("/api/watchers/%d", watcher.ID)

	rec := authRequest(router, http.MethodPut, path, `{"source_type":"http","metadata_url":"https://releases.internal/api/","release_ref":"latest"}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("switch to feed: %d %s", rec.Code, rec.Body.String())
	}
	var stored database.Watcher
	if err := db.First(&stored, watcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SourceType != agent.SourceHTTP || stored.MetadataURL != "https://releases.internal/api" {
		t.Fatalf("stored source=%q url=%q", stored.SourceType, stored.MetadataURL)
	}

	// The stored source still constrains later partial updates.
	rec = authRequest(router, http.MethodPut, path, `{"release_ref":"v2.0.0/.."}`, "watcher")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("path-escaping ref on feed: %d %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestInspectFeedSourceNeverSendsGitHubToken(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("GitHub token sent to feed host")
		}
		if r.URL.Path != "/feeds/api/version.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"services":{"api":{"version":"v3","artifact":"api.zip","artifact_url":"releases/v3/api.zip","app_kind":"nssm","binary_name":"api.exe"}}}`))
	}))
	defer feed.Close()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(db, "nssm", t.TempDir(), "test", "global-github-token", ".env", &config.AppConfig{}, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), make(chan struct{}, 1), nil, nil)

	body := fmt.Sprintf(`{"source_type":"http","repo_url":%q,"github_token":"inspect-token"}`, feed.URL+"/feeds/api/version.json")
	rec := authRequest(router, http.MethodPost, "/api/sources/inspect", body, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body.String())
	}
	var resp agent.InspectRepoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	svc := resp.Services["api"]
	if resp.Source != "manifest" || resp.MetadataURL != feed.URL+"/feeds/api" || resp.LatestVersion != "v3" || svc.ArtifactURL != feed.URL+"/feeds/api/releases/v3/api.zip" {
		t.Fatalf("inspect response = %+v", resp)
	}

	rec = authRequest(router, http.MethodPost, "/api/sources/inspect", fmt.Sprintf(`{"source_type":"http","repo_url":%q}`, feed.URL+"/missing"), "watcher")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "HTTP 404") {
		t.Fatalf("missing feed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestReleaseCatalogServesFeedReleases(t *testing.T) {
	artifact := zipBytes(t, map[string]string{"api.exe": "v1"})
	sum := sha256.Sum256(artifact)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/releases.json":
			fmt.Fprintf(w, `{"releases":[
				{"tag":"v2","published_at":"2026-10-30T00:00:00Z","services":{"worker":{"version":"v2","artifact_url":"releases/v2/worker.zip"}}},
				{"tag":"v1","name":"First","prerelease":true,"services":{"api":{"version":"v1","artifact":"api.zip","artifact_url":"releases/v1/api.zip","sha256":"%x","size":%d}}}
			]}`, sum, len(artifact))
		case "/api/releases/v1/api.zip":
			_, _ = w.Write(artifact)
		default:
			http.NotFound(w, r)
		}
	}))
	defer feed.Close()
	router, db, watcher, _ := candidateAPIFixture(t)
	if err := db.Model(&watcher).UpdateColumns(map[string]any{"source_type": agent.SourceHTTP, "metadata_url": feed.URL + "/api", "pending_version": "", "status": "healthy"}).Error; err != nil {
		t.Fatal(err)
	}

	rec := authRequest(router, http.MethodGet, fmt.Sprintf("/api/watchers/%d/releases", watcher.ID), "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var catalog struct {
		Releases   []agent.CatalogRelease `json:"releases"`
		Repository string                 `json:"repository"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	// Only releases publishing this watcher's service are listed.
	if len(catalog.Releases) != 1 || catalog.Releases[0].TagName != "v1" || !catalog.Releases[0].Prerelease || catalog.Repository != feed.URL+"/api" {
		t.Fatalf("catalog = %+v", catalog)
	}
	release := catalog.Releases[0]
	asset := release.Assets[0]
	if asset.Name != "api.zip" || asset.Size != int64(len(artifact)) || asset.Digest != fmt.Sprintf("sha256:%x", sum) {
		t.Fatalf("asset = %+v", asset)
	}

	body := fmt.Sprintf(`{"release_id":%d,"asset_id":%d}`, release.ID, asset.ID)
	rec = authRequest(router, http.MethodPost, fmt.Sprintf("/api/watchers/%d/releases/download", watcher.ID), body, "watcher")
	if rec.Code != http.StatusCreated {
		t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
	}
	var staged database.CatalogArtifact
	if err := json.Unmarshal(rec.Body.Bytes(), &staged); err != nil {
		t.Fatal(err)
	}
	if staged.Version != "v1" || staged.SHA256 != fmt.Sprintf("%x", sum) || staged.Repository != feed.URL+"/api" {
		t.Fatalf("staged = %+v", staged)
	}
	var stored database.Watcher
	if err := db.First(&stored, watcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	var persisted database.CatalogArtifact
	if err := db.First(&persisted, staged.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := agent.ValidateCatalogArtifact(&stored, &persisted); err != nil {
		t.Fatalf("staged feed artifact does not validate: %v", err)
	}
}

func TestInspectFSFeedSource(t *testing.T) {
	router, _, _, _ := candidateAPIFixture(t)
	feed := t.TempDir()
	manifest := `{"services":{"api":{"version":"v4","artifact_url":"releases/v4/api.zip"}}}`
	if err := os.WriteFile(filepath.Join(feed, "version.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"source_type": "fs", "repo_url": feed})
	rec := authRequest(router, http.MethodPost, "/api/sources/inspect", string(body), "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body.String())
	}
	var resp agent.InspectRepoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.MetadataURL != feed || resp.Services["api"].ArtifactURL != filepath.Join(feed, "releases", "v4", "api.zip") {
		t.Fatalf("inspect response = %+v", resp)
	}
}

func TestFeedCredentialsAreStoredMaskedAndScopedToHTTP(t *testing.T) {
	router, db, _, _ := candidateAPIFixture(t)
	body, _ := json.Marshal(map[string]any{
		"name": "feed", "service_name": "api", "source_type": "http", "metadata_url": "https://releases.internal/api",
		"install_dir": t.TempDir(), "feed_auth_type": "bearer", "feed_auth_secret": "feed-secret-token",
	})
	rec := authRequest(router, http.MethodPost, "/api/watchers", string(body), "watcher")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "feed-secret-token") {
		t.Fatalf("API response leaked the feed secret: %s", rec.Body.String())
	}
	var created struct {
		ID                   uint   `json:"id"`
		HasFeedAuthSecret    bool   `json:"has_feed_auth_secret"`
		FeedAuthSecretMasked string `json:"feed_auth_secret_masked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.HasFeedAuthSecret || created.FeedAuthSecretMasked != "feed*********oken" {
		t.Fatalf("masked secret = %+v", created)
	}
	path := fmt.Sprintf("/api/watchers/%d", created.ID)
	stored := func() database.Watcher {
		var w database.Watcher
		if err := db.First(&w, created.ID).Error; err != nil {
			t.Fatal(err)
		}
		return w
	}

	// A blank secret keeps the saved one while other fields change.
	rec = authRequest(router, http.MethodPut, path, `{"feed_auth_type":"header","feed_auth_header":"X-Api-Key","feed_auth_secret":""}`, "watcher")
	if got := stored(); rec.Code != http.StatusOK || got.FeedAuthType != "header" || got.FeedAuthHeader != "X-Api-Key" || got.FeedAuthSecret != "feed-secret-token" {
		t.Fatalf("keep secret: %d %s, stored %+v", rec.Code, rec.Body.String(), got)
	}
	rec = authRequest(router, http.MethodPut, path, `{"feed_auth_type":"bearer","feed_auth_header":"","feed_auth_secret":""}`, "watcher")
	if got := stored(); rec.Code != http.StatusOK || got.FeedAuthType != "bearer" || got.FeedAuthHeader != "" {
		t.Fatalf("switch kind: %d %s, stored %+v", rec.Code, rec.Body.String(), got)
	}
	// Credentials cannot be attached to a file share feed.
	rec = authRequest(router, http.MethodPut, path, `{"source_type":"fs","metadata_url":"\\\\fileserver\\releases\\api","feed_auth_type":"bearer"}`, "watcher")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("fs with credential: %d %s", rec.Code, rec.Body.String())
	}
	// Leaving http clears the credential.
	rec = authRequest(router, http.MethodPut, path, `{"source_type":"github","metadata_url":"https://github.com/org/api"}`, "watcher")
	if got := stored(); rec.Code != http.StatusOK || got.FeedAuthType != "" || got.FeedAuthSecret != "" {
		t.Fatalf("leave http: %d %s, stored %+v", rec.Code, rec.Body.String(), got)
	}
}

func TestInspectAuthenticatedFeed(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "deploy" || pass != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"services":{"api":{"version":"v5","artifact_url":"releases/v5/api.zip"}}}`))
	}))
	defer feed.Close()
	router, _, _, _ := candidateAPIFixture(t)

	body := fmt.Sprintf(`{"source_type":"http","repo_url":%q,"feed_auth_type":"basic","feed_auth_username":"deploy","feed_auth_secret":"pw"}`, feed.URL)
	if rec := authRequest(router, http.MethodPost, "/api/sources/inspect", body, "watcher"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"latest_version":"v5"`) {
		t.Fatalf("authenticated inspect: %d %s", rec.Code, rec.Body.String())
	}
	body = fmt.Sprintf(`{"source_type":"http","repo_url":%q}`, feed.URL)
	if rec := authRequest(router, http.MethodPost, "/api/sources/inspect", body, "watcher"); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "feed authentication failed") {
		t.Fatalf("anonymous inspect: %d %s", rec.Code, rec.Body.String())
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPollFilterUpToDateIncludesUnchangedFeedPolls(t *testing.T) {
	router, db, watcher, _ := candidateAPIFixture(t)
	for _, status := range []string{"no_update", "unchanged", "error", "new_release"} {
		if err := db.Create(&database.PollEvent{WatcherID: watcher.ID, Status: status}).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := authRequest(router, http.MethodGet, fmt.Sprintf("/api/watchers/%d/polls?status=up_to_date", watcher.ID), "", "watcher")
	var page struct {
		Data  []database.PollEvent `json:"data"`
		Total int64                `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("polls: %d %s", rec.Code, rec.Body.String())
	}
	if page.Total != 2 || len(page.Data) != 2 {
		t.Fatalf("up_to_date polls = %+v", page)
	}
}

func TestS3SourceRequiresSigV4Credentials(t *testing.T) {
	router, db, _, _ := candidateAPIFixture(t)
	create := func(fields map[string]any) *httptest.ResponseRecorder {
		body := map[string]any{"name": "s3", "service_name": "api", "metadata_url": "https://minio.internal:9000/releases/api", "install_dir": t.TempDir()}
		for k, v := range fields {
			body[k] = v
		}
		data, _ := json.Marshal(body)
		return authRequest(router, http.MethodPost, "/api/watchers", string(data), "watcher")
	}
	for name, fields := range map[string]map[string]any{
		"s3 without credentials": {"source_type": "s3"},
		"s3 with bearer":         {"source_type": "s3", "feed_auth_type": "bearer", "feed_auth_secret": "t"},
		"s3 without access key":  {"source_type": "s3", "feed_auth_type": "sigv4", "feed_auth_secret": "s"},
		"http with sigv4":        {"source_type": "http", "feed_auth_type": "sigv4", "feed_auth_username": "AK", "feed_auth_secret": "s"},
		"fs with sigv4":          {"source_type": "fs", "metadata_url": `\\fileserver\releases\api`, "feed_auth_type": "sigv4", "feed_auth_username": "AK", "feed_auth_secret": "s"},
	} {
		if rec := create(fields); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s, want 400", name, rec.Code, rec.Body.String())
		}
	}

	rec := create(map[string]any{"source_type": "s3", "feed_auth_type": "sigv4", "feed_auth_username": "AKIAEXAMPLE", "feed_auth_secret": "s3-secret-key-value"})
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), "s3-secret-key-value") {
		t.Fatalf("create s3: %d %s", rec.Code, rec.Body.String())
	}
	var created database.Watcher
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	var stored database.Watcher
	if err := db.First(&stored, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.SourceType != agent.SourceS3 || stored.FeedAuthType != agent.FeedAuthSigV4 || stored.FeedAuthUsername != "AKIAEXAMPLE" || stored.FeedAuthRegion != "us-east-1" || stored.FeedAuthSecret != "s3-secret-key-value" {
		t.Fatalf("stored = %+v", stored)
	}
}
