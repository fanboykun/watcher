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
	"sort"
	"strings"
	"time"
)

type Deployer struct {
	wcfg     *WatcherConfig
	nssmPath string
	log      *Logger
	logFn    func(string)
}

func NewDeployer(wcfg *WatcherConfig, nssmPath string, log *Logger, logFn func(string)) *Deployer {
	return &Deployer{wcfg: wcfg, nssmPath: nssmPath, log: log, logFn: logFn}
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

func (d *Deployer) Deploy(ctx context.Context, version, zipPath, previousVersion string) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")

	d.l("deploying", "version", version, "release_dir", releaseDir)

	// Extract to a temporary directory first to avoid file-in-use errors during redeploys
	tempReleaseDir := releaseDir + fmt.Sprintf("-%d", time.Now().UnixNano())

	if err := d.extractZip(zipPath, tempReleaseDir); err != nil {
		os.RemoveAll(tempReleaseDir)
		return fmt.Errorf("extract zip: %w", err)
	}

	d.l("stopping services")
	for _, svc := range d.wcfg.Services {
		if err := d.stopServiceByType(svc); err != nil {
			return fmt.Errorf("stop %s: %w", svc.WindowsServiceName, err)
		}
	}

	// Now that services are stopped, safely remove the old releaseDir if it exists (for redeploys)
	if err := os.RemoveAll(releaseDir); err != nil {
		d.lWarn("failed to remove existing release dir", "dir", releaseDir, "error", err)
	}

	// Rename temp directory to final release directory
	if err := os.Rename(tempReleaseDir, releaseDir); err != nil {
		d.lWarn("rename failed, falling back to copy", "error", err)
		if err := copyDir(tempReleaseDir, releaseDir); err != nil {
			return fmt.Errorf("rename fallback copy: %w", err)
		}
		os.RemoveAll(tempReleaseDir)
	}

	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		return fmt.Errorf("swap current: %w", err)
	}

	d.l("starting services")
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

	d.l("deploy successful", "version", version)
	return nil
}

func (d *Deployer) Rollback(ctx context.Context, version string) error {
	releaseDir := filepath.Join(d.wcfg.InstallDir, "releases", version)
	currentDir := filepath.Join(d.wcfg.InstallDir, "current")

	d.lWarn("rolling back", "to_version", version)

	if _, err := os.Stat(releaseDir); os.IsNotExist(err) {
		return fmt.Errorf("rollback target %s not on disk", releaseDir)
	}

	for _, svc := range d.wcfg.Services {
		if err := d.stopServiceByType(svc); err != nil {
			return fmt.Errorf("stop %s during rollback: %w", svc.WindowsServiceName, err)
		}
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

	d.l("rollback successful", "version", version)
	return nil
}

func (d *Deployer) tryRollback(ctx context.Context, previousVersion string, originalErr error) error {
	if previousVersion == "" {
		return fmt.Errorf("%w (no previous version to roll back to)", originalErr)
	}
	d.lWarn("attempting rollback", "to", previousVersion, "reason", originalErr)
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
		d.lWarn("mklink /J failed, falling back to copy", "output", string(out))
		return copyDir(releaseDir, currentDir)
	}
	return nil
}

// ensureServiceByType dispatches to the correct ensure logic based on ServiceType.
func (d *Deployer) ensureServiceByType(svc ServiceConfig, currentDir string) error {
	switch svc.ServiceType {
	case "static":
		d.l("static service -- no NSSM registration needed", "name", svc.WindowsServiceName)
		return nil
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

		out, err := exec.Command(d.nssmPath, "install", svc.WindowsServiceName, binPath).CombinedOutput()
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
			o, e := exec.Command(d.nssmPath, "set", svc.WindowsServiceName, kv[0], kv[1]).CombinedOutput()
			if e != nil {
				d.lWarn("nssm set warning", "key", kv[0], "error", e, "output", string(o))
			}
		}

		d.l("service installed", "name", svc.WindowsServiceName, "binary", binPath)
	} else {
		// Service exists -- just update the binary path
		d.l("updating service binary", "name", svc.WindowsServiceName, "binary", binPath)
		out, err := exec.Command(d.nssmPath, "set", svc.WindowsServiceName, "Application", binPath).CombinedOutput()
		if err != nil {
			d.lWarn("failed to update binary path", "name", svc.WindowsServiceName, "error", err, "output", string(out))
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
func (d *Deployer) stopServiceByType(svc ServiceConfig) error {
	switch svc.ServiceType {
	case "static":
		// Static services don't have a process to stop.
		// Optionally stop the IIS app pool, but usually unnecessary
		// since we just swap the junction.
		d.l("static service -- skipping stop", "name", svc.WindowsServiceName)
		return nil
	default: // "nssm"
		return d.stopService(svc.WindowsServiceName)
	}
}

func (d *Deployer) stopService(name string) error {
	const (
		gracefulTimeout = 45 * time.Second
		forceTimeout    = 20 * time.Second
		pollInterval    = 2 * time.Second
	)

	d.l("stopping service", "name", name)
	out, err := exec.Command(d.nssmPath, "stop", name, "confirm").CombinedOutput()
	if err != nil && !isServiceMissingOutput(string(out)) {
		d.lWarn("nssm stop returned non-zero", "name", name, "error", err, "output", string(out))
	}

	state, waitErr := d.waitForServiceState(name, []string{"SERVICE_STOPPED", "SERVICE_MISSING"}, gracefulTimeout, pollInterval)
	if waitErr == nil {
		d.l("service stopped", "name", name, "state", state)
		return nil
	}

	d.lWarn("service did not stop gracefully, forcing stop", "name", name, "error", waitErr)
	killOut, killErr := exec.Command("taskkill", "/F", "/FI", fmt.Sprintf("SERVICES eq %s", name)).CombinedOutput()
	if killErr != nil {
		d.lWarn("taskkill fallback failed", "name", name, "error", killErr, "output", string(killOut))
	}

	state, waitErr = d.waitForServiceState(name, []string{"SERVICE_STOPPED", "SERVICE_MISSING"}, forceTimeout, pollInterval)
	if waitErr != nil {
		return fmt.Errorf("failed to stop service %s: %w", name, waitErr)
	}

	d.lWarn("service stopped after force fallback", "name", name, "state", state)
	return nil
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
	const (
		startTimeout = 60 * time.Second
		pollInterval = 2 * time.Second
	)

	d.l("starting service", "name", name)
	outBytes, err := exec.Command(d.nssmPath, "start", name).CombinedOutput()
	out := string(outBytes)

	if err != nil && !strings.Contains(out, "SERVICE_START_PENDING") && !strings.Contains(out, "SERVICE_RUNNING") {
		return fmt.Errorf("nssm start %s: %w (output: %s)", name, err, out)
	}
	if err != nil {
		d.l("service start pending or already running", "name", name, "output", out)
	}

	state, waitErr := d.waitForServiceState(name, []string{"SERVICE_RUNNING"}, startTimeout, pollInterval)
	if waitErr != nil {
		return fmt.Errorf("service %s did not reach running state: %w", name, waitErr)
	}

	d.l("service running", "name", name, "state", state)
	return nil
}

// recycleAppPool recycles an IIS app pool via appcmd.exe.
// This clears cached content and picks up the newly swapped junction files.
func (d *Deployer) recycleAppPool(svc ServiceConfig) error {
	if svc.IISAppPool == "" {
		d.l("no IIS app pool configured, skipping recycle", "name", svc.WindowsServiceName)
		return nil
	}

	appcmd := `C:\Windows\System32\inetsrv\appcmd.exe`
	d.l("recycling IIS app pool", "pool", svc.IISAppPool)

	out, err := exec.Command(appcmd, "recycle", "apppool", svc.IISAppPool).CombinedOutput()
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

func parseServiceState(output string) string {
	up := strings.ToUpper(output)
	for _, state := range []string{
		"SERVICE_RUNNING",
		"SERVICE_STOPPED",
		"SERVICE_START_PENDING",
		"SERVICE_STOP_PENDING",
		"SERVICE_PAUSED",
	} {
		if strings.Contains(up, state) {
			return state
		}
	}
	return ""
}

func isServiceMissingOutput(output string) bool {
	return containsAny(output,
		"Can't open service",
		"does not exist",
		"OpenService()",
		"SERVICE_DOES_NOT_EXIST",
	)
}

func (d *Deployer) queryServiceState(name string) (string, error) {
	out, err := exec.Command(d.nssmPath, "status", name).CombinedOutput()
	text := string(out)
	state := parseServiceState(text)
	if err != nil {
		if isServiceMissingOutput(text) {
			return "SERVICE_MISSING", nil
		}
		if state != "" {
			return state, nil
		}
		return "", fmt.Errorf("nssm status %s: %w (output: %s)", name, err, text)
	}
	if state == "" {
		return "", fmt.Errorf("nssm status %s returned unknown state (output: %s)", name, text)
	}
	return state, nil
}

func (d *Deployer) waitForServiceState(name string, expected []string, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	lastState := ""
	var lastErr error
	for {
		state, err := d.queryServiceState(name)
		lastState = state
		lastErr = err
		if err == nil {
			for _, exp := range expected {
				if state == exp {
					return state, nil
				}
			}
			d.l("waiting for service state", "name", name, "current", state, "expected", strings.Join(expected, ","))
		} else {
			d.lWarn("service status check failed", "name", name, "error", err)
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return "", fmt.Errorf("timeout waiting for %s; last status error: %w", strings.Join(expected, ","), lastErr)
			}
			return "", fmt.Errorf("timeout waiting for %s; last observed state=%s", strings.Join(expected, ","), lastState)
		}
		time.Sleep(interval)
	}
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
	Version   string    `json:"version"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	ModTime   time.Time `json:"mod_time"`
	IsCurrent bool      `json:"is_current"`
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
			Version:   e.Name(),
			Path:      fullPath,
			ModTime:   info.ModTime(),
			SizeBytes: dirSize(fullPath),
			IsCurrent: fullPath == currentTarget,
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
