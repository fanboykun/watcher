package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestS3FeedAgainstRealStore reads a feed from a real S3-compatible bucket.
// It runs only when WATCHER_TEST_S3_URL (feed root, e.g.
// http://127.0.0.1:9000/releases/api), WATCHER_TEST_S3_ACCESS_KEY and
// WATCHER_TEST_S3_SECRET_KEY are set; the bucket must hold version.json and
// releases/v1/version.json for service "api", and releases.json listing v1.
func TestS3FeedAgainstRealStore(t *testing.T) {
	root := os.Getenv("WATCHER_TEST_S3_URL")
	accessKey, secretKey := os.Getenv("WATCHER_TEST_S3_ACCESS_KEY"), os.Getenv("WATCHER_TEST_S3_SECRET_KEY")
	if root == "" || accessKey == "" || secretKey == "" {
		t.Skip("set WATCHER_TEST_S3_URL, WATCHER_TEST_S3_ACCESS_KEY and WATCHER_TEST_S3_SECRET_KEY to run")
	}
	auth, err := NormalizeFeedAuth(FeedAuth{Type: FeedAuthSigV4, Username: accessKey, Secret: secretKey, Region: os.Getenv("WATCHER_TEST_S3_REGION")})
	if err != nil {
		t.Fatal(err)
	}
	source := newFeedSource(SourceS3, root, auth, newTestLogger())

	meta, err := source.FetchMetadata(context.Background(), "latest", "api")
	if err != nil {
		t.Fatal(err)
	}
	svc := meta.Services["api"]
	dest := filepath.Join(t.TempDir(), "api.zip")
	if err := source.DownloadArtifact(context.Background(), svc.ArtifactURL, dest, 1); err != nil {
		t.Fatal(err)
	}
	if sum, err := fileSHA256(dest); err != nil || (svc.SHA256 != "" && sum != svc.SHA256) {
		t.Fatalf("artifact sha256 = %s, manifest %s, err %v", sum, svc.SHA256, err)
	}
	pinned, err := source.FetchMetadata(context.Background(), "v1", "api")
	if err != nil || pinned.Services["api"].ArtifactURL != svc.ArtifactURL {
		t.Fatalf("pinned v1 = %+v, err %v; want artifact %s", pinned, err, svc.ArtifactURL)
	}
	if releases, err := source.fetchIndex(context.Background()); err != nil || len(releases) == 0 {
		t.Fatalf("releases.json: %d releases, err %v", len(releases), err)
	}

	wrong := auth
	wrong.Secret = "not-the-secret"
	_, err = newFeedSource(SourceS3, root, wrong, newTestLogger()).FetchMetadata(context.Background(), "latest", "api")
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed") {
		t.Fatalf("wrong secret error = %v", err)
	}
	_, err = newFeedSource(SourceHTTP, root, FeedAuth{}, newTestLogger()).FetchMetadata(context.Background(), "latest", "api")
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed") {
		t.Fatalf("anonymous read of a private bucket error = %v", err)
	}
}
