package agent

import (
	"fmt"
	"net/url"
	"strings"
)

// appcmdPath returns the Windows appcmd executable path.
func appcmdPath() string {
	return `C:\Windows\System32\inetsrv\appcmd.exe`
}

// ensureIISService ensures iis service.
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

// withDefaultIISTargets fills missing IIS app-pool and site names from the service name.
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

// resolvedIISManagedRuntime resolves the effective IIS managed runtime for a service.
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

// runtimeDisplay formats an IIS managed-runtime value for logs.
func runtimeDisplay(runtime string) string {
	if normalizeIISManagedRuntime(runtime) == "" {
		return "No Managed Code"
	}
	return normalizeIISManagedRuntime(runtime)
}

// ensureIISAppPool ensures iis app pool.
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

// ensureIISSite ensures iis site.
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

// iisObjectExists queries appcmd for a named IIS object.
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

// isIISObjectMissingOutput reports whether appcmd output means an IIS object does not exist.
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

// normalizeIISManagedRuntime normalizes iis managed runtime.
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

// setIISAppPoolManagedRuntime sets iis app pool managed runtime.
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

// setIISSitePhysicalPath sets iis site physical path.
func (d *Deployer) setIISSitePhysicalPath(siteName, currentDir string) error {
	d.l("updating IIS site path", "site", siteName, "path", currentDir)
	out, err := runCommand(appcmdPath(), "set", "vdir", siteName+"/", "/physicalPath:"+currentDir)
	if err != nil {
		return fmt.Errorf("set IIS site %s physical path: %w (output: %s)", siteName, err, string(out))
	}
	return nil
}

// setIISSiteAppPool sets iis site app pool.
func (d *Deployer) setIISSiteAppPool(siteName, appPool string) error {
	d.l("assigning IIS app pool", "site", siteName, "pool", appPool)
	out, err := runCommand(appcmdPath(), "set", "app", siteName+"/", "/applicationPool:"+appPool)
	if err != nil {
		return fmt.Errorf("set IIS site %s app pool %s: %w (output: %s)", siteName, appPool, err, string(out))
	}
	return nil
}

// iisBindingFromPublicURL converts an HTTP URL into an appcmd binding expression.
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
