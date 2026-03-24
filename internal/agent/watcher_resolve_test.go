package agent

import (
	"testing"

	"github.com/fanboykun/watcher/internal/config"
)

func TestResolveDeploymentEnvironment(t *testing.T) {
	tests := []struct {
		name       string
		watcherEnv string
		globalEnv  string
		want       string
	}{
		{name: "watcher takes precedence", watcherEnv: "staging", globalEnv: "production", want: "staging"},
		{name: "fallback to global", watcherEnv: "", globalEnv: "production", want: "production"},
		{name: "trim watcher value", watcherEnv: "  qa  ", globalEnv: "production", want: "qa"},
		{name: "both empty", watcherEnv: " ", globalEnv: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDeploymentEnvironment(tt.watcherEnv, tt.globalEnv)
			if got != tt.want {
				t.Fatalf("resolveDeploymentEnvironment(%q, %q) = %q, want %q", tt.watcherEnv, tt.globalEnv, got, tt.want)
			}
		})
	}
}

func TestResolveGitHubToken(t *testing.T) {
	rw := &RepoWatcher{
		wcfg:   &WatcherConfig{GitHubToken: " watcher-token "},
		global: &config.AppConfig{GitHubToken: "global-token"},
	}
	if got := rw.resolveGitHubToken(); got != "watcher-token" {
		t.Fatalf("resolveGitHubToken() = %q, want watcher token", got)
	}

	rw.wcfg.GitHubToken = "   "
	if got := rw.resolveGitHubToken(); got != "global-token" {
		t.Fatalf("resolveGitHubToken() fallback = %q, want global-token", got)
	}
}
