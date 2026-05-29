package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestAuthLoginSucceedsAndFails(t *testing.T) {
	t.Parallel()

	r := newTestAuthRouter(t)

	bad := authRequest(r, http.MethodPost, "/api/auth/login", `{"password":"wrong"}`, "")
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want %d", bad.Code, http.StatusUnauthorized)
	}

	good := authRequest(r, http.MethodPost, "/api/auth/login", `{"password":"watcher"}`, "")
	if good.Code != http.StatusOK {
		t.Fatalf("good login status = %d, want %d; body=%s", good.Code, http.StatusOK, good.Body.String())
	}
}

func TestAuthMiddlewareProtectsAPI(t *testing.T) {
	t.Parallel()

	r := newTestAuthRouter(t)

	missing := authRequest(r, http.MethodGet, "/api/status", "", "")
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status = %d, want %d", missing.Code, http.StatusUnauthorized)
	}

	invalid := authRequest(r, http.MethodGet, "/api/status", "", "wrong")
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("invalid auth status = %d, want %d", invalid.Code, http.StatusUnauthorized)
	}

	valid := authRequest(r, http.MethodGet, "/api/status", "", "watcher")
	if valid.Code != http.StatusOK {
		t.Fatalf("valid auth status = %d, want %d; body=%s", valid.Code, http.StatusOK, valid.Body.String())
	}
}

func TestAuthPasswordChangeClearsDefaultFlag(t *testing.T) {
	t.Parallel()

	r := newTestAuthRouter(t)

	changed := authRequest(r, http.MethodPut, "/api/auth/password", `{"current_password":"watcher","new_password":"changed-password"}`, "watcher")
	if changed.Code != http.StatusOK {
		t.Fatalf("password change status = %d, want %d; body=%s", changed.Code, http.StatusOK, changed.Body.String())
	}

	oldPassword := authRequest(r, http.MethodGet, "/api/auth/status", "", "watcher")
	if oldPassword.Code != http.StatusUnauthorized {
		t.Fatalf("old password status = %d, want %d", oldPassword.Code, http.StatusUnauthorized)
	}

	status := authRequest(r, http.MethodGet, "/api/auth/status", "", "changed-password")
	if status.Code != http.StatusOK {
		t.Fatalf("new password status = %d, want %d; body=%s", status.Code, http.StatusOK, status.Body.String())
	}
	if strings.Contains(status.Body.String(), `"using_default_password":true`) {
		t.Fatalf("expected default flag to be false, body=%s", status.Body.String())
	}
}

func newTestAuthRouter(t *testing.T) http.Handler {
	t.Helper()

	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	return NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, nil, make(chan uint, 1), make(chan struct{}, 1))
}

func authRequest(handler http.Handler, method, path, body, password string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if password != "" {
		req.Header.Set("Authorization", "Bearer "+password)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
