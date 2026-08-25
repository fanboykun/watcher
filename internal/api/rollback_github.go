package api

import (
	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func resolveRollbackDeploymentTarget(watcher *database.Watcher, version string) (owner, repo, ref, source string, err error) {
	if metadata, metadataErr := agent.ReadReleaseDeploymentMetadata(watcher.InstallDir, version); metadataErr == nil {
		return metadata.Owner, metadata.Repository, metadata.Ref, "release_metadata", nil
	}
	owner, repo, err = agent.ParseGitHubURL(watcher.MetadataURL)
	if err != nil {
		return "", "", "", "", err
	}
	return owner, repo, version, "version_fallback", nil
}
