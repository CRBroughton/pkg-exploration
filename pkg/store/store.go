package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{
		root: root,
	}
}

func (s *Store) InstallBinary(name string, version string, downloadPath string) (string, error) {
	storePath := filepath.Join(s.root, fmt.Sprintf("%s-%s", name, version))
	binDir := filepath.Join(storePath, "bin")

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", err
	}

	destPath := filepath.Join(binDir, name)
	if err := copyFile(downloadPath, destPath); err != nil {
		return "", err
	}

	if err := os.Chmod(destPath, 0755); err != nil {
		return "", err
	}

	return storePath, nil
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

	_, err = io.Copy(fileDest, source)
	return err
}
