package agent

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"github.com/fanboykun/watcher/internal/database"
)

// deploy downloads one target artifact, invokes the deployer, and reports its GitHub status.
func (r *RepoWatcher) deploy(ctx context.Context, gh *GitHubClient, svcMeta ServiceMeta, targetVersion, previousVersion string) error {
	deployLogID, _ := r.state.SetDeploying(targetVersion, previousVersion)

	// ── GitHub Deployment API integration (optional) ──────────────
	var ghDeploymentID int64
	var ghOwner, ghRepo string
	resolvedEnv := resolveDeploymentEnvironment(r.wcfg.DeploymentEnvironment, r.global.Environment)
	useGHDeploy := r.global.GitHubDeployEnabled && strings.TrimSpace(r.resolveGitHubToken()) != ""
	logURL := buildDeployLogURL(r.global.APIBaseURL, r.watcherID, deployLogID)
	r.state.AppendDeployLog(fmt.Sprintf("github_deployment: environment=%q", resolvedEnv))
	if !r.global.GitHubDeployEnabled {
		r.state.AppendDeployLog("github_deployment: disabled by GITHUB_DEPLOY_ENABLED=false")
	} else if !useGHDeploy {
		r.state.AppendDeployLog("github_deployment: disabled because GitHub token is empty (watcher + global)")
	}
	if useGHDeploy && resolvedEnv == "" {
		r.state.AppendDeployLog("github_deployment: disabled because deployment environment is empty (watcher + global ENVIRONMENT)")
		useGHDeploy = false
	}

	if useGHDeploy {
		var err error
		ghOwner, ghRepo, err = resolveDeploymentRepo(r.wcfg.MetadataURL, svcMeta.ArtifactURL)
		if err != nil {
			r.state.AppendDeployLog("github_deployment: parse repo failed: " + err.Error())
			r.log.Warn("cannot parse repo for GitHub Deployment API", "error", err)
			useGHDeploy = false
		}
	}

	if useGHDeploy {
		if logURL == "" && strings.TrimSpace(r.global.APIBaseURL) != "" {
			r.state.AppendDeployLog("github_deployment: invalid API_BASE_URL, skipping log_url in deployment statuses")
		}
		desc := fmt.Sprintf("Deploying %s %s", r.wcfg.ServiceName, targetVersion)
		deployRef := resolveDeploymentRef(targetVersion, svcMeta.ArtifactURL)
		r.state.AppendDeployLog(fmt.Sprintf("github_deployment: creating deployment repo=%s/%s ref=%s env=%q", ghOwner, ghRepo, deployRef, resolvedEnv))
		id, err := gh.CreateDeployment(ctx, ghOwner, ghRepo, deployRef, resolvedEnv, desc)
		if err != nil {
			r.state.AppendDeployLog("github_deployment: create deployment failed: " + err.Error())
			r.log.Warn("failed to create GitHub deployment", "error", err)
			useGHDeploy = false
		} else {
			ghDeploymentID = id
			_ = r.state.SetGitHubDeploymentID(deployLogID, ghDeploymentID)
			r.state.AppendDeployLog(fmt.Sprintf("github_deployment: created deployment id=%d", ghDeploymentID))
			r.state.AppendDeployLog("github_deployment: setting status=in_progress")
			if err := gh.UpdateDeploymentStatus(ctx, ghOwner, ghRepo, ghDeploymentID, "in_progress", logURL, desc); err != nil {
				r.state.AppendDeployLog("github_deployment: set status=in_progress failed: " + err.Error())
			}
		}
	}

	// ── Actual deploy ─────────────────────────────────────────────
	if err := os.MkdirAll(r.wcfg.InstallDir, 0755); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("create install dir: %w", err)
	}

	if svcMeta.ArtifactURL == "" {
		err := fmt.Errorf("artifact_url missing in version.json for %s", targetVersion)
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return err
	}

	downloadsDir := filepath.Join(r.wcfg.InstallDir, "downloads")
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("create downloads dir: %w", err)
	}

	zipPath := filepath.Join(downloadsDir, releaseStorageName(targetVersion)+".zip")
	if err := gh.DownloadArtifact(ctx, svcMeta.ArtifactURL, zipPath, r.wcfg.DownloadRetries); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("download artifact: %w", err)
	}
	defer func() {
		r.log.Debug("removing zip", "path", zipPath)
		os.Remove(zipPath)
	}()

	for i := range r.wcfg.Services {
		svc := &r.wcfg.Services[i]
		
		// Find service ID from DB
		var dbSvc database.Service
		if err := r.db.Where("watcher_id = ? AND windows_service_name = ?", r.watcherID, svc.WindowsServiceName).First(&dbSvc).Error; err == nil {
			
			var revision database.ServiceConfigRevision
			// Try exact match first, then fallback to "next"
			err := r.db.Where("service_id = ? AND target_version = ?", dbSvc.ID, targetVersion).First(&revision).Error
			if err != nil {
				err = r.db.Where("service_id = ? AND target_version = ?", dbSvc.ID, "next").First(&revision).Error
			}
			
			if err == nil {
				r.state.AppendDeployLog(fmt.Sprintf("applying configuration candidate for version: %s", revision.TargetVersion))
				svc.EnvContent = revision.EnvContent
				
				// Update active service config
				r.db.Model(&dbSvc).Update("env_content", svc.EnvContent)
				
				// Delete the consumed revision (especially important for "next")
				r.db.Delete(&revision)
				
				// Write the file to disk so deployer snapshot catches it
				if svc.EnvFile != "" {
					targetPath := filepath.Join(r.wcfg.InstallDir, svc.EnvFile)
					os.MkdirAll(filepath.Dir(targetPath), 0755)
					os.WriteFile(targetPath, []byte(svc.EnvContent), 0600)
				}
			}
		}
	}

	if err := r.deployer.Deploy(ctx, targetVersion, zipPath, previousVersion); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return err
	}

	if err := r.state.WriteVersion(targetVersion); err != nil {
		r.log.Warn("failed to write version", "error", err)
	}
	if err := r.state.SetHealthy(targetVersion); err != nil {
		r.log.Warn("failed to update state", "error", err)
	}
	if owner, repo, err := resolveDeploymentRepo(r.wcfg.MetadataURL, svcMeta.ArtifactURL); err == nil {
		metadata := ReleaseDeploymentMetadata{
			Version:     targetVersion,
			Owner:       owner,
			Repository:  repo,
			Ref:         resolveDeploymentRef(targetVersion, svcMeta.ArtifactURL),
			ArtifactURL: svcMeta.ArtifactURL,
		}
		if err := WriteReleaseDeploymentMetadata(r.wcfg.InstallDir, metadata); err != nil {
			r.log.Warn("failed to persist release deployment metadata", "version", targetVersion, "error", err)
		}
	}

	// ── GitHub Deployment: success ────────────────────────────────
	if useGHDeploy {
		r.state.AppendDeployLog("github_deployment: setting status=success")
		if err := gh.UpdateDeploymentStatus(ctx, ghOwner, ghRepo, ghDeploymentID, "success", logURL,
			fmt.Sprintf("Deployed %s %s", r.wcfg.ServiceName, targetVersion)); err != nil {
			r.state.AppendDeployLog("github_deployment: set status=success failed: " + err.Error())
		}
	}

	// ── Clean old releases ────────────────────────────────────────
	if err := CleanOldReleases(r.wcfg.InstallDir, r.wcfg.MaxKeptVersions); err != nil {
		r.log.Warn("failed to clean old releases", "error", err)
	}

	r.log.Info("deploy complete", "version", targetVersion)
	return nil
}

// ghDeployFailure updates the GitHub deployment status to "failure" if integration is active.
func (r *RepoWatcher) ghDeployFailure(ctx context.Context, gh *GitHubClient, active bool, owner, repo string, deploymentID int64, deployLogID uint, errMsg string) {
	if !active {
		return
	}
	logURL := buildDeployLogURL(r.global.APIBaseURL, r.watcherID, deployLogID)
	r.state.AppendDeployLog("github_deployment: setting status=failure")
	if err := gh.UpdateDeploymentStatus(ctx, owner, repo, deploymentID, "failure", logURL, errMsg); err != nil {
		r.state.AppendDeployLog("github_deployment: set status=failure failed: " + err.Error())
	}
}

// resolveGitHubToken returns the watcher token override or the global fallback.
func (r *RepoWatcher) resolveGitHubToken() string {
	if t := strings.TrimSpace(r.wcfg.GitHubToken); t != "" {
		return t
	}
	return strings.TrimSpace(r.global.GitHubToken)
}

// keys returns the keys from a map for diagnostic messages.
func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// resolveDeploymentRepo resolves the GitHub repository from metadata or artifact URLs.
func resolveDeploymentRepo(metadataURL, artifactURL string) (owner, repo string, err error) {
	if artifactURL != "" {
		aOwner, aRepo, _, _, aErr := parseReleaseDownloadURL(artifactURL)
		if aErr == nil {
			return aOwner, aRepo, nil
		}
	}
	return ParseGitHubURL(metadataURL)
}

// resolveDeploymentRef resolves the release ref reported to the GitHub Deployment API.
func resolveDeploymentRef(targetVersion, artifactURL string) string {
	if artifactURL != "" {
		_, _, tag, _, err := parseReleaseDownloadURL(artifactURL)
		if err == nil && strings.TrimSpace(tag) != "" {
			return tag
		}
	}
	return targetVersion
}

// resolveDeploymentEnvironment returns the watcher environment override or global fallback.
func resolveDeploymentEnvironment(watcherEnv, globalEnv string) string {
	if env := strings.TrimSpace(watcherEnv); env != "" {
		return env
	}
	return strings.TrimSpace(globalEnv)
}

// buildDeployLogURL builds a public deployment-log URL when the API base URL is valid.
func buildDeployLogURL(apiBaseURL string, watcherID, deployLogID uint) string {
	base := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if strings.EqualFold(u.Path, "/api") {
		u.Path = ""
		base = strings.TrimRight(u.String(), "/")
	}
	return fmt.Sprintf("%s/watchers/%d/logs/%d", base, watcherID, deployLogID)
}
