package api

import (
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func TestResolveRollbackDeploymentTargetUsesRecordedReleaseRef(t *testing.T) {
	watcher := &database.Watcher{
		InstallDir:  t.TempDir(),
		MetadataURL: "https://github.com/fallback/repository",
	}
	metadata := agent.ReleaseDeploymentMetadata{
		Version:    "0.3.6",
		Owner:      "actual",
		Repository: "simple-go-server",
		Ref:        "v0.3.6",
	}
	if err := agent.WriteReleaseDeploymentMetadata(watcher.InstallDir, metadata); err != nil {
		t.Fatal(err)
	}

	owner, repo, ref, source, err := resolveRollbackDeploymentTarget(watcher, "0.3.6")
	if err != nil {
		t.Fatal(err)
	}
	if owner != "actual" || repo != "simple-go-server" || ref != "v0.3.6" || source != "release_metadata" {
		t.Fatalf("target = %s/%s ref=%s source=%s", owner, repo, ref, source)
	}
}
