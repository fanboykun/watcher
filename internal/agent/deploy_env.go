package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type deploymentEnvFile struct {
	path    string
	content []byte
	mode    os.FileMode
	existed bool
	next    string
	remove  bool
}

// prepareDeploymentEnv validates all paths and preserves live contents before
// shutdown. No files are changed until the replacement release is activated.
func (d *Deployer) prepareDeploymentEnv() ([]deploymentEnvFile, error) {
	var files []deploymentEnvFile
	nextFiles := make(map[string]deploymentEnvFile)
	add := func(svc ServiceConfig, next bool) error {
		configs := append([]ConfigFile(nil), svc.ConfigFiles...)
		if strings.TrimSpace(svc.EnvFile) != "" {
			configs = append(configs, ConfigFile{FilePath: svc.EnvFile, Target: "app_dir", Content: svc.EnvContent})
		}
		for _, file := range configs {
			if normalizeConfigFileTarget(file.Target) != "app_dir" {
				continue
			}
			target, err := managedFilePath(d.wcfg.InstallDir, file.FilePath)
			if err != nil {
				return err
			}
			// Windows file identities are case-insensitive. Keep the new path
			// spelling for writes, but retire only targets absent from the new set.
			key := strings.ToLower(filepath.Clean(target))
			if next {
				nextFiles[key] = deploymentEnvFile{path: target, next: file.Content, mode: 0600}
			} else if _, ok := nextFiles[key]; !ok {
				nextFiles[key] = deploymentEnvFile{path: target, mode: 0600}
			}
		}
		return nil
	}
	for _, svc := range d.wcfg.Services {
		if err := add(svc, true); err != nil {
			return nil, err
		}
	}
	activeTargets := make(map[string]bool, len(nextFiles))
	for target := range nextFiles {
		activeTargets[target] = true
	}
	for _, svc := range d.previousServices {
		if err := add(svc, false); err != nil {
			return nil, err
		}
	}
	for key, file := range nextFiles {
		file.remove = !activeTargets[key]
		target := file.path
		info, err := os.Stat(target)
		if err == nil {
			file.existed = true
			file.mode = info.Mode().Perm()
			file.content, err = os.ReadFile(target)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("preserve managed config %s: %w", target, err)
		}
		files = append(files, file)
	}
	return files, nil
}

func restoreDeploymentEnv(files []deploymentEnvFile, cause error) error {
	for _, file := range files {
		var err error
		if file.existed {
			err = os.WriteFile(file.path, file.content, file.mode)
		} else {
			err = os.Remove(file.path)
			if os.IsNotExist(err) {
				err = nil
			}
		}
		if err != nil {
			cause = errors.Join(cause, fmt.Errorf("restore environment %s: %w", file.path, err))
		}
	}
	return cause
}

func applyDeploymentEnv(files []deploymentEnvFile) error {
	for _, file := range files {
		if file.remove {
			if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(file.path, []byte(file.next), 0600); err != nil {
			return err
		}
	}
	return nil
}
