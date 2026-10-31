package agent

import (
	"net/http"
	"time"
)

var knownAssetSuffixTokens = map[string]struct{}{
	"windows": {}, "linux": {}, "darwin": {}, "macos": {},
	"amd64": {}, "x64": {}, "386": {}, "x86": {}, "arm64": {}, "arm": {},
}

const defaultAPIBase = "https://api.github.com"
const githubDeploymentStatusDescriptionLimit = 140

type VersionMetadata struct {
	Services map[string]ServiceMeta `json:"services"`

	// notModified marks a feed manifest served from the poll cache because the
	// feed reported it unchanged.
	notModified bool
}

type ServiceMeta struct {
	Version            string `json:"version"`
	Artifact           string `json:"artifact"`
	ArtifactURL        string `json:"artifact_url"`
	SHA256             string `json:"sha256,omitempty"` // hex digest of the artifact; verified before extraction
	Size               int64  `json:"size,omitempty"`   // artifact bytes, shown in the release catalog
	PublishedAt        string `json:"published_at"`
	AppKind            string `json:"app_kind,omitempty"`
	WindowsServiceName string `json:"windows_service_name,omitempty"`
	BinaryName         string `json:"binary_name,omitempty"`
	StartArguments     string `json:"start_arguments,omitempty"`
	EnvFile            string `json:"env_file,omitempty"`
	HealthCheckURL     string `json:"health_check_url,omitempty"`
	IISAppPool         string `json:"iis_app_pool,omitempty"`
	IISSiteName        string `json:"iis_site_name,omitempty"`
	IISManagedRuntime  string `json:"iis_managed_runtime,omitempty"`
	PublicURL          string `json:"public_url,omitempty"`
}

// githubRelease is the subset of the GitHub releases API response we need
type githubRelease struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	UpdatedAt          string `json:"updated_at"`
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	URL                string `json:"url"`                  // API download URL — works for private repos
	BrowserDownloadURL string `json:"browser_download_url"` // Direct download URL — for reference
}

type GitHubClient struct {
	token   string
	apiBase string // defaults to defaultAPIBase, overridable in tests
	client  *http.Client
	bucket  *githubBucket // rate-limit and cache state shared per token
	log     *Logger
}

// NewGitHubClient creates a GitHub client with optional token authentication.
// Rate-limit and cache state is shared with every other client using the same token.
func NewGitHubClient(token string, log *Logger) *GitHubClient {
	return defaultGitHubPool.Client(token, log)
}

func newGitHubClient(token string, log *Logger, bucket *githubBucket) *GitHubClient {
	return &GitHubClient{
		token:   token,
		apiBase: defaultAPIBase,
		client: &http.Client{
			Timeout: 90 * time.Second,
			Transport: cachingTransport{
				base:   rateLimitTransport{base: tracedTransport{log: log}, limiter: bucket.limiter},
				bucket: bucket,
			},
		},
		bucket: bucket,
		log:    log,
	}
}

// quotaShortfall reports when this token cannot afford n more requests right now.
func (g *GitHubClient) quotaShortfall(n int) *RateLimitError {
	if g.bucket == nil {
		return nil
	}
	return g.bucket.limiter.shortfall(n)
}

// now reads the limiter clock so rate-limit waits and blocks agree in tests.
func (g *GitHubClient) now() time.Time {
	if g.bucket == nil {
		return time.Now()
	}
	return g.bucket.limiter.now()
}
