package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type nssmSetting struct {
	key      string
	value    string
	required bool
}

// ensureServiceByType ensures service by type.
func (d *Deployer) ensureServiceByType(ctx context.Context, svc ServiceConfig, currentDir string) error {
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
		return d.ensureService(ctx, svc, newBin)
	}
}

// ensureService registers the service with NSSM if it does not exist yet,
// or updates the binary path if it already exists.
// This means you never need to manually register services -- the watcher
// handles it on first deploy.
func (d *Deployer) ensureService(ctx context.Context, svc ServiceConfig, binPath string) error {
	existing, err := d.serviceExists(ctx, svc.WindowsServiceName)
	if err != nil {
		return err
	}

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

		settings := []nssmSetting{
			{key: "AppDirectory", value: d.wcfg.InstallDir, required: true},
			{key: "AppParameters", value: svc.StartArguments, required: true},
			{key: "Start", value: "SERVICE_AUTO_START", required: true},
			{key: "AppStdout", value: filepath.Join(logDir, svc.WindowsServiceName+".out.log")},
			{key: "AppStderr", value: filepath.Join(logDir, svc.WindowsServiceName+".err.log")},
			{key: "AppRotateFiles", value: "1"},
			{key: "AppRotateOnline", value: "1"},
			{key: "AppRotateSeconds", value: "86400"},
			{key: "AppRestartDelay", value: "5000"},
		}
		if svc.EnvFile != "" {
			settings = append(settings, nssmSetting{key: "AppEnvironmentExtra", value: "ENV_FILE=" + svc.EnvFile, required: true})
		}

		if err := d.applyNSSMSettings(svc.WindowsServiceName, settings); err != nil {
			return err
		}

		d.l("service installed", "name", svc.WindowsServiceName, "binary", binPath)
	} else {
		// Service exists -- update its executable settings in place.
		d.l("updating service settings", "name", svc.WindowsServiceName, "binary", binPath)
		settings := []nssmSetting{
			{key: "Application", value: binPath, required: true},
			{key: "AppDirectory", value: d.wcfg.InstallDir, required: true},
			{key: "AppParameters", value: svc.StartArguments, required: true},
		}
		if svc.EnvFile != "" {
			settings = append(settings, nssmSetting{key: "AppEnvironmentExtra", value: "ENV_FILE=" + svc.EnvFile, required: true})
		}
		if err := d.applyNSSMSettings(svc.WindowsServiceName, settings); err != nil {
			return err
		}
	}

	return nil
}

// applyNSSMSettings applies nssm settings.
func (d *Deployer) applyNSSMSettings(name string, settings []nssmSetting) error {
	var requiredErrors []error
	for _, setting := range settings {
		out, err := runCommand(d.nssmPath, "set", name, setting.key, setting.value)
		if err == nil {
			continue
		}
		settingErr := fmt.Errorf("nssm set %s %s: %w (output: %s)", name, setting.key, err, strings.TrimSpace(string(out)))
		if setting.required {
			requiredErrors = append(requiredErrors, settingErr)
			continue
		}
		d.lWarn("optional nssm setting failed", "name", name, "key", setting.key, "error", err, "output", strings.TrimSpace(string(out)))
	}
	return errors.Join(requiredErrors...)
}

// serviceExists checks registration through the shared NSSM/SCM status contract.
// Unknown status failures are returned instead of being mistaken for an existing
// service and causing later configuration commands to fail misleadingly.
func (d *Deployer) serviceExists(ctx context.Context, name string) (bool, error) {
	_, err := d.serviceManager.Status(ctx, name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrServiceNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("inspect service %s registration: %w", name, err)
}

// stopServices stops services.
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

// recoverStoppedServices restarts services stopped before a later deployment phase failed.
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

// startServices starts services.
func (d *Deployer) startServices(ctx context.Context, currentDir, phase string) error {
	var startErrors []error
	for _, svc := range d.wcfg.Services {
		if err := d.ensureServiceByType(ctx, svc, currentDir); err != nil {
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
