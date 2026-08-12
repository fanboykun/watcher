package agent

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var runCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type Deployer struct {
	wcfg           *WatcherConfig
	nssmPath       string
	serviceManager ServiceManager
	log            *Logger
	logFn          func(string)
}

func NewDeployer(wcfg *WatcherConfig, nssmPath string, log *Logger, logFn func(string)) *Deployer {
	return &Deployer{
		wcfg:           wcfg,
		nssmPath:       nssmPath,
		serviceManager: NewNSSMServiceManager(nssmPath),
		log:            log,
		logFn:          logFn,
	}
}

func (d *Deployer) l(msg string, args ...any) {
	d.log.Info(msg, args...)
	if d.logFn != nil {
		tz := time.Now().UTC().Format("15:04:05")
		text := fmt.Sprintf("[%s] %s", tz, msg)
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				text += fmt.Sprintf(" %v=%v", args[i], args[i+1])
			}
		}
		d.logFn(text)
	}
}

func (d *Deployer) lWarn(msg string, args ...any) {
	d.log.Warn(msg, args...)
	if d.logFn != nil {
		tz := time.Now().UTC().Format("15:04:05")
		text := fmt.Sprintf("[%s] WARN: %s", tz, msg)
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				text += fmt.Sprintf(" %v=%v", args[i], args[i+1])
			}
		}
		d.logFn(text)
	}
}

func releaseStorageName(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	return url.PathEscape(version)
}

func restoreReleaseVersion(storage string) string {
	storage = strings.TrimSpace(storage)
	if storage == "" {
		return storage
	}
	restored, err := url.PathUnescape(storage)
	if err != nil {
		return storage
	}
	return restored
}

func currentVersionFromCurrentDir(installDir string) (string, error) {
	currentDir := filepath.Join(installDir, "current")
	releasesDir := filepath.Join(installDir, "releases")

	target, err := os.Readlink(currentDir)
	if err != nil || strings.TrimSpace(target) == "" {
		target, err = filepath.EvalSymlinks(currentDir)
		if err != nil {
			return "", nil
		}
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(currentDir), target)
	}
	target = filepath.Clean(target)

	rel, err := filepath.Rel(releasesDir, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", nil
	}

	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", nil
	}

	return restoreReleaseVersion(parts[0]), nil
}

type releasePromotion struct {
	releaseDir string
	backupDir  string
}

func (p *releasePromotion) restore() error {
	if p == nil {
		return nil
	}
	if err := removePath(p.releaseDir); err != nil {
		return fmt.Errorf("remove promoted release: %w", err)
	}
	if p.backupDir == "" {
		return nil
	}
	if err := os.Rename(p.backupDir, p.releaseDir); err != nil {
		return fmt.Errorf("restore release backup: %w", err)
	}
	return nil
}

func (p *releasePromotion) cleanup() error {
	if p == nil || p.backupDir == "" {
		return nil
	}
	return removePath(p.backupDir)
}

func (d *Deployer) prepareRelease(stagedDir string) error {
	if err := d.validateReleaseArtifacts(stagedDir); err != nil {
		return err
	}
	if err := d.writeReleaseConfigFiles(stagedDir); err != nil {
		return err
	}
	if err := d.captureConfigSnapshot(stagedDir); err != nil {
		return fmt.Errorf("capture config snapshot: %w", err)
	}
	return nil
}

func (d *Deployer) validateReleaseArtifacts(releaseDir string) error {
	for _, svc := range d.wcfg.Services {
		if svc.ServiceType == "iis" || svc.ServiceType == "static" {
			continue
		}

		binaryName := strings.TrimSpace(svc.BinaryName)
		if binaryName == "" {
			return fmt.Errorf("binary_name is empty for service %s", svc.WindowsServiceName)
		}
		binaryPath := filepath.Join(releaseDir, binaryName)
		rel, err := filepath.Rel(releaseDir, binaryPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("binary %q for service %s escapes the release directory", binaryName, svc.WindowsServiceName)
		}
		info, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("inspect binary %q for service %s: %w", binaryName, svc.WindowsServiceName, err)
		}
		if info.IsDir() {
			return fmt.Errorf("binary %q for service %s is a directory", binaryName, svc.WindowsServiceName)
		}
	}
	return nil
}

func (d *Deployer) promoteRelease(stagedDir, releaseDir string) (*releasePromotion, error) {
	promotion := &releasePromotion{releaseDir: releaseDir}
	if _, err := os.Lstat(releaseDir); err == nil {
		promotion.backupDir = filepath.Join(
			d.wcfg.InstallDir,
			fmt.Sprintf(".watcher-release-backup-%d", time.Now().UnixNano()),
		)
		if err := os.Rename(releaseDir, promotion.backupDir); err != nil {
			return nil, fmt.Errorf("backup existing release %s: %w", releaseDir, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect existing release %s: %w", releaseDir, err)
	}

	if err := os.Rename(stagedDir, releaseDir); err == nil {
		return promotion, nil
	} else {
		d.lWarn("release rename failed, falling back to copy", "error", err)
		if cleanupErr := removePath(releaseDir); cleanupErr != nil {
			restoreErr := promotion.restore()
			failure := errors.Join(fmt.Errorf("prepare release copy target: %w", cleanupErr), fmt.Errorf("rename staged release: %w", err))
			if restoreErr != nil {
				failure = errors.Join(failure, restoreErr)
			}
			return nil, failure
		}
		if copyErr := copyDir(stagedDir, releaseDir); copyErr != nil {
			restoreErr := promotion.restore()
			failure := errors.Join(fmt.Errorf("rename staged release: %w", err), fmt.Errorf("copy staged release: %w", copyErr))
			if restoreErr != nil {
				failure = errors.Join(failure, restoreErr)
			}
			return nil, failure
		}
	}
	return promotion, nil
}

func (d *Deployer) cleanupPromotion(promotion *releasePromotion) {
	if err := promotion.cleanup(); err != nil {
		d.lWarn("failed to remove release backup", "path", promotion.backupDir, "error", err)
	}
}

func (d *Deployer) Deploy(ctx context.Context, version, zipPath, previousVersion string) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", releaseStorageName(version))
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")
	rollbackVersion := d.resolveRollbackVersion(version, previousVersion)

	d.l("deploying", "version", version, "release_dir", releaseDir)

	// Extract to a temporary directory first to avoid file-in-use errors during redeploys
	tempReleaseDir := releaseDir + fmt.Sprintf("-%d", time.Now().UnixNano())

	if err := d.extractZip(zipPath, tempReleaseDir); err != nil {
		os.RemoveAll(tempReleaseDir)
		return fmt.Errorf("extract zip: %w", err)
	}
	defer os.RemoveAll(tempReleaseDir)

	if err := d.prepareRelease(tempReleaseDir); err != nil {
		return fmt.Errorf("prepare release: %w", err)
	}

	d.l("stopping services")
	stoppedServices, err := d.stopServices(ctx, "deploy")
	if err != nil {
		return err
	}

	promotion, err := d.promoteRelease(tempReleaseDir, releaseDir)
	if err != nil {
		return d.recoverStoppedServices(ctx, stoppedServices, fmt.Errorf("promote release: %w", err))
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		restoreErr := promotion.restore()
		failure := fmt.Errorf("swap current: %w", err)
		if restoreErr != nil {
			failure = errors.Join(failure, fmt.Errorf("restore previous release: %w", restoreErr))
		}
		return d.recoverStoppedServices(ctx, stoppedServices, failure)
	}

	d.l("starting services")
	if err := d.startServices(ctx, currentDir, "deploy"); err != nil {
		return d.recoverActivatedDeployment(ctx, version, currentDir, rollbackVersion, err, promotion)
	}

	if d.wcfg.HealthCheck.Enabled {
		for _, svc := range d.wcfg.Services {
			url := svc.HealthCheckURL
			if url == "" {
				url = d.wcfg.HealthCheck.URL
			}
			if url == "" {
				continue
			}
			if err := d.healthCheck(ctx, svc.WindowsServiceName, url); err != nil {
				return d.recoverActivatedDeployment(ctx, version, currentDir, rollbackVersion,
					fmt.Errorf("health check failed for %s: %w", svc.WindowsServiceName, err), promotion)
			}
		}
	}

	d.cleanupPromotion(promotion)
	d.l("deploy successful", "version", version)
	return nil
}

func (d *Deployer) resolveRollbackVersion(targetVersion, previousVersion string) string {
	previousVersion = strings.TrimSpace(previousVersion)
	if previousVersion != "" {
		releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", releaseStorageName(previousVersion))
		if _, err := os.Stat(releaseDir); err == nil {
			return previousVersion
		}
		d.lWarn("configured previous version is unavailable on disk", "version", previousVersion, "path", releaseDir)
	}

	if currentVersion, err := currentVersionFromCurrentDir(d.wcfg.InstallDir); err == nil && currentVersion != "" && currentVersion != targetVersion {
		d.l("resolved rollback version from current dir", "version", currentVersion)
		return currentVersion
	}

	versions, err := ListAvailableVersions(d.wcfg.InstallDir)
	if err != nil {
		d.lWarn("failed to list releases while resolving rollback version", "error", err)
		return ""
	}
	for _, version := range versions {
		if strings.TrimSpace(version.Version) == "" || version.Version == targetVersion {
			continue
		}
		d.l("resolved rollback version from releases dir", "version", version.Version)
		return version.Version
	}

	return ""
}

func (d *Deployer) Rollback(ctx context.Context, version string) error {
	return d.rollback(ctx, version, true)
}

func (d *Deployer) rollback(ctx context.Context, version string, restoreOriginalOnFailure bool) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", releaseStorageName(version))
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")
	originalVersion := ""
	originalReleaseDir := ""
	if restoreOriginalOnFailure {
		if currentVersion, err := currentVersionFromCurrentDir(d.wcfg.InstallDir); err == nil && currentVersion != "" && currentVersion != version {
			candidate := filepath.Join(d.wcfg.InstallDir, "releases", releaseStorageName(currentVersion))
			if _, statErr := os.Stat(candidate); statErr == nil {
				originalVersion = currentVersion
				originalReleaseDir = candidate
			}
		}
	}

	d.lWarn("rolling back", "to_version", version)

	if _, err := os.Stat(releaseDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("rollback target %s not on disk", releaseDir)
		}
		return fmt.Errorf("inspect rollback target %s: %w", releaseDir, err)
	}
	if err := d.validateReleaseArtifacts(releaseDir); err != nil {
		return fmt.Errorf("validate rollback target: %w", err)
	}

	stoppedServices, err := d.stopServices(ctx, "rollback")
	if err != nil {
		return err
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return d.recoverStoppedServices(ctx, stoppedServices, fmt.Errorf("swap during rollback: %w", err))
	}

	// Restore config snapshot before re-registering services
	if err := RestoreConfigSnapshot(releaseDir, d.wcfg.InstallDir, currentDir); err != nil {
		d.lWarn("snapshot restoration failed, proceeding without config restore", "error", err)
	}

	if err := d.startServices(ctx, currentDir, "rollback"); err != nil {
		return d.recoverManualRollback(ctx, version, originalVersion, originalReleaseDir, currentDir, err)
	}

	if d.wcfg.HealthCheck.Enabled {
		for _, svc := range d.wcfg.Services {
			url := svc.HealthCheckURL
			if url == "" {
				url = d.wcfg.HealthCheck.URL
			}
			if url == "" {
				continue
			}
			if err := d.healthCheck(ctx, svc.WindowsServiceName, url); err != nil {
				return d.recoverManualRollback(ctx, version, originalVersion, originalReleaseDir, currentDir,
					fmt.Errorf("health check failed after rollback for %s: %w", svc.WindowsServiceName, err))
			}
		}
	}

	d.l("rollback successful", "version", version)
	return nil
}

func (d *Deployer) recoverManualRollback(
	ctx context.Context,
	targetVersion string,
	originalVersion string,
	originalReleaseDir string,
	currentDir string,
	originalErr error,
) error {
	if originalReleaseDir == "" {
		return originalErr
	}

	d.lWarn("rollback target failed; restoring original release", "target", targetVersion, "original", originalVersion, "reason", originalErr)
	recoveryCtx := context.WithoutCancel(ctx)
	stoppedServices, err := d.stopServices(recoveryCtx, "manual rollback recovery")
	if err != nil {
		return errors.Join(originalErr, fmt.Errorf("restore original release: %w", err))
	}
	if err := d.swapCurrent(originalReleaseDir, currentDir); err != nil {
		return d.recoverStoppedServices(recoveryCtx, stoppedServices,
			errors.Join(originalErr, fmt.Errorf("restore original current: %w", err)))
	}
	if err := RestoreConfigSnapshot(originalReleaseDir, d.wcfg.InstallDir, currentDir); err != nil {
		d.lWarn("original config snapshot restoration failed", "version", originalVersion, "error", err)
	}
	if err := d.startServices(recoveryCtx, currentDir, "manual rollback recovery"); err != nil {
		return errors.Join(originalErr, fmt.Errorf("restart original release %s: %w", originalVersion, err))
	}
	return fmt.Errorf("rollback to %s failed, restored %s: %w", targetVersion, originalVersion, originalErr)
}

func (d *Deployer) tryRollbackWithResult(ctx context.Context, previousVersion string, originalErr error) (bool, error) {
	if previousVersion == "" {
		return false, fmt.Errorf("%w (no previous version to roll back to)", originalErr)
	}
	d.lWarn("attempting rollback", "to", previousVersion, "reason", originalErr)
	if rbErr := d.rollback(ctx, previousVersion, false); rbErr != nil {
		return false, fmt.Errorf("deploy failed AND rollback failed: deploy=%w rollback=%v", originalErr, rbErr)
	}
	return true, fmt.Errorf("deploy failed, rolled back to %s: %w", previousVersion, originalErr)
}

func (d *Deployer) recoverActivatedDeployment(
	ctx context.Context,
	version string,
	currentDir string,
	rollbackVersion string,
	originalErr error,
	promotion *releasePromotion,
) error {
	// Once activation has started, request cancellation must not prevent the
	// bounded compensation path from restoring a runnable release.
	recoveryCtx := context.WithoutCancel(ctx)
	rolledBack, rollbackErr := d.tryRollbackWithResult(recoveryCtx, rollbackVersion, originalErr)
	if rolledBack {
		d.cleanupPromotion(promotion)
		return rollbackErr
	}
	if promotion == nil || promotion.backupDir == "" {
		return rollbackErr
	}

	d.lWarn("standard rollback unavailable; restoring release backup", "path", promotion.backupDir, "reason", rollbackErr)
	if backupErr := d.restorePromotionAndRestart(recoveryCtx, currentDir, promotion); backupErr != nil {
		combinedRollbackErr := errors.Join(rollbackErr, fmt.Errorf("release backup recovery failed: %w", backupErr))
		return fmt.Errorf("deploy failed AND rollback failed: deploy=%w rollback=%v", originalErr, combinedRollbackErr)
	}
	d.cleanupPromotion(promotion)
	return fmt.Errorf("deploy failed, rolled back to %s: %w", version, originalErr)
}

func (d *Deployer) restorePromotionAndRestart(ctx context.Context, currentDir string, promotion *releasePromotion) error {
	recoveryCtx := context.WithoutCancel(ctx)
	stoppedServices, err := d.stopServices(recoveryCtx, "release backup recovery")
	if err != nil {
		return err
	}
	if err := promotion.restore(); err != nil {
		return d.recoverStoppedServices(recoveryCtx, stoppedServices, err)
	}
	if err := d.swapCurrent(promotion.releaseDir, currentDir); err != nil {
		return d.recoverStoppedServices(recoveryCtx, stoppedServices, fmt.Errorf("reactivate restored release: %w", err))
	}
	if err := d.startServices(recoveryCtx, currentDir, "release backup recovery"); err != nil {
		return err
	}
	return nil
}

func (d *Deployer) extractZip(zipPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if err := extractZipFile(f, destDir); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

func extractZipFile(f *zip.File, destDir string) error {
	destPath := filepath.Join(destDir, filepath.Clean("/" + f.Name)[1:])
	if destPath == destDir {
		return nil
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(destPath, f.Mode())
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	src, err := f.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (d *Deployer) writeReleaseConfigFiles(currentDir string) error {
	for _, svc := range d.wcfg.Services {
		for _, file := range svc.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" || normalizeConfigFileTarget(file.Target) != "release_dir" {
				continue
			}
			if err := writeManagedFile(currentDir, file.FilePath, file.Content); err != nil {
				return fmt.Errorf("write release config %s for %s: %w", file.FilePath, svc.WindowsServiceName, err)
			}
		}
	}
	return nil
}

func normalizeConfigFileTarget(target string) string {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "", "app", "app_dir", "install_dir":
		return "app_dir"
	case "release", "release_dir", "current":
		return "release_dir"
	default:
		return "app_dir"
	}
}

func writeManagedFile(rootDir, relativePath, content string) error {
	targetPath, err := managedFilePath(rootDir, relativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, []byte(content), 0600)
}

func managedFilePath(rootDir, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", errors.New("managed file path is empty")
	}
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve managed file root: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(root, relativePath))
	if err != nil {
		return "", fmt.Errorf("resolve managed file path %q: %w", relativePath, err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("managed file path %q escapes root %s", relativePath, rootDir)
	}
	return target, nil
}

const snapshotDir = ".watcher-snapshot"

// CaptureConfigSnapshot writes a mirrored copy of all managed config for the
// given watcher config into releaseDir/.watcher-snapshot/. Layout:
//
//	.watcher-snapshot/
//	  env/<windows_service_name>/<env_file>
//	  app/<windows_service_name>/<file_path>
//	  release/<windows_service_name>/<file_path>
func CaptureConfigSnapshot(wcfg *WatcherConfig, releaseDir string) error {
	snapRoot := filepath.Join(releaseDir, snapshotDir)
	// Remove any pre-existing snapshot (e.g. from a redeploy of the same version)
	if err := os.RemoveAll(snapRoot); err != nil {
		return fmt.Errorf("remove old snapshot: %w", err)
	}

	for _, svc := range wcfg.Services {
		name := svc.WindowsServiceName

		// env file
		if strings.TrimSpace(svc.EnvFile) != "" && strings.TrimSpace(svc.EnvContent) != "" {
			if err := writeManagedFile(filepath.Join(snapRoot, "env", name), svc.EnvFile, svc.EnvContent); err != nil {
				return fmt.Errorf("snapshot env %s for %s: %w", svc.EnvFile, name, err)
			}
		}

		// config files
		for _, file := range svc.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" || strings.TrimSpace(file.Content) == "" {
				continue
			}
			target := normalizeConfigFileTarget(file.Target)
			var category string
			switch target {
			case "app_dir":
				category = "app"
			case "release_dir":
				category = "release"
			default:
				category = "app"
			}
			if err := writeManagedFile(filepath.Join(snapRoot, category, name), file.FilePath, file.Content); err != nil {
				return fmt.Errorf("snapshot %s config %s for %s: %w", category, file.FilePath, name, err)
			}
		}
	}
	return nil
}

// HasConfigSnapshot returns true when a .watcher-snapshot directory exists
// inside the given release directory.
func HasConfigSnapshot(releaseDir string) bool {
	info, err := os.Stat(filepath.Join(releaseDir, snapshotDir))
	return err == nil && info.IsDir()
}

// BackfillConfigSnapshots checks all release dirs for a watcher and writes a
// snapshot from the current WatcherConfig for any that are missing one.
// Errors are logged but do not halt the backfill.
func BackfillConfigSnapshots(wcfg *WatcherConfig, log *Logger) {
	versions, err := ListAvailableVersions(wcfg.InstallDir)
	if err != nil {
		log.Warn("backfill: failed to list versions", "watcher", wcfg.Name, "error", err)
		return
	}
	for _, v := range versions {
		if HasConfigSnapshot(v.Path) {
			continue
		}
		if err := CaptureConfigSnapshot(wcfg, v.Path); err != nil {
			log.Warn("backfill: failed to write snapshot", "watcher", wcfg.Name, "version", v.Version, "error", err)
			continue
		}
		log.Info("backfilled config snapshot (approximation from current DB config)", "watcher", wcfg.Name, "version", v.Version)
	}
}

func (d *Deployer) captureConfigSnapshot(releaseDir string) error {
	return CaptureConfigSnapshot(d.wcfg, releaseDir)
}

// RestoreConfigSnapshot reads the config snapshot from releaseDir/.watcher-snapshot/
// and writes each file to its actual target path on disk.
// Mapping:
//
//	env/<svc>/<file>     → installDir/<file>
//	app/<svc>/<file>     → installDir/<file>
//	release/<svc>/<file> → currentDir/<file>
//
// Returns nil if no snapshot directory exists (caller should log a warning).
func RestoreConfigSnapshot(releaseDir, installDir, currentDir string) error {
	snapRoot := filepath.Join(releaseDir, snapshotDir)
	if _, err := os.Stat(snapRoot); os.IsNotExist(err) {
		return nil // no snapshot — degrade gracefully
	}

	type categoryMapping struct {
		subdir    string
		targetDir string
	}
	categories := []categoryMapping{
		{"env", installDir},
		{"app", installDir},
		{"release", currentDir},
	}

	for _, cat := range categories {
		catDir := filepath.Join(snapRoot, cat.subdir)
		svcEntries, err := os.ReadDir(catDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read snapshot %s: %w", cat.subdir, err)
		}
		for _, svcEntry := range svcEntries {
			if !svcEntry.IsDir() {
				continue
			}
			svcDir := filepath.Join(catDir, svcEntry.Name())
			if err := restoreSnapshotTree(svcDir, cat.targetDir); err != nil {
				return fmt.Errorf("restore %s/%s: %w", cat.subdir, svcEntry.Name(), err)
			}
		}
	}
	return nil
}

// restoreSnapshotTree walks a snapshot service subdirectory and writes each
// file to the target directory, preserving relative paths.
func restoreSnapshotTree(srcDir, targetDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read snapshot file %s: %w", relPath, err)
		}
		return writeManagedFile(targetDir, relPath, string(content))
	})
}

func (d *Deployer) swapCurrent(releaseDir, currentDir string) error {
	suffix := time.Now().UnixNano()
	candidateDir := fmt.Sprintf("%s.next-%d", currentDir, suffix)
	previousDir := fmt.Sprintf("%s.previous-%d", currentDir, suffix)
	defer removePath(candidateDir)

	out, err := runCommand("cmd", "/C", "mklink", "/J", candidateDir, releaseDir)
	if err != nil {
		d.lWarn("mklink /J failed, falling back to copy", "output", string(out))
		if err := removePath(candidateDir); err != nil {
			return fmt.Errorf("remove failed current candidate: %w", err)
		}
		if err := copyDir(releaseDir, candidateDir); err != nil {
			return fmt.Errorf("prepare current candidate: %w", err)
		}
	}

	hadCurrent := false
	if _, err := os.Lstat(currentDir); err == nil {
		hadCurrent = true
		if err := os.Rename(currentDir, previousDir); err != nil {
			return fmt.Errorf("preserve old current: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect current path: %w", err)
	}

	if err := os.Rename(candidateDir, currentDir); err != nil {
		failure := fmt.Errorf("activate current candidate: %w", err)
		if hadCurrent {
			if restoreErr := os.Rename(previousDir, currentDir); restoreErr != nil {
				failure = errors.Join(failure, fmt.Errorf("restore old current: %w", restoreErr))
			}
		}
		return failure
	}

	if hadCurrent {
		if err := removePath(previousDir); err != nil {
			d.lWarn("failed to remove previous current path", "path", previousDir, "error", err)
		}
	}
	return nil
}

func removePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Remove(path); err == nil {
		return nil
	}
	return os.RemoveAll(path)
}

// ensureServiceByType dispatches to the correct ensure logic based on ServiceType.
func (d *Deployer) ensureServiceByType(svc ServiceConfig, currentDir string) error {
	switch svc.ServiceType {
	case "iis", "static":
		return d.ensureIISService(svc, currentDir)
	default: // "nssm"
		if svc.BinaryName == "" {
			return fmt.Errorf("binary_name is empty for service %s — cannot register with NSSM", svc.WindowsServiceName)
		}
		newBin := filepath.Join(currentDir, svc.BinaryName)
		if _, err := os.Stat(newBin); os.IsNotExist(err) {
			// List what's actually in the directory to help debug
			entries, _ := os.ReadDir(currentDir)
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			return fmt.Errorf("binary %q not found in %s (available: %v)", svc.BinaryName, currentDir, names)
		}
		return d.ensureService(svc, newBin)
	}
}

// ensureService registers the service with NSSM if it does not exist yet,
// or updates the binary path if it already exists.
// This means you never need to manually register services -- the watcher
// handles it on first deploy.
func (d *Deployer) ensureService(svc ServiceConfig, binPath string) error {
	existing := d.serviceExists(svc.WindowsServiceName)

	if !existing {
		d.l("service not registered, installing via NSSM", "name", svc.WindowsServiceName)

		out, err := runCommand(d.nssmPath, "install", svc.WindowsServiceName, binPath)
		if err != nil {
			return fmt.Errorf("nssm install %s: %w (output: %s)", svc.WindowsServiceName, err, string(out))
		}

		// Configure service settings
		logDir := filepath.Join(d.wcfg.InstallDir, "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			d.lWarn("could not create log dir", "path", logDir, "error", err)
		}

		settings := [][]string{
			{"AppDirectory", d.wcfg.InstallDir},
			{"AppParameters", svc.StartArguments},
			{"Start", "SERVICE_AUTO_START"},
			{"AppStdout", filepath.Join(logDir, svc.WindowsServiceName+".out.log")},
			{"AppStderr", filepath.Join(logDir, svc.WindowsServiceName+".err.log")},
			{"AppRotateFiles", "1"},
			{"AppRotateOnline", "1"},
			{"AppRotateSeconds", "86400"},
			{"AppRestartDelay", "5000"},
		}
		if svc.EnvFile != "" {
			settings = append(settings, []string{"AppEnvironmentExtra", "ENV_FILE=" + svc.EnvFile})
		}

		for _, kv := range settings {
			o, e := runCommand(d.nssmPath, "set", svc.WindowsServiceName, kv[0], kv[1])
			if e != nil {
				d.lWarn("nssm set warning", "key", kv[0], "error", e, "output", string(o))
			}
		}

		d.l("service installed", "name", svc.WindowsServiceName, "binary", binPath)
	} else {
		// Service exists -- update its executable settings in place.
		d.l("updating service settings", "name", svc.WindowsServiceName, "binary", binPath)
		settings := [][]string{
			{"Application", binPath},
			{"AppDirectory", d.wcfg.InstallDir},
			{"AppParameters", svc.StartArguments},
		}
		for _, kv := range settings {
			out, err := runCommand(d.nssmPath, "set", svc.WindowsServiceName, kv[0], kv[1])
			if err != nil {
				d.lWarn("failed to update service setting", "name", svc.WindowsServiceName, "key", kv[0], "error", err, "output", string(out))
			}
		}
	}

	return nil
}

// serviceExists checks if a Windows service is registered (via NSSM or SCM)
func (d *Deployer) serviceExists(name string) bool {
	out, err := runCommand(d.nssmPath, "status", name)
	if err != nil {
		// NSSM exits non-zero if service doesn't exist
		// Double-check the output to distinguish "not found" from other errors
		return !containsAny(string(out),
			"Can't open service",
			"does not exist",
			"OpenService()",
		)
	}
	return true
}

func (d *Deployer) stopServices(ctx context.Context, phase string) ([]ServiceConfig, error) {
	stopped := make([]ServiceConfig, 0, len(d.wcfg.Services))
	for _, svc := range d.wcfg.Services {
		wasActive, err := d.stopServiceByType(ctx, svc)
		if err != nil {
			failure := fmt.Errorf("stop %s during %s: %w", svc.WindowsServiceName, phase, err)
			return nil, d.recoverStoppedServices(ctx, stopped, failure)
		}
		if wasActive {
			stopped = append(stopped, svc)
		}
	}
	return stopped, nil
}

func (d *Deployer) recoverStoppedServices(ctx context.Context, services []ServiceConfig, cause error) error {
	if len(services) == 0 {
		return cause
	}

	d.lWarn("recovering previously running services", "count", len(services), "reason", cause)
	recoveryCtx := context.WithoutCancel(ctx)
	var recoveryErrors []error
	for _, svc := range services {
		if err := d.startServiceByType(recoveryCtx, svc); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("restart %s during recovery: %w", svc.WindowsServiceName, err))
		}
	}
	if len(recoveryErrors) == 0 {
		return cause
	}
	return errors.Join(cause, fmt.Errorf("service recovery failed: %w", errors.Join(recoveryErrors...)))
}

func (d *Deployer) startServices(ctx context.Context, currentDir, phase string) error {
	var startErrors []error
	for _, svc := range d.wcfg.Services {
		if err := d.ensureServiceByType(svc, currentDir); err != nil {
			startErrors = append(startErrors, fmt.Errorf("ensure service %s during %s: %w", svc.WindowsServiceName, phase, err))
			continue
		}
		if err := d.startServiceByType(ctx, svc); err != nil {
			startErrors = append(startErrors, fmt.Errorf("start service %s during %s: %w", svc.WindowsServiceName, phase, err))
		}
	}
	return errors.Join(startErrors...)
}

// stopServiceByType dispatches to the correct stop logic based on ServiceType.
// The boolean reports whether the service was active and may need compensation.
func (d *Deployer) stopServiceByType(ctx context.Context, svc ServiceConfig) (bool, error) {
	switch svc.ServiceType {
	case "iis", "static":
		// IIS targets do not have a service process to stop here.
		// IIS continues serving from the stable current/ path.
		d.l("iis service -- skipping stop", "name", svc.WindowsServiceName, "kind", svc.IISAppKind)
		return false, nil
	default: // "nssm"
		state, err := d.serviceManager.Status(ctx, svc.WindowsServiceName)
		if err != nil {
			if errors.Is(err, ErrServiceNotFound) {
				d.l("service is not registered; skipping stop", "name", svc.WindowsServiceName)
				return false, nil
			}
			return false, err
		}
		if state == ServiceStateStopped {
			d.l("service already stopped", "name", svc.WindowsServiceName)
			return false, nil
		}

		d.l("stopping service", "name", svc.WindowsServiceName)
		if err := d.serviceManager.Stop(ctx, svc.WindowsServiceName); err != nil {
			return false, err
		}
		d.l("service stopped", "name", svc.WindowsServiceName, "state", ServiceStateStopped)
		return true, nil
	}
}

// startServiceByType dispatches to the correct start logic based on ServiceType.
func (d *Deployer) startServiceByType(ctx context.Context, svc ServiceConfig) error {
	switch svc.ServiceType {
	case "iis", "static":
		return d.recycleAppPool(svc)
	default: // "nssm"
		d.l("starting service", "name", svc.WindowsServiceName)
		if err := d.serviceManager.Start(ctx, svc.WindowsServiceName); err != nil {
			return err
		}
		d.l("service running", "name", svc.WindowsServiceName, "state", ServiceStateRunning)
		return nil
	}
}

func appcmdPath() string {
	return `C:\Windows\System32\inetsrv\appcmd.exe`
}

func (d *Deployer) ensureIISService(svc ServiceConfig, currentDir string) error {
	svc = withDefaultIISTargets(svc)

	if strings.TrimSpace(svc.IISSiteName) == "" && strings.TrimSpace(svc.IISAppPool) == "" {
		return fmt.Errorf("iis service %s requires windows_service_name, iis_app_pool, or iis_site_name", svc.WindowsServiceName)
	}

	runtime := resolvedIISManagedRuntime(svc)
	d.l("ensuring IIS service", "name", svc.WindowsServiceName, "kind", svc.IISAppKind, "runtime", runtimeDisplay(runtime))

	if svc.IISAppPool != "" {
		if err := d.ensureIISAppPool(svc.IISAppPool, runtime); err != nil {
			return err
		}
	}
	if svc.IISSiteName != "" {
		if err := d.ensureIISSite(svc, currentDir); err != nil {
			return err
		}
	}
	return nil
}

func withDefaultIISTargets(svc ServiceConfig) ServiceConfig {
	defaultName := strings.TrimSpace(svc.WindowsServiceName)
	if strings.TrimSpace(svc.IISAppPool) == "" {
		svc.IISAppPool = defaultName
	}
	if strings.TrimSpace(svc.IISSiteName) == "" {
		svc.IISSiteName = defaultName
	}
	return svc
}

func resolvedIISManagedRuntime(svc ServiceConfig) string {
	switch strings.TrimSpace(strings.ToLower(svc.IISAppKind)) {
	case "", "static", "php":
		return ""
	case "aspnet_classic":
		if normalized := normalizeIISManagedRuntime(svc.IISManagedRuntime); normalized != "" {
			return normalized
		}
		return "v4.0"
	default:
		return normalizeIISManagedRuntime(svc.IISManagedRuntime)
	}
}

func runtimeDisplay(runtime string) string {
	if normalizeIISManagedRuntime(runtime) == "" {
		return "No Managed Code"
	}
	return normalizeIISManagedRuntime(runtime)
}

func (d *Deployer) ensureIISAppPool(name, runtime string) error {
	exists, err := d.iisObjectExists("apppool", name)
	if err != nil {
		return err
	}
	if exists {
		d.l("IIS app pool already exists", "pool", name)
	} else {
		d.l("creating IIS app pool", "pool", name)
		out, err := runCommand(appcmdPath(), "add", "apppool", "/name:"+name)
		if err != nil {
			return fmt.Errorf("create IIS app pool %s: %w (output: %s)", name, err, string(out))
		}
	}

	if err := d.setIISAppPoolManagedRuntime(name, runtime); err != nil {
		return err
	}
	return nil
}

func (d *Deployer) ensureIISSite(svc ServiceConfig, currentDir string) error {
	exists, err := d.iisObjectExists("site", svc.IISSiteName)
	if err != nil {
		return err
	}
	if !exists {
		binding, err := iisBindingFromPublicURL(svc.PublicURL)
		if err != nil {
			return fmt.Errorf("build IIS binding for %s: %w", svc.IISSiteName, err)
		}

		d.l("creating IIS site", "site", svc.IISSiteName, "binding", binding, "path", currentDir)
		out, err := runCommand(appcmdPath(), "add", "site", "/name:"+svc.IISSiteName, "/bindings:"+binding, "/physicalPath:"+currentDir)
		if err != nil {
			return fmt.Errorf("create IIS site %s: %w (output: %s)", svc.IISSiteName, err, string(out))
		}
	} else {
		d.l("IIS site already exists", "site", svc.IISSiteName)
	}

	if err := d.setIISSitePhysicalPath(svc.IISSiteName, currentDir); err != nil {
		return err
	}
	if svc.IISAppPool != "" {
		if err := d.setIISSiteAppPool(svc.IISSiteName, svc.IISAppPool); err != nil {
			return err
		}
	}
	return nil
}

func (d *Deployer) iisObjectExists(kind, name string) (bool, error) {
	out, err := runCommand(appcmdPath(), "list", kind, name)
	if err == nil {
		return true, nil
	}
	if isIISObjectMissingOutput(string(out)) {
		return false, nil
	}
	return false, fmt.Errorf("check IIS %s %s: %w (output: %s)", kind, name, err, string(out))
}

func isIISObjectMissingOutput(output string) bool {
	lower := strings.ToLower(output)
	return containsAny(lower,
		"cannot find requested collection element",
		"cannot find config object",
		"object identifier",
		"was not found",
		"does not exist",
	)
}

func normalizeIISManagedRuntime(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case "", "none", "no-managed-code", "no managed code":
		return ""
	case "v2.0", "v2":
		return "v2.0"
	case "v4.0", "v4", ".net clr v4.0":
		return "v4.0"
	default:
		return strings.TrimSpace(raw)
	}
}

func (d *Deployer) setIISAppPoolManagedRuntime(poolName, runtime string) error {
	runtime = normalizeIISManagedRuntime(runtime)
	display := runtime
	if display == "" {
		display = "No Managed Code"
	}

	d.l("configuring IIS app pool runtime", "pool", poolName, "runtime", display)
	out, err := runCommand(appcmdPath(), "set", "apppool", poolName, "/managedRuntimeVersion:"+runtime)
	if err != nil {
		return fmt.Errorf("set IIS app pool %s runtime %s: %w (output: %s)", poolName, display, err, string(out))
	}
	return nil
}

func (d *Deployer) setIISSitePhysicalPath(siteName, currentDir string) error {
	d.l("updating IIS site path", "site", siteName, "path", currentDir)
	out, err := runCommand(appcmdPath(), "set", "vdir", siteName+"/", "/physicalPath:"+currentDir)
	if err != nil {
		return fmt.Errorf("set IIS site %s physical path: %w (output: %s)", siteName, err, string(out))
	}
	return nil
}

func (d *Deployer) setIISSiteAppPool(siteName, appPool string) error {
	d.l("assigning IIS app pool", "site", siteName, "pool", appPool)
	out, err := runCommand(appcmdPath(), "set", "app", siteName+"/", "/applicationPool:"+appPool)
	if err != nil {
		return fmt.Errorf("set IIS site %s app pool %s: %w (output: %s)", siteName, appPool, err, string(out))
	}
	return nil
}

func iisBindingFromPublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("public_url is required to auto-create an IIS site")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse public_url: %w", err)
	}
	if u.Scheme == "" {
		return "", fmt.Errorf("public_url must include a scheme, for example http://example.com")
	}

	protocol := strings.ToLower(u.Scheme)
	if protocol != "http" && protocol != "https" {
		return "", fmt.Errorf("unsupported public_url scheme %q", u.Scheme)
	}

	port := u.Port()
	if port == "" {
		if protocol == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	host := u.Hostname()
	return fmt.Sprintf("%s/*:%s:%s", protocol, port, host), nil
}

// recycleAppPool recycles an IIS app pool via appcmd.exe.
// This clears cached content and picks up the newly swapped junction files.
func (d *Deployer) recycleAppPool(svc ServiceConfig) error {
	if svc.IISAppPool == "" {
		d.l("no IIS app pool configured, skipping recycle", "name", svc.WindowsServiceName)
		return nil
	}

	d.l("recycling IIS app pool", "pool", svc.IISAppPool)

	out, err := runCommand(appcmdPath(), "recycle", "apppool", svc.IISAppPool)
	if err != nil {
		d.lWarn("app pool recycle failed", "pool", svc.IISAppPool, "error", err, "output", string(out))
		return fmt.Errorf("recycle apppool %s: %w (output: %s)", svc.IISAppPool, err, string(out))
	}

	d.l("app pool recycled", "pool", svc.IISAppPool)
	return nil
}

func (d *Deployer) healthCheck(ctx context.Context, serviceName, url string) error {
	hc := d.wcfg.HealthCheck
	client := &http.Client{Timeout: time.Duration(hc.TimeoutSec) * time.Second}
	interval := time.Duration(hc.IntervalSec) * time.Second

	d.l("health check", "service", serviceName, "url", url, "retries", hc.Retries)

	for i := 1; i <= hc.Retries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			d.l("health check passed", "service", serviceName, "attempt", i)
			return nil
		}
		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		d.lWarn("not healthy yet", "service", serviceName, "attempt", i, "status", status)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("not healthy after %d attempts", hc.Retries)
}

// containsAny reports whether s contains any of the given substrings
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ── Version retention ─────────────────────────────────────────────────

// ReleaseInfo describes a version directory on disk.
type ReleaseInfo struct {
	Version     string    `json:"version"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	ModTime     time.Time `json:"mod_time"`
	IsCurrent   bool      `json:"is_current"`
	HasSnapshot bool      `json:"has_snapshot"`
}

// ListAvailableVersions returns the release directories on disk for a given installDir.
func ListAvailableVersions(installDir string) ([]ReleaseInfo, error) {
	releasesDir := filepath.Join(installDir, "releases")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read releases dir: %w", err)
	}

	// Determine current version by reading the junction/symlink target
	currentDir := filepath.Join(installDir, "current")
	currentTarget, _ := os.Readlink(currentDir)
	// On Windows with junctions, Readlink may fail — try filepath.EvalSymlinks
	if currentTarget == "" {
		currentTarget, _ = filepath.EvalSymlinks(currentDir)
	}

	var versions []ReleaseInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(releasesDir, e.Name())
		ri := ReleaseInfo{
			Version:     restoreReleaseVersion(e.Name()),
			Path:        fullPath,
			ModTime:     info.ModTime(),
			SizeBytes:   dirSize(fullPath),
			IsCurrent:   fullPath == currentTarget,
			HasSnapshot: HasConfigSnapshot(fullPath),
		}
		versions = append(versions, ri)
	}

	// Sort by mod time descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ModTime.After(versions[j].ModTime)
	})

	return versions, nil
}

// CleanOldReleases removes old release directories, keeping only the `keep` most recent.
func CleanOldReleases(installDir string, keep int) error {
	versions, err := ListAvailableVersions(installDir)
	if err != nil {
		return err
	}

	if len(versions) <= keep {
		return nil
	}

	for _, v := range versions[keep:] {
		if v.IsCurrent {
			continue // never delete the current version
		}
		if err := os.RemoveAll(v.Path); err != nil {
			return fmt.Errorf("remove old release %s: %w", v.Version, err)
		}
	}
	return nil
}

// dirSize calculates the total size of all files in a directory tree.
func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

// DeleteVersion removes a specific version directory.
func DeleteVersion(installDir, version string) error {
	versions, err := ListAvailableVersions(installDir)
	if err != nil {
		return err
	}

	for _, v := range versions {
		if v.Version == version {
			if v.IsCurrent {
				return fmt.Errorf("cannot delete the current active version")
			}
			if err := os.RemoveAll(v.Path); err != nil {
				return fmt.Errorf("failed to remove version %s: %w", version, err)
			}
			return nil
		}
	}
	return fmt.Errorf("version %s not found on disk", version)
}
