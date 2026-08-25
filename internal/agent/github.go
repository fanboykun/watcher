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
}

type ServiceMeta struct {
	Version            string `json:"version"`
	Artifact           string `json:"artifact"`
	ArtifactURL        string `json:"artifact_url"`
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
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	URL                string `json:"url"`                  // API download URL — works for private repos
	BrowserDownloadURL string `json:"browser_download_url"` // Direct download URL — for reference
}

type GitHubClient struct {
	token   string
	apiBase string // defaults to defaultAPIBase, overridable in tests
	client  *http.Client
	log     *Logger
}

// NewGitHubClient creates a GitHub client with optional token authentication.
func NewGitHubClient(token string, log *Logger) *GitHubClient {
	return &GitHubClient{
		token:   token,
		apiBase: defaultAPIBase,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
		log: log,
	}
}
