package agent

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// Extract to a temporary directory first to avoid file-in-use errors during redeploys
	tempReleaseDir := releaseDir + fmt.Sprintf("-%d", time.Now().UnixNano())

	if err := d.extractZip(zipPath, tempReleaseDir); err != nil {
		os.RemoveAll(tempReleaseDir)
		return fmt.Errorf("extract zip: %w", err)
	}

	d.log.Info("stopping services")
	for _, svc := range d.wcfg.Services {
		d.stopServiceByType(svc)
	}

	// Now that services are stopped, safely remove the old releaseDir if it exists (for redeploys)
	if err := os.RemoveAll(releaseDir); err != nil {
		d.log.Warn("failed to remove existing release dir", "dir", releaseDir, "error", err)
	}

	// Rename temp directory to final release directory
	if err := os.Rename(tempReleaseDir, releaseDir); err != nil {
		d.log.Warn("rename failed, falling back to copy", "error", err)
		if err := copyDir(tempReleaseDir, releaseDir); err != nil {
			return fmt.Errorf("rename fallback copy: %w", err)
		}
		os.RemoveAll(tempReleaseDir)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap current: %w", err)
	}

	d.log.Info("starting services")
	for _, svc := range d.wcfg.Services {
		if err := d.ensureServiceByType(svc, currentDir); err != nil {
			return d.tryRollback(ctx, previousVersion,
				fmt.Errorf("ensure service %s: %w", svc.WindowsServiceName, err))
		}

		if err := d.startServiceByType(svc); err != nil {
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
		d.stopServiceByType(svc)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap during rollback: %w", err)
	}

	for _, svc := range d.wcfg.Services {
		if err := d.ensureServiceByType(svc, currentDir); err != nil {
			return fmt.Errorf("ensure service %s during rollback: %w", svc.WindowsServiceName, err)
		}
		if err := d.startServiceByType(svc); err != nil {
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

// ensureServiceByType dispatches to the correct ensure logic based on ServiceType.
func (d *Deployer) ensureServiceByType(svc ServiceConfig, currentDir string) error {
	switch svc.ServiceType {
	case "static":
		d.log.Info("static service — no NSSM registration needed", "name", svc.WindowsServiceName)
		return nil
	default: // "nssm"
		newBin := filepath.Join(currentDir, svc.BinaryName)
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
		d.log.Info("service not registered, installing via NSSM", "name", svc.WindowsServiceName)

		out, err := exec.Command(d.nssmPath, "install", svc.WindowsServiceName, binPath).CombinedOutput()
		if err != nil {
			return fmt.Errorf("nssm install %s: %w (output: %s)", svc.WindowsServiceName, err, string(out))
		}

		// Configure service settings
		logDir := filepath.Join(d.wcfg.InstallDir, "logs")
		if err := os.MkdirAll(logDir, 0755); err != nil {
			d.log.Warn("could not create log dir", "path", logDir, "error", err)
		}

		settings := [][]string{
			{"AppDirectory", d.wcfg.InstallDir},
			{"Start", "SERVICE_AUTO_START"},
			{"AppStdout", filepath.Join(logDir, svc.WindowsServiceName+".out.log")},
			{"AppStderr", filepath.Join(logDir, svc.WindowsServiceName+".err.log")},
			{"AppRotateFiles", "1"},
			{"AppRotateOnline", "1"},
			{"AppRotateSeconds", "86400"},
			{"AppRestartDelay", "5000"},
		}
		if svc.EnvFile != "" {
			settings = append(settings, []string{"AppEnvExtra", "ENV_FILE=" + svc.EnvFile})
		}

		for _, kv := range settings {
			o, e := exec.Command(d.nssmPath, "set", svc.WindowsServiceName, kv[0], kv[1]).CombinedOutput()
			if e != nil {
				d.log.Warn("nssm set warning", "key", kv[0], "error", e, "output", string(o))
			}
		}

		d.log.Info("service installed", "name", svc.WindowsServiceName, "binary", binPath)
	} else {
		// Service exists -- just update the binary path
		d.log.Info("updating service binary", "name", svc.WindowsServiceName, "binary", binPath)
		out, err := exec.Command(d.nssmPath, "set", svc.WindowsServiceName, "Application", binPath).CombinedOutput()
		if err != nil {
			d.log.Warn("failed to update binary path", "name", svc.WindowsServiceName, "error", err, "output", string(out))
		}
	}

	return nil
}

// serviceExists checks if a Windows service is registered (via NSSM or SCM)
func (d *Deployer) serviceExists(name string) bool {
	out, err := exec.Command(d.nssmPath, "status", name).CombinedOutput()
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

// stopServiceByType dispatches to the correct stop logic based on ServiceType.
func (d *Deployer) stopServiceByType(svc ServiceConfig) {
	switch svc.ServiceType {
	case "static":
		// Static services don't have a process to stop.
		// Optionally stop the IIS app pool, but usually unnecessary
		// since we just swap the junction.
		d.log.Info("static service — skipping stop", "name", svc.WindowsServiceName)
	default: // "nssm"
		d.stopService(svc.WindowsServiceName)
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

// startServiceByType dispatches to the correct start logic based on ServiceType.
func (d *Deployer) startServiceByType(svc ServiceConfig) error {
	switch svc.ServiceType {
	case "static":
		return d.recycleAppPool(svc)
	default: // "nssm"
		return d.startService(svc.WindowsServiceName)
	}
}

func (d *Deployer) startService(name string) error {
	d.log.Info("starting service", "name", name)
	outBytes, err := exec.Command(d.nssmPath, "start", name).CombinedOutput()
	out := string(outBytes)
	
	if err != nil && !strings.Contains(out, "SERVICE_START_PENDING") && !strings.Contains(out, "SERVICE_RUNNING") {
		return fmt.Errorf("nssm start %s: %w (output: %s)", name, err, out)
	}
	if err != nil {
		d.log.Info("service start pending or already running", "name", name, "output", out)
	}

	time.Sleep(2 * time.Second)
	return nil
}

// recycleAppPool recycles an IIS app pool via appcmd.exe.
// This clears cached content and picks up the newly swapped junction files.
func (d *Deployer) recycleAppPool(svc ServiceConfig) error {
	if svc.IISAppPool == "" {
		d.log.Info("no IIS app pool configured, skipping recycle", "name", svc.WindowsServiceName)
		return nil
	}

	appcmd := `C:\Windows\System32\inetsrv\appcmd.exe`
	d.log.Info("recycling IIS app pool", "pool", svc.IISAppPool)

	out, err := exec.Command(appcmd, "recycle", "apppool", svc.IISAppPool).CombinedOutput()
	if err != nil {
		d.log.Warn("app pool recycle failed", "pool", svc.IISAppPool, "error", err, "output", string(out))
		return fmt.Errorf("recycle apppool %s: %w (output: %s)", svc.IISAppPool, err, string(out))
	}

	d.log.Info("app pool recycled", "pool", svc.IISAppPool)
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