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
}

// prepareDeploymentEnv validates all paths and preserves live contents before
// shutdown. No files are changed until the replacement release is activated.
func (d *Deployer) prepareDeploymentEnv() ([]deploymentEnvFile, error) {
	var files []deploymentEnvFile
	seen := make(map[string]string)
	for _, svc := range d.wcfg.Services {
		if strings.TrimSpace(svc.EnvFile) == "" {
			continue
		}
		path, err := managedFilePath(d.wcfg.InstallDir, svc.EnvFile)
		if err != nil {
			return nil, err
		}
		if content, ok := seen[path]; ok {
			if content != svc.EnvContent {
				return nil, fmt.Errorf("conflicting environments for %s", svc.EnvFile)
			}
			continue
		}
		seen[path] = svc.EnvContent
		file := deploymentEnvFile{path: path, next: svc.EnvContent, mode: 0600}
		info, err := os.Stat(path)
		if err == nil {
			file.existed = true
			file.mode = info.Mode().Perm()
			file.content, err = os.ReadFile(path)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("preserve environment %s: %w", svc.EnvFile, err)
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
		if err := os.MkdirAll(filepath.Dir(file.path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(file.path, []byte(file.next), 0600); err != nil {
			return err
		}
	}
	return nil
}
