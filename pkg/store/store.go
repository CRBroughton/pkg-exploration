package store

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{
		root: root,
	}
}

func (s *Store) Install(name string, version string, downloadPath string, binaryNames []string) (string, error) {
	storePath := filepath.Join(s.root, fmt.Sprintf("%s-%s", name, version))
	if _, err := os.Stat(storePath); err == nil {
		return storePath, nil
	}

	extension := filepath.Ext(downloadPath)
	switch {
	case strings.HasSuffix(downloadPath, ".tar.gz") || extension == ".tgz":
		return s.installTarGz(downloadPath, storePath, binaryNames)
	default:
		return s.installBinary(name, downloadPath, storePath)
	}
}

func (s *Store) installBinary(name string, downloadPath string, storePath string) (string, error) {
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return "", err
	}

	destPath := filepath.Join(storePath, name)
	if err := copyFile(downloadPath, destPath); err != nil {
		return "", err
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return "", err
	}

	return storePath, nil
}

func (s *Store) installTarGz(downloadPath string, storePath string, binaryNames []string) (string, error) {
	tempDir := storePath + ".tmp"
	if err := os.RemoveAll(tempDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDir)

	if err := s.extractTarGz(downloadPath, tempDir); err != nil {
		return "", err
	}

	if err := os.MkdirAll(storePath, 0755); err != nil {
		return "", err
	}

	for _, binaryName := range binaryNames {
		found, err := s.findAndMoveBinary(tempDir, storePath, binaryName)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("binary %s not found in archive", binaryName)
		}
	}

	return storePath, nil
}

func (s *Store) extractTarGz(downloadPath string, destDir string) error {
	file, err := os.Open(downloadPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}

			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

// findAndMoveBinary searches the temp directory tree for the binary and moves it to store root
func (s *Store) findAndMoveBinary(tempDir string, storePath string, binaryName string) (bool, error) {
	var foundPath string

	// Walk the temp directory tree
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if this file matches our binary name
		if filepath.Base(path) == binaryName {
			foundPath = path
			return filepath.SkipAll // Stop walking once found
		}

		return nil
	})

	if err != nil {
		return false, err
	}

	if foundPath == "" {
		return false, nil
	}

	destPath := filepath.Join(storePath, binaryName)
	if err := os.Rename(foundPath, destPath); err != nil {
		if err := copyFile(foundPath, destPath); err != nil {
			return false, err
		}
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return false, err
	}

	return true, nil
}

func copyFile(src string, dest string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	fileDest, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer fileDest.Close()

	_, err = io.Copy(fileDest, source)
	return err
}
