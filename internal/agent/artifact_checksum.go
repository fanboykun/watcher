package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// normalizeSHA256 returns a manifest checksum as lowercase hex. An optional
// "sha256:" prefix (GitHub's digest format) is accepted; empty means not provided.
func normalizeSHA256(raw string) (string, error) {
	sum := strings.ToLower(strings.TrimSpace(raw))
	sum = strings.TrimPrefix(sum, "sha256:")
	if sum == "" {
		return "", nil
	}
	if decoded, err := hex.DecodeString(sum); err != nil || len(decoded) != sha256.Size {
		return "", fmt.Errorf("invalid sha256 %q in release metadata: want %d hex characters", raw, sha256.Size*2)
	}
	return sum, nil
}

// fileSHA256 hashes a file as lowercase hex.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// verifyArtifactChecksum checks a downloaded artifact against the manifest
// before anything is extracted or any service is stopped. expected must
// already be normalized; empty skips verification but says so in the deploy log.
func (r *RepoWatcher) verifyArtifactChecksum(path, expected string) error {
	if expected == "" {
		r.state.AppendDeployLog("artifact: no sha256 in release metadata; integrity not verified")
		return nil
	}
	actual, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}
	if actual != expected {
		return fmt.Errorf("artifact sha256 mismatch: release metadata expects %s, downloaded file is %s", expected, actual)
	}
	r.state.AppendDeployLog("artifact: sha256 verified " + actual)
	return nil
}
