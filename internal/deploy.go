package internal

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Deployer struct {
	cfg *Config
	log *Logger
}

func NewDeployer(cfg *Config, log *Logger) *Deployer {
	return &Deployer{cfg: cfg, log: log}
}

// Deploy extracts zipPath into releases/<version>/, swaps current/,
// restarts all configured services, validates health, and rolls back on failure.
func (d *Deployer) Deploy(ctx context.Context, version, zipPath, previousVersion string) error {
	releaseDir := filepath.Join(d.cfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.cfg.InstallDir, "current")

	d.log.Info("starting deploy", "version", version, "release_dir", releaseDir)

	// 1. Extract zip into releases/<version>/
	if err := d.extractZip(zipPath, releaseDir); err != nil {
		return fmt.Errorf("extract zip: %w", err)
	}
	d.log.Info("zip extracted", "dir", releaseDir)

	// 2. Stop all services
	d.log.Info("stopping all managed services")
	for _, svc := range d.cfg.Services {
		if err := d.stopService(svc.WindowsServiceName); err != nil {
			d.log.Warn("stop service warning (continuing)", "service", svc.WindowsServiceName, "error", err)
		}
	}

	// 3. Swap current/ junction to the new release
	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap current: %w", err)
	}
	d.log.Info("current dir swapped", "pointing_to", releaseDir)

	// 4. Update each service binary path and start
	d.log.Info("starting all managed services")
	for _, svc := range d.cfg.Services {
		// Update the NSSM application path to the new binary
		newBinPath := filepath.Join(currentDir, svc.BinaryName)
		if err := d.updateServiceBinary(svc.WindowsServiceName, newBinPath); err != nil {
			d.log.Warn("failed to update service binary path", "service", svc.WindowsServiceName, "error", err)
		}

		if err := d.startService(svc.WindowsServiceName); err != nil {
			d.log.Error("failed to start service", "service", svc.WindowsServiceName, "error", err)
			return d.tryRollback(ctx, previousVersion, fmt.Errorf("start service %s: %w", svc.WindowsServiceName, err))
		}
	}

	// 5. Health check per service
	if d.cfg.HealthCheck.Enabled {
		for _, svc := range d.cfg.Services {
			url := svc.HealthCheckURL
			if url == "" {
				url = d.cfg.HealthCheck.URL
			}
			if url == "" {
				d.log.Debug("no health check URL for service, skipping", "service", svc.WindowsServiceName)
				continue
			}
			if err := d.healthCheck(ctx, svc.WindowsServiceName, url); err != nil {
				d.log.Error("health check failed", "service", svc.WindowsServiceName, "error", err)
				return d.tryRollback(ctx, previousVersion, fmt.Errorf("health check failed for %s: %w", svc.WindowsServiceName, err))
			}
		}
	}

	d.log.Info("deploy successful", "version", version)
	return nil
}

// Rollback swaps current/ back to releases/<previousVersion>/ and restarts services
func (d *Deployer) Rollback(ctx context.Context, version string) error {
	releaseDir := filepath.Join(d.cfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.cfg.InstallDir, "current")

	d.log.Warn("rolling back", "to_version", version)

	if _, err := os.Stat(releaseDir); os.IsNotExist(err) {
		return fmt.Errorf("rollback target %s not found on disk — cannot roll back", releaseDir)
	}

	for _, svc := range d.cfg.Services {
		d.stopService(svc.WindowsServiceName)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap current during rollback: %w", err)
	}

	for _, svc := range d.cfg.Services {
		newBinPath := filepath.Join(currentDir, svc.BinaryName)
		d.updateServiceBinary(svc.WindowsServiceName, newBinPath)
		if err := d.startService(svc.WindowsServiceName); err != nil {
			return fmt.Errorf("start service %s during rollback: %w", svc.WindowsServiceName, err)
		}
	}

	if d.cfg.HealthCheck.Enabled {
		for _, svc := range d.cfg.Services {
			url := svc.HealthCheckURL
			if url == "" {
				url = d.cfg.HealthCheck.URL
			}
			if url == "" {
				continue
			}
			if err := d.healthCheck(ctx, svc.WindowsServiceName, url); err != nil {
				return fmt.Errorf("health check failed after rollback for %s: %w", svc.WindowsServiceName, err)
			}
		}
	}

	d.log.Info("rollback successful", "version", version)
	return nil
}

// tryRollback attempts a rollback and wraps the original error
func (d *Deployer) tryRollback(ctx context.Context, previousVersion string, originalErr error) error {
	if previousVersion == "" {
		return fmt.Errorf("%w (no previous version to roll back to)", originalErr)
	}

	d.log.Warn("attempting automatic rollback", "to_version", previousVersion, "reason", originalErr)
	if rbErr := d.Rollback(ctx, previousVersion); rbErr != nil {
		return fmt.Errorf("deploy failed AND rollback failed: deploy_err=%w rollback_err=%v", originalErr, rbErr)
	}

	return fmt.Errorf("deploy failed, rolled back to %s: %w", previousVersion, originalErr)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (d *Deployer) extractZip(zipPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
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
	// Sanitize path — prevent zip-slip attacks
	destPath := filepath.Join(destDir, filepath.Clean("/"+f.Name)[1:])
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

// swapCurrent creates a directory junction from currentDir → releaseDir.
// On Windows, mklink /J does not require elevated privileges.
// Falls back to copying if mklink fails.
func (d *Deployer) swapCurrent(releaseDir, currentDir string) error {
	// Remove existing junction or directory
	if _, err := os.Lstat(currentDir); err == nil {
		if err := os.Remove(currentDir); err != nil {
			if err2 := os.RemoveAll(currentDir); err2 != nil {
				return fmt.Errorf("remove old current: %w", err2)
			}
		}
	}

	out, err := exec.Command("cmd", "/C", "mklink", "/J", currentDir, releaseDir).CombinedOutput()
	if err != nil {
		d.log.Warn("mklink /J failed, falling back to directory copy", "output", string(out))
		return copyDir(releaseDir, currentDir)
	}
	return nil
}

func (d *Deployer) updateServiceBinary(serviceName, newBinPath string) error {
	out, err := exec.Command(d.cfg.NssmPath, "set", serviceName, "Application", newBinPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nssm set Application: %w (output: %s)", err, string(out))
	}
	return nil
}

func (d *Deployer) stopService(name string) error {
	d.log.Info("stopping service", "name", name)
	out, err := exec.Command(d.cfg.NssmPath, "stop", name, "confirm").CombinedOutput()
	if err != nil {
		// Not fatal — service may already be stopped
		d.log.Debug("stop returned non-zero (may already be stopped)", "name", name, "output", string(out))
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (d *Deployer) startService(name string) error {
	d.log.Info("starting service", "name", name)
	out, err := exec.Command(d.cfg.NssmPath, "start", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nssm start %s: %w (output: %s)", name, err, string(out))
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (d *Deployer) healthCheck(ctx context.Context, serviceName, url string) error {
	cfg := d.cfg.HealthCheck
	client := &http.Client{Timeout: time.Duration(cfg.TimeoutSec) * time.Second}
	interval := time.Duration(cfg.IntervalSec) * time.Second

	d.log.Info("health check", "service", serviceName, "url", url, "retries", cfg.Retries)

	for i := 1; i <= cfg.Retries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			d.log.Info("health check passed", "service", serviceName, "attempt", i)
			return nil
		}

		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		d.log.Warn("health check not ready", "service", serviceName, "attempt", i, "status", status, "error", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}

	return fmt.Errorf("not healthy after %d attempts", cfg.Retries)
}

// copyDir recursively copies src into dst (fallback when mklink unavailable)
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