package agent

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// writeReleaseConfigFiles writes configuration whose target is the staged release.
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

// normalizeConfigFileTarget normalizes a configured file target to a supported location.
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

// writeManagedFile writes a managed configuration file without allowing root traversal.
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

// managedFilePath resolves and validates a managed file path beneath its configured root.
func managedFilePath(rootDir, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", errors.New("managed file path is empty")
	}
	normalized, err := normalizeManagedRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	relativePath = filepath.FromSlash(normalized)
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
	// Existing symlinks beneath the root could redirect a safe relative path.
	for candidate := target; candidate != root; candidate = filepath.Dir(candidate) {
		info, statErr := os.Lstat(candidate)
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("managed file path %q traverses a symlink", relativePath)
		}
	}
	return target, nil
}

// normalizeManagedRelativePath accepts Windows or URL separators while rejecting
// absolute paths, traversal, Windows drive/ADS paths, and agent-private state.
func normalizeManagedRelativePath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\x00") {
		return "", fmt.Errorf("invalid managed file path %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", fmt.Errorf("managed file path %q escapes root", value)
		}
	}
	value = path.Clean(value)
	if value == "." || strings.EqualFold(strings.Split(value, "/")[0], watcherStateDir) {
		return "", fmt.Errorf("invalid managed file path %q", value)
	}
	return value, nil
}

// NormalizeManagedConfigFiles validates one service's staged file set in place.
func NormalizeManagedConfigFiles(envFile string, files []ConfigFile) error {
	seen := make(map[string]bool)
	if strings.TrimSpace(envFile) != "" {
		normalized, err := normalizeManagedRelativePath(envFile)
		if err != nil {
			return err
		}
		seen["app_dir/"+strings.ToLower(normalized)] = true
	}
	for i := range files {
		file := &files[i]
		target := strings.ToLower(strings.TrimSpace(file.Target))
		switch target {
		case "", "app", "app_dir", "install_dir", "release", "release_dir", "current":
		default:
			return fmt.Errorf("invalid managed file target %q", file.Target)
		}
		file.Target = normalizeConfigFileTarget(target)
		normalized, err := normalizeManagedRelativePath(file.FilePath)
		if err != nil {
			return err
		}
		file.FilePath = normalized
		if file.Target == "app_dir" {
			switch strings.ToLower(strings.Split(normalized, "/")[0]) {
			case "releases", "current", "downloads":
				return fmt.Errorf("app config path %q overlaps release storage", normalized)
			}
		}
		key := file.Target + "/" + strings.ToLower(normalized)
		if seen[key] {
			return fmt.Errorf("duplicate managed file target %s", key)
		}
		for existing := range seen {
			if managedPathsOverlap(existing, key) {
				return fmt.Errorf("managed file targets %s and %s overlap", existing, key)
			}
		}
		seen[key] = true
	}
	return nil
}

func validateDeploymentManagedFiles(services []ServiceConfig) error {
	seen := make(map[string]string)
	for _, svc := range services {
		if err := NormalizeManagedConfigFiles(svc.EnvFile, svc.ConfigFiles); err != nil {
			return err
		}
		files := append([]ConfigFile(nil), svc.ConfigFiles...)
		if strings.TrimSpace(svc.EnvFile) != "" {
			files = append(files, ConfigFile{FilePath: svc.EnvFile, Target: "app_dir", Content: svc.EnvContent})
		}
		for _, file := range files {
			normalized, err := normalizeManagedRelativePath(file.FilePath)
			if err != nil {
				return err
			}
			key := normalizeConfigFileTarget(file.Target) + "/" + strings.ToLower(normalized)
			for existing := range seen {
				if managedPathsOverlap(existing, key) {
					return fmt.Errorf("managed file targets %s and %s overlap", existing, key)
				}
			}
			if content, ok := seen[key]; ok && content != file.Content {
				return fmt.Errorf("conflicting managed files for %s", key)
			}
			seen[key] = file.Content
		}
	}
	return nil
}

func managedPathsOverlap(left, right string) bool {
	return strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}
