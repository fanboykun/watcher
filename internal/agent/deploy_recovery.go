package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type deployRecoveryError struct {
	cause                 error
	rollbackVersion       string
	rollbackErr           error
	restoredBackupVersion string
	noRollbackVersion     bool
}

// Error formats the deployment recovery failure and its compensation result.
func (e *deployRecoveryError) Error() string {
	switch {
	case e.rollbackVersion != "":
		return fmt.Sprintf("deploy failed, rolled back to %s: %v", e.rollbackVersion, e.cause)
	case e.rollbackErr != nil:
		return fmt.Sprintf("deploy failed AND rollback failed: deploy=%v rollback=%v", e.cause, e.rollbackErr)
	case e.restoredBackupVersion != "":
		return fmt.Sprintf("deploy failed, restored release backup for %s: %v", e.restoredBackupVersion, e.cause)
	case e.noRollbackVersion:
		return fmt.Sprintf("%v (no previous version to roll back to)", e.cause)
	default:
		return e.cause.Error()
	}
}

// Unwrap exposes the original deployment failure for errors.Is and errors.As.
func (e *deployRecoveryError) Unwrap() error {
	return e.cause
}

// resolveRollbackVersion selects a valid recorded previous version for automatic rollback.
func (d *Deployer) resolveRollbackVersion(targetVersion, previousVersion string) string {
	previousVersion = strings.TrimSpace(previousVersion)
	if previousVersion == "" || previousVersion == targetVersion {
		return ""
	}

	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", releaseStorageName(previousVersion))
	if _, err := os.Stat(releaseDir); err == nil {
		return previousVersion
	}
	d.lWarn("configured previous version is unavailable on disk", "version", previousVersion, "path", releaseDir)
	return ""
}

// Rollback activates a retained release and verifies its services.
func (d *Deployer) Rollback(ctx context.Context, version string) error {
	return d.rollback(ctx, version, true)
}

// rollback activates and verifies a retained version, restoring the original on failure when requested.
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
	if err := ValidateConfigSnapshot(d.wcfg.InstallDir, version); err != nil {
		return fmt.Errorf("validate rollback target config: %w", err)
	}
	if originalVersion != "" {
		if err := ValidateConfigSnapshot(d.wcfg.InstallDir, originalVersion); err != nil {
			return fmt.Errorf("validate recovery config for current version %s: %w", originalVersion, err)
		}
	}

	stoppedServices, err := d.stopServices(ctx, "rollback")
	if err != nil {
		return err
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return d.recoverStoppedServices(ctx, stoppedServices, fmt.Errorf("swap during rollback: %w", err))
	}

	if err := RestoreConfigSnapshot(d.wcfg, version, currentDir); err != nil {
		return d.recoverManualRollback(ctx, version, originalVersion, originalReleaseDir, currentDir,
			fmt.Errorf("restore rollback config: %w", err))
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

// recoverManualRollback restores the original release when a manual rollback target fails.
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
	if err := RestoreConfigSnapshot(d.wcfg, originalVersion, currentDir); err != nil {
		return errors.Join(originalErr, fmt.Errorf("restore original config for %s: %w", originalVersion, err))
	}
	if err := d.startServices(recoveryCtx, currentDir, "manual rollback recovery"); err != nil {
		return errors.Join(originalErr, fmt.Errorf("restart original release %s: %w", originalVersion, err))
	}
	return fmt.Errorf("rollback to %s failed, restored %s: %w", targetVersion, originalVersion, originalErr)
}

// tryRollbackWithResult attempts a version rollback and preserves both deployment and rollback errors.
func (d *Deployer) tryRollbackWithResult(ctx context.Context, previousVersion string, originalErr error) (bool, error) {
	if previousVersion == "" {
		return false, &deployRecoveryError{cause: originalErr, noRollbackVersion: true}
	}
	d.lWarn("attempting rollback", "to", previousVersion, "reason", originalErr)
	if rbErr := d.rollback(ctx, previousVersion, false); rbErr != nil {
		return false, &deployRecoveryError{cause: originalErr, rollbackErr: rbErr}
	}
	return true, &deployRecoveryError{cause: originalErr, rollbackVersion: previousVersion}
}

// recoverActivatedDeployment compensates after the new release has already been activated.
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
		return &deployRecoveryError{cause: originalErr, rollbackErr: combinedRollbackErr}
	}
	d.cleanupPromotion(promotion)
	return &deployRecoveryError{cause: originalErr, restoredBackupVersion: version}
}

// restorePromotionAndRestart restores the pre-promotion release and restarts its services.
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
