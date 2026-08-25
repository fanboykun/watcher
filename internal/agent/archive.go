package agent

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// extractZip extracts zip.
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

// extractZipFile extracts zip file.
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
