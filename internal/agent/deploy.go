package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// NewDeployer creates a configured deployer.
func NewDeployer(wcfg *WatcherConfig, nssmPath string, log *Logger, logFn func(string)) *Deployer {
	return &Deployer{
		wcfg:           wcfg,
		nssmPath:       nssmPath,
		serviceManager: NewNSSMServiceManager(nssmPath),
		log:            log,
		logFn:          logFn,
	}
}

// l writes an informational deployment message to both structured and persisted logs.
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

// lWarn writes a warning deployment message to both structured and persisted logs.
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

// Deploy executes the validated release deployment and compensation pipeline.
func (d *Deployer) Deploy(ctx context.Context, version, zipPath, previousVersion string) (deployErr error) {
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
	envFiles, err := d.prepareDeploymentEnv()
	if err != nil {
		return fmt.Errorf("prepare environment: %w", err)
	}
	// Keep the trusted snapshot for a redeployed version until it succeeds.
	restoreSnapshot, err := preserveConfigSnapshot(d.wcfg.InstallDir, version)
	if err != nil {
		return err
	}
	defer func() { deployErr = restoreSnapshot(deployErr) }()
	if err := d.captureConfigSnapshot(version, SnapshotSourceDeployment); err != nil {
		return fmt.Errorf("capture config snapshot: %w", err)
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

	if err := applyDeploymentEnv(envFiles); err != nil {
		failure := restoreDeploymentEnv(envFiles, fmt.Errorf("write deployment environment: %w", err))
		return d.recoverActivatedDeployment(ctx, version, currentDir, rollbackVersion, failure, promotion)
	}

	d.l("starting services")
	if err := d.startServices(ctx, currentDir, "deploy"); err != nil {
		return d.recoverActivatedDeployment(ctx, version, currentDir, rollbackVersion, restoreDeploymentEnv(envFiles, err), promotion)
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
					restoreDeploymentEnv(envFiles, fmt.Errorf("health check failed for %s: %w", svc.WindowsServiceName, err)), promotion)
			}
		}
	}

	d.cleanupPromotion(promotion)
	d.l("deploy successful", "version", version)
	return nil
}
