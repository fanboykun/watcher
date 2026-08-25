package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const releaseMetadataStoreDir = "release-metadata"

// ReleaseDeploymentMetadata preserves the GitHub identity used by a deployed release.
type ReleaseDeploymentMetadata struct {
	Version     string    `json:"version"`
	Owner       string    `json:"owner"`
	Repository  string    `json:"repository"`
	Ref         string    `json:"ref"`
	ArtifactURL string    `json:"artifact_url,omitempty"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// WriteReleaseDeploymentMetadata atomically records the actual repository and ref.
func WriteReleaseDeploymentMetadata(installDir string, metadata ReleaseDeploymentMetadata) error {
	metadata.Version = strings.TrimSpace(metadata.Version)
	metadata.Owner = strings.TrimSpace(metadata.Owner)
	metadata.Repository = strings.TrimSpace(metadata.Repository)
	metadata.Ref = strings.TrimSpace(metadata.Ref)
	if metadata.Version == "" || metadata.Owner == "" || metadata.Repository == "" || metadata.Ref == "" {
		return errors.New("release deployment metadata is incomplete")
	}
	metadata.RecordedAt = time.Now().UTC()
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release deployment metadata: %w", err)
	}

	target := releaseMetadataPath(installDir, metadata.Version)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return fmt.Errorf("create release metadata store: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".release-metadata-")
	if err != nil {
		return fmt.Errorf("create release metadata file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure release metadata file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write release metadata file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close release metadata file: %w", err)
	}
	if err := replaceFile(temporaryPath, target); err != nil {
		return fmt.Errorf("publish release deployment metadata: %w", err)
	}
	return nil
}

// ReadReleaseDeploymentMetadata loads the GitHub identity for a retained version.
func ReadReleaseDeploymentMetadata(installDir, version string) (*ReleaseDeploymentMetadata, error) {
	data, err := os.ReadFile(releaseMetadataPath(installDir, version))
	if err != nil {
		return nil, err
	}
	var metadata ReleaseDeploymentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("decode release deployment metadata: %w", err)
	}
	if metadata.Version != strings.TrimSpace(version) || metadata.Owner == "" || metadata.Repository == "" || metadata.Ref == "" {
		return nil, fmt.Errorf("invalid release deployment metadata for %s", version)
	}
	return &metadata, nil
}

// DeleteReleaseDeploymentMetadata removes metadata owned by a deleted release.
func DeleteReleaseDeploymentMetadata(installDir, version string) error {
	if err := os.Remove(releaseMetadataPath(installDir, version)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove release deployment metadata for %s: %w", version, err)
	}
	return nil
}

func releaseMetadataPath(installDir, version string) string {
	return filepath.Join(installDir, watcherStateDir, releaseMetadataStoreDir, releaseStorageName(version)+".json")
}

func replaceFile(source, target string) error {
	backup := fmt.Sprintf("%s.backup-%d", target, time.Now().UnixNano())
	hadTarget := false
	if _, err := os.Lstat(target); err == nil {
		hadTarget = true
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if hadTarget {
		return os.Remove(backup)
	}
	return nil
}
