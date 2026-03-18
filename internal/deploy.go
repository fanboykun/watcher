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
	wcfg     *WatcherConfig
	nssmPath string
	log      *Logger
}

func NewDeployer(wcfg *WatcherConfig, nssmPath string, log *Logger) *Deployer {
	return &Deployer{wcfg: wcfg, nssmPath: nssmPath, log: log}
}

func (d *Deployer) Deploy(ctx context.Context, version, zipPath, previousVersion string) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")

	d.log.Info("deploying", "version", version, "release_dir", releaseDir)

	if err := d.extractZip(zipPath, releaseDir); err != nil {
		return fmt.Errorf("extract zip: %w", err)
	}

	d.log.Info("stopping services")
	for _, svc := range d.wcfg.Services {
		d.stopService(svc.WindowsServiceName)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap current: %w", err)
	}

	d.log.Info("starting services")
	for _, svc := range d.wcfg.Services {
		newBin := filepath.Join(currentDir, svc.BinaryName)
		d.updateServiceBinary(svc.WindowsServiceName, newBin)

		if err := d.startService(svc.WindowsServiceName); err != nil {
			return d.tryRollback(ctx, previousVersion,
				fmt.Errorf("start %s: %w", svc.WindowsServiceName, err))
		}
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
				return d.tryRollback(ctx, previousVersion,
					fmt.Errorf("health check failed for %s: %w", svc.WindowsServiceName, err))
			}
		}
	}

	d.log.Info("deploy successful", "version", version)
	return nil
}

func (d *Deployer) Rollback(ctx context.Context, version string) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")

	d.log.Warn("rolling back", "to_version", version)

	if _, err := os.Stat(releaseDir); os.IsNotExist(err) {
		return fmt.Errorf("rollback target %s not on disk", releaseDir)
	}

	for _, svc := range d.wcfg.Services {
		d.stopService(svc.WindowsServiceName)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap during rollback: %w", err)
	}

	for _, svc := range d.wcfg.Services {
		newBin := filepath.Join(currentDir, svc.BinaryName)
		d.updateServiceBinary(svc.WindowsServiceName, newBin)
		if err := d.startService(svc.WindowsServiceName); err != nil {
			return fmt.Errorf("start %s during rollback: %w", svc.WindowsServiceName, err)
		}
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
				return fmt.Errorf("health check failed after rollback for %s: %w", svc.WindowsServiceName, err)
			}
		}
	}

	d.log.Info("rollback successful", "version", version)
	return nil
}

func (d *Deployer) tryRollback(ctx context.Context, previousVersion string, originalErr error) error {
	if previousVersion == "" {
		return fmt.Errorf("%w (no previous version to roll back to)", originalErr)
	}
	d.log.Warn("attempting rollback", "to", previousVersion, "reason", originalErr)
	if rbErr := d.Rollback(ctx, previousVersion); rbErr != nil {
		return fmt.Errorf("deploy failed AND rollback failed: deploy=%w rollback=%v", originalErr, rbErr)
	}
	return fmt.Errorf("deploy failed, rolled back to %s: %w", previousVersion, originalErr)
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

func (d *Deployer) swapCurrent(releaseDir, currentDir string) error {
	if _, err := os.Lstat(currentDir); err == nil {
		if err := os.Remove(currentDir); err != nil {
			if err2 := os.RemoveAll(currentDir); err2 != nil {
				return fmt.Errorf("remove old current: %w", err2)
			}
		}
	}
	out, err := exec.Command("cmd", "/C", "mklink", "/J", currentDir, releaseDir).CombinedOutput()
	if err != nil {
		d.log.Warn("mklink /J failed, falling back to copy", "output", string(out))
		return copyDir(releaseDir, currentDir)
	}
	return nil
}

func (d *Deployer) updateServiceBinary(name, binPath string) {
	out, err := exec.Command(d.nssmPath, "set", name, "Application", binPath).CombinedOutput()
	if err != nil {
		d.log.Warn("failed to update service binary", "name", name, "error", err, "output", string(out))
	}
}

func (d *Deployer) stopService(name string) {
	d.log.Info("stopping service", "name", name)
	out, err := exec.Command(d.nssmPath, "stop", name, "confirm").CombinedOutput()
	if err != nil {
		d.log.Debug("stop returned non-zero (may already be stopped)", "name", name, "output", string(out))
	}
	time.Sleep(2 * time.Second)
}

func (d *Deployer) startService(name string) error {
	d.log.Info("starting service", "name", name)
	out, err := exec.Command(d.nssmPath, "start", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("nssm start %s: %w (output: %s)", name, err, string(out))
	}
	time.Sleep(2 * time.Second)
	return nil
}

func (d *Deployer) healthCheck(ctx context.Context, serviceName, url string) error {
	hc := d.wcfg.HealthCheck
	client := &http.Client{Timeout: time.Duration(hc.TimeoutSec) * time.Second}
	interval := time.Duration(hc.IntervalSec) * time.Second

	d.log.Info("health check", "service", serviceName, "url", url, "retries", hc.Retries)

	for i := 1; i <= hc.Retries; i++ {
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
		d.log.Warn("not healthy yet", "service", serviceName, "attempt", i, "status", status)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("not healthy after %d attempts", hc.Retries)
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