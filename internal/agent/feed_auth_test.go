package agent

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeFeedAuth(t *testing.T) {
	tests := []struct {
		name    string
		in      FeedAuth
		want    FeedAuth
		wantErr string
	}{
		{name: "none clears fields", in: FeedAuth{Username: "u", Secret: "s"}, want: FeedAuth{}},
		{name: "bearer drops unused fields", in: FeedAuth{Type: " Bearer ", Username: "u", Header: "X", Secret: " tok "}, want: FeedAuth{Type: FeedAuthBearer, Secret: "tok"}},
		{name: "basic", in: FeedAuth{Type: "basic", Username: "deploy", Secret: "pw"}, want: FeedAuth{Type: FeedAuthBasic, Username: "deploy", Secret: "pw"}},
		{name: "header", in: FeedAuth{Type: "header", Header: "X-JFrog-Art-Api", Secret: "key"}, want: FeedAuth{Type: FeedAuthHeader, Header: "X-JFrog-Art-Api", Secret: "key"}},
		{name: "basic needs username", in: FeedAuth{Type: "basic", Secret: "pw"}, wantErr: "username"},
		{name: "basic rejects colon", in: FeedAuth{Type: "basic", Username: "a:b", Secret: "pw"}, wantErr: "username"},
		{name: "header needs valid name", in: FeedAuth{Type: "header", Header: "Bad Header", Secret: "k"}, wantErr: "header name"},
		{name: "header rejects Host", in: FeedAuth{Type: "header", Header: "host", Secret: "k"}, wantErr: "cannot carry"},
		{name: "secret required", in: FeedAuth{Type: "bearer"}, wantErr: "secret is required"},
		{name: "secret single line", in: FeedAuth{Type: "bearer", Secret: "a\r\nX-Injected: 1"}, wantErr: "single line"},
		{name: "unknown type", in: FeedAuth{Type: "digest", Secret: "s"}, wantErr: "must be one of"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeFeedAuth(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

// credentialLog records which credential headers each path received.
type credentialLog struct {
	mu   sync.Mutex
	seen map[string]string
}

func (l *credentialLog) handler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.seen[r.URL.Path] = r.Header.Get("Authorization") + r.Header.Get("X-Api-Key")
		l.mu.Unlock()
		next(w, r)
	}
}

func (l *credentialLog) get(path string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.seen[path]
}

func TestHTTPFeedSource_CredentialsStayOnFeedOrigin(t *testing.T) {
	tests := []struct {
		name string
		auth FeedAuth
		want string
	}{
		{name: "bearer", auth: FeedAuth{Type: FeedAuthBearer, Secret: "tok"}, want: "Bearer tok"},
		{name: "basic", auth: FeedAuth{Type: FeedAuthBasic, Username: "deploy", Secret: "pw"}, want: "Basic " + base64.StdEncoding.EncodeToString([]byte("deploy:pw"))},
		{name: "header", auth: FeedAuth{Type: FeedAuthHeader, Header: "X-Api-Key", Secret: "key"}, want: "key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := &credentialLog{seen: map[string]string{}}
			cdn := httptest.NewServer(other.handler(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("zip")) }))
			defer cdn.Close()
			feedLog := &credentialLog{seen: map[string]string{}}
			feed := httptest.NewServer(feedLog.handler(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/app/version.json":
					_, _ = w.Write([]byte(`{"services":{"api":{"version":"v1","artifact_url":"releases/v1/api.zip"},"cdn":{"version":"v1","artifact_url":"` + cdn.URL + `/api.zip"}}}`))
				case "/app/releases/v1/api.zip":
					_, _ = w.Write([]byte("zip"))
				case "/app/moved.zip":
					http.Redirect(w, r, cdn.URL+"/redirected.zip", http.StatusFound)
				default:
					http.NotFound(w, r)
				}
			}))
			defer feed.Close()

			source := newHTTPFeedSource(feed.URL+"/app", tt.auth, newTestLogger())
			meta, err := source.FetchMetadata(context.Background(), "latest", "api")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for i, artifact := range []string{meta.Services["api"].ArtifactURL, meta.Services["cdn"].ArtifactURL, feed.URL + "/app/moved.zip"} {
				if err := source.DownloadArtifact(context.Background(), artifact, filepath.Join(dir, string(rune('a'+i))+".zip"), 1); err != nil {
					t.Fatalf("download %s: %v", artifact, err)
				}
			}

			for _, path := range []string{"/app/version.json", "/app/releases/v1/api.zip", "/app/moved.zip"} {
				if got := feedLog.get(path); got != tt.want {
					t.Errorf("feed %s credential = %q, want %q", path, got, tt.want)
				}
			}
			for _, path := range []string{"/api.zip", "/redirected.zip"} {
				if got := other.get(path); got != "" {
					t.Errorf("credential leaked to other origin %s: %q", path, got)
				}
			}
		})
	}
}

func TestSameOrigin(t *testing.T) {
	parse := func(raw string) *url.URL {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	tests := []struct {
		a, b string
		want bool
	}{
		{a: "https://feed.internal/app", b: "https://FEED.internal:443/other", want: true},
		{a: "http://feed.internal/app", b: "http://feed.internal:80/x", want: true},
		{a: "https://feed.internal/app", b: "http://feed.internal/app", want: false},
		{a: "https://feed.internal/app", b: "https://feed.internal:8443/app", want: false},
		{a: "https://feed.internal/app", b: "https://cdn.internal/app", want: false},
	}
	for _, tt := range tests {
		if got := sameOrigin(parse(tt.a), parse(tt.b)); got != tt.want {
			t.Errorf("sameOrigin(%s, %s) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestHTTPFeedSource_AuthenticationFailure(t *testing.T) {
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer feed.Close()
	source := newHTTPFeedSource(feed.URL, FeedAuth{Type: FeedAuthBearer, Secret: "super-secret"}, newTestLogger())

	_, err := source.FetchMetadata(context.Background(), "latest", "api")
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed (HTTP 401)") || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("metadata error = %v", err)
	}
	err = source.DownloadArtifact(context.Background(), feed.URL+"/api.zip", filepath.Join(t.TempDir(), "api.zip"), 1)
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed") || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("download error = %v", err)
	}
}
