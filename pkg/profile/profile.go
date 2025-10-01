package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

type Profile struct {
	root string
}

func NewProfile(root string) *Profile {
	return &Profile{
		root: root,
	}
}

func (p *Profile) Link(storePath string, binaries []string) error {
	binDir := filepath.Join(p.root, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	for _, binary := range binaries {
		// Binary is always at: storePath/binary
		source := filepath.Join(storePath, binary)
		target := filepath.Join(binDir, binary)

		// Remove existing symlink
		os.Remove(target)

		// Create symlink
		if err := os.Symlink(source, target); err != nil {
			return fmt.Errorf("failed to link %s: %w", binary, err)
		}
	}

	return nil
}
