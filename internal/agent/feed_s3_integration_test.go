package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/database"
)

// TestS3FeedAgainstRealStore reads a feed from a real S3-compatible bucket,
// such as one published by workflows/scripts/publish-feed.sh. It runs only
// when WATCHER_TEST_S3_URL (feed root, e.g. http://127.0.0.1:9000/releases/api),
// WATCHER_TEST_S3_ACCESS_KEY and WATCHER_TEST_S3_SECRET_KEY are set. The feed
// must publish service WATCHER_TEST_S3_SERVICE (default "api"); set
// WATCHER_TEST_S3_PINNED to also read one release by tag.
func TestS3FeedAgainstRealStore(t *testing.T) {
	root := os.Getenv("WATCHER_TEST_S3_URL")
	accessKey, secretKey := os.Getenv("WATCHER_TEST_S3_ACCESS_KEY"), os.Getenv("WATCHER_TEST_S3_SECRET_KEY")
	if root == "" || accessKey == "" || secretKey == "" {
		t.Skip("set WATCHER_TEST_S3_URL, WATCHER_TEST_S3_ACCESS_KEY and WATCHER_TEST_S3_SECRET_KEY to run")
	}
	service := os.Getenv("WATCHER_TEST_S3_SERVICE")
	if service == "" {
		service = "api"
	}
	auth, err := NormalizeFeedAuth(FeedAuth{Type: FeedAuthSigV4, Username: accessKey, Secret: secretKey, Region: os.Getenv("WATCHER_TEST_S3_REGION")})
	if err != nil {
		t.Fatal(err)
	}
	source := newFeedSource(SourceS3, root, auth, newTestLogger())
	ctx := context.Background()

	download := func(t *testing.T, svc ServiceMeta) {
		t.Helper()
		dest := filepath.Join(t.TempDir(), "artifact.zip")
		if err := source.DownloadArtifact(ctx, svc.ArtifactURL, dest, 1); err != nil {
			t.Fatal(err)
		}
		if sum, err := fileSHA256(dest); err != nil || (svc.SHA256 != "" && sum != svc.SHA256) {
			t.Fatalf("artifact sha256 = %s, manifest %s, err %v", sum, svc.SHA256, err)
		}
	}
	meta, err := source.FetchMetadata(ctx, "latest", service)
	if err != nil {
		t.Fatal(err)
	}
	download(t, meta.Services[service])
	if pinned := os.Getenv("WATCHER_TEST_S3_PINNED"); pinned != "" {
		release, err := source.FetchMetadata(ctx, pinned, service)
		if err != nil {
			t.Fatal(err)
		}
		download(t, release.Services[service])
	}

	catalog, err := NewFeedCatalog(&database.Watcher{ServiceName: service, SourceType: SourceS3, MetadataURL: root,
		FeedAuthType: auth.Type, FeedAuthUsername: auth.Username, FeedAuthRegion: auth.Region, FeedAuthSecret: auth.Secret}, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	page, err := catalog.ListReleaseCatalog(ctx, "", 1)
	if err != nil || len(page.Releases) == 0 {
		t.Fatalf("catalog: %+v, err %v", page, err)
	}
	staged := filepath.Join(t.TempDir(), "staged", "artifact.zip")
	if _, err := catalog.DownloadCatalogAsset(ctx, "", page.Releases[0].Assets[0].ID, staged, page.Releases[0].Assets[0].Digest); err != nil {
		t.Fatalf("stage %s: %v", page.Releases[0].TagName, err)
	}

	wrong := auth
	wrong.Secret = "not-the-secret"
	_, err = newFeedSource(SourceS3, root, wrong, newTestLogger()).FetchMetadata(ctx, "latest", service)
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed") {
		t.Fatalf("wrong secret error = %v", err)
	}
	_, err = newFeedSource(SourceHTTP, root, FeedAuth{}, newTestLogger()).FetchMetadata(ctx, "latest", service)
	if err == nil || !strings.Contains(err.Error(), "feed authentication failed") {
		t.Fatalf("anonymous read of a private bucket error = %v", err)
	}
}
