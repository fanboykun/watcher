package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestProcessIndexHTML(t *testing.T) {
	sampleHTML := `<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8" />
		<link href="/_app/immutable/entry/start.123.js" rel="modulepreload">
		<link href="/_app/immutable/chunks/abc.js" rel="modulepreload">
		<script src="/_app/immutable/vendor.js"></script>
	</head>
	<body>
		<script>
			{
				__sveltekit_xyz = {
					base: ""
				};
				Promise.all([
					import("/_app/immutable/entry/start.123.js"),
					import('/_app/immutable/entry/app.456.js')
				]).then(([kit, app]) => {
					kit.start(app, element);
				});
			}
		</script>
	</body>
</html>`

	t.Run("empty base path returns original content", func(t *testing.T) {
		res := processIndexHTML([]byte(sampleHTML), "")
		if string(res) != sampleHTML {
			t.Errorf("expected content to be unchanged")
		}
	})

	t.Run("subpath transforms all paths and config", func(t *testing.T) {
		res := string(processIndexHTML([]byte(sampleHTML), "/watcher"))

		if !strings.Contains(res, `base: "/watcher", assets: "/watcher"`) {
			t.Errorf("expected sveltekit base config to be updated, got:\n%s", res)
		}
		if !strings.Contains(res, `href="/watcher/_app/immutable/entry/start.123.js"`) {
			t.Errorf("expected href to be prefixed")
		}
		if !strings.Contains(res, `src="/watcher/_app/immutable/vendor.js"`) {
			t.Errorf("expected src to be prefixed")
		}
		if !strings.Contains(res, `import("/watcher/_app/immutable/entry/start.123.js")`) {
			t.Errorf("expected double-quote import to be prefixed")
		}
		if !strings.Contains(res, `import('/watcher/_app/immutable/entry/app.456.js')`) {
			t.Errorf("expected single-quote import to be prefixed")
		}
		if strings.Contains(res, `"/_app/`) {
			t.Errorf("expected no remaining root-relative /_app/ paths, got:\n%s", res)
		}
	})

	t.Run("does not double prefix if already prefixed", func(t *testing.T) {
		prefixed := string(processIndexHTML([]byte(sampleHTML), "/watcher"))
		secondPass := string(processIndexHTML([]byte(prefixed), "/watcher"))
		if secondPass != prefixed {
			t.Errorf("expected second pass to be idempotent, got:\n%s", secondPass)
		}
	})
}

func TestRouterWithWebAssetsPath(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	cfg := &config.AppConfig{
		APIPort:       "8080",
		LogDir:        t.TempDir(),
		WebAssetsPath: "/watcher",
	}
	r := NewRouter(
		db, "nssm", cfg.LogDir, "test", "", ".env", cfg,
		agent.NewLoggerWithWriter("api", io.Discard, "error"),
		nil, make(chan uint, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1),
	)

	t.Run("redirects base path without trailing slash", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/watcher", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusMovedPermanently {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
		}
		if loc := rec.Header().Get("Location"); loc != "/watcher/" {
			t.Fatalf("location = %q, want /watcher/", loc)
		}
	})

	t.Run("serves transformed index.html at base path trailing slash", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/watcher/", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `base: "/watcher"`) {
			t.Fatalf("expected transformed index.html to contain base: \"/watcher\", got:\n%s", body)
		}
		if !strings.Contains(body, `href="/watcher/_app/`) {
			t.Fatalf("expected transformed index.html to contain prefixed href, got:\n%s", body)
		}
	})

	t.Run("serves static file under base path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/watcher/_app/version.json", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("serves static file from root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/_app/version.json", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("serves SPA fallback under base path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/watcher/watchers/123", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `base: "/watcher"`) {
			t.Fatalf("expected transformed index.html for SPA route fallback, got:\n%s", body)
		}
	})

	t.Run("serves API under base path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/watcher/api/status", nil)
		req.Header.Set("Authorization", "Bearer watcher")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var res map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if res["status"] != "running" {
			t.Fatalf("expected status 'running', got %v", res["status"])
		}
	})

	t.Run("serves API under root", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.Header.Set("Authorization", "Bearer watcher")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var res map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if res["status"] != "running" {
			t.Fatalf("expected status 'running', got %v", res["status"])
		}
	})
}

func TestSelfConfigWebAssetsPath(t *testing.T) {
	envFile := filepath.Join(t.TempDir(), ".env")
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	cfg := &config.AppConfig{
		LogLevel:      "info",
		LogMaxSizeMB:  100,
		APIPort:       "8080",
		LogDir:        t.TempDir(),
		WebAssetsPath: "/initial-path",
	}
	r := NewRouter(
		db, "nssm", cfg.LogDir, "test", "", envFile, cfg,
		agent.NewLoggerWithWriter("api", io.Discard, "error"),
		nil, make(chan uint, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1),
	)

	// GET /api/self/config (authenticated)
	req := httptest.NewRequest(http.MethodGet, "/api/self/config", nil)
	req.Header.Set("Authorization", "Bearer watcher")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var confResp SelfConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &confResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if confResp.WebAssetsPath != "/initial-path" {
		t.Fatalf("WebAssetsPath = %q, want /initial-path", confResp.WebAssetsPath)
	}

	// PUT /api/self/config to change WebAssetsPath
	newPath := "/watcher"
	updateBody, _ := json.Marshal(UpdateSelfConfigRequest{
		WebAssetsPath: &newPath,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/self/config", bytes.NewReader(updateBody))
	putReq.Header.Set("Authorization", "Bearer watcher")
	putReq.Header.Set("Content-Type", "application/json")
	putRec := httptest.NewRecorder()
	r.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", putRec.Code, http.StatusOK, putRec.Body.String())
	}
	var updateResp struct {
		Config SelfConfigResponse `json:"config"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &updateResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if updateResp.Config.WebAssetsPath != "/watcher" {
		t.Fatalf("WebAssetsPath after update = %q, want /watcher", updateResp.Config.WebAssetsPath)
	}
	if cfg.WebAssetsPath != "/watcher" {
		t.Fatalf("in-memory cfg.WebAssetsPath = %q, want /watcher", cfg.WebAssetsPath)
	}
}

func TestRouterDynamicRuntimeWebAssetsPath(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	cfg := &config.AppConfig{
		LogLevel:      "info",
		LogMaxSizeMB:  100,
		APIPort:       "8080",
		LogDir:        t.TempDir(),
		WebAssetsPath: "", // Start empty
	}
	r := NewRouter(
		db, "nssm", cfg.LogDir, "test", "", ".env", cfg,
		agent.NewLoggerWithWriter("api", io.Discard, "error"),
		nil, make(chan uint, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1),
	)

	// 1. Initial request to root serves standard index.html
	reqRoot := httptest.NewRequest(http.MethodGet, "/", nil)
	recRoot := httptest.NewRecorder()
	r.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusOK {
		t.Fatalf("initial root status = %d, want %d", recRoot.Code, http.StatusOK)
	}
	if strings.Contains(recRoot.Body.String(), `base: "/watcher"`) {
		t.Fatalf("did not expect base /watcher in initial root")
	}

	// 2. Change WebAssetsPath dynamically at runtime without restarting router
	cfg.WebAssetsPath = "/watcher"

	// 3. Requesting dynamic base path immediately redirects and serves transformed HTML
	reqWatcherSlash := httptest.NewRequest(http.MethodGet, "/watcher/", nil)
	recWatcherSlash := httptest.NewRecorder()
	r.ServeHTTP(recWatcherSlash, reqWatcherSlash)
	if recWatcherSlash.Code != http.StatusOK {
		t.Fatalf("dynamic /watcher/ status = %d, want %d", recWatcherSlash.Code, http.StatusOK)
	}
	if !strings.Contains(recWatcherSlash.Body.String(), `base: "/watcher"`) {
		t.Fatalf("expected transformed base /watcher at runtime, got:\n%s", recWatcherSlash.Body.String())
	}

	// 4. Requesting API under dynamic base path immediately routes to API handler
	reqAPI := httptest.NewRequest(http.MethodGet, "/watcher/api/status", nil)
	reqAPI.Header.Set("Authorization", "Bearer watcher")
	recAPI := httptest.NewRecorder()
	r.ServeHTTP(recAPI, reqAPI)
	if recAPI.Code != http.StatusOK {
		t.Fatalf("dynamic /watcher/api/status = %d, want %d; body=%s", recAPI.Code, http.StatusOK, recAPI.Body.String())
	}

	// 5. Change again dynamically at runtime to /socfindo
	cfg.WebAssetsPath = "/socfindo"

	reqSocfindoSlash := httptest.NewRequest(http.MethodGet, "/socfindo/", nil)
	recSocfindoSlash := httptest.NewRecorder()
	r.ServeHTTP(recSocfindoSlash, reqSocfindoSlash)
	if recSocfindoSlash.Code != http.StatusOK {
		t.Fatalf("dynamic /socfindo/ status = %d, want %d", recSocfindoSlash.Code, http.StatusOK)
	}
	if !strings.Contains(recSocfindoSlash.Body.String(), `base: "/socfindo"`) {
		t.Fatalf("expected transformed base /socfindo at runtime, got:\n%s", recSocfindoSlash.Body.String())
	}

	reqSocfindoAPI := httptest.NewRequest(http.MethodGet, "/socfindo/api/status", nil)
	reqSocfindoAPI.Header.Set("Authorization", "Bearer watcher")
	recSocfindoAPI := httptest.NewRecorder()
	r.ServeHTTP(recSocfindoAPI, reqSocfindoAPI)
	if recSocfindoAPI.Code != http.StatusOK {
		t.Fatalf("dynamic /socfindo/api/status = %d, want %d; body=%s", recSocfindoAPI.Code, http.StatusOK, recSocfindoAPI.Body.String())
	}

	// 6. Test reverse proxy X-Forwarded-Prefix header when config is empty
	cfg.WebAssetsPath = ""
	reqProxy := httptest.NewRequest(http.MethodGet, "/proxied/", nil)
	reqProxy.Header.Set("X-Forwarded-Prefix", "/proxied")
	recProxy := httptest.NewRecorder()
	r.ServeHTTP(recProxy, reqProxy)
	if recProxy.Code != http.StatusOK {
		t.Fatalf("X-Forwarded-Prefix /proxied/ status = %d, want %d", recProxy.Code, http.StatusOK)
	}
	if !strings.Contains(recProxy.Body.String(), `base: "/proxied"`) {
		t.Fatalf("expected base /proxied from X-Forwarded-Prefix, got:\n%s", recProxy.Body.String())
	}
}
