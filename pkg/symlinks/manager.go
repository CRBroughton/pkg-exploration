package symlinks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/crbroughton/pkg-exploration/pkg/config"
)

// SymlinkManager handles symlink operations and cleanup
type SymlinkManager struct {
	baseDir string
}

// NewSymlinkManager creates a new symlink manager
func NewSymlinkManager(baseDir string) *SymlinkManager {
	return &SymlinkManager{
		baseDir: baseDir,
	}
}

// NewDefaultSymlinkManager creates a symlink manager with default base directory
func NewDefaultSymlinkManager() *SymlinkManager {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")
	return NewSymlinkManager(baseDir)
}

// CleanupOrphanedSymlinks removes broken symlinks and container symlinks for inactive containers
func (s *SymlinkManager) CleanupOrphanedSymlinks() error {
	profileBin := filepath.Join(s.baseDir, "profiles", "default", "bin")
	
	// Check if profile bin directory exists
	if _, err := os.Stat(profileBin); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}
	
	// Load current config to know which containers are active
	configPath := filepath.Join(s.baseDir, "config.toml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// If no config, still clean up truly broken symlinks
		cfg = &config.Config{Containers: make(map[string]config.ContainerConfig)}
	}
	
	entries, err := os.ReadDir(profileBin)
	if err != nil {
		return fmt.Errorf("failed to read profile bin directory: %w", err)
	}
	
	removedCount := 0
	for _, entry := range entries {
		if s.shouldRemoveSymlink(entry, cfg, profileBin) {
			symlinkPath := filepath.Join(profileBin, entry.Name())
			fmt.Printf("  🗑️  Removing orphaned symlink: %s\n", entry.Name())
			if err := os.Remove(symlinkPath); err != nil {
				fmt.Printf("     ⚠️  Failed to remove %s: %v\n", entry.Name(), err)
			} else {
				removedCount++
			}
		}
	}
	
	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d orphaned symlinks\n", removedCount)
	}
	
	return nil
}

// shouldRemoveSymlink determines if a symlink should be removed
func (s *SymlinkManager) shouldRemoveSymlink(entry os.DirEntry, cfg *config.Config, profileBin string) bool {
	symlinkPath := filepath.Join(profileBin, entry.Name())
	
	// Check if it's a symlink
	info, err := os.Lstat(symlinkPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	
	// Get the symlink target
	target, err := os.Readlink(symlinkPath)
	if err != nil {
		return false
	}
	
	// Check if the symlink target exists
	if _, err := os.Stat(symlinkPath); os.IsNotExist(err) {
		return true // Broken symlink, remove it
	}
	
	// Check if it points to a container store directory that's no longer active
	storePath := filepath.Join(s.baseDir, "store")
	if !strings.HasPrefix(target, storePath) {
		return false // Not a store symlink
	}
	
	// Extract container name from store path
	// Format: ~/.yourpm/store/containername-version/command
	relativePath, err := filepath.Rel(storePath, target)
	if err != nil {
		return false
	}
	
	parts := strings.Split(relativePath, string(filepath.Separator))
	if len(parts) == 0 {
		return false
	}
	
	// Extract container name from directory name
	storeDirName := parts[0]
	
	// Check if this looks like a container store directory
	// Container stores contain symlinks to container-exec
	containerExecPath := filepath.Join(s.baseDir, "bin", "container-exec")
	targetFilePath := filepath.Join(storePath, storeDirName, parts[len(parts)-1])
	
	linkTarget, err := os.Readlink(targetFilePath)
	if err != nil || linkTarget != containerExecPath {
		return false // Not a container store directory
	}
	
	// This is a container store directory, check if container is still active
	for containerName := range cfg.Containers {
		if strings.HasPrefix(storeDirName, containerName+"-") {
			return false // Container is active, keep symlink
		}
	}
	
	// No matching active container found, should remove
	return true
}

// RemoveSymlink removes a specific symlink
func (s *SymlinkManager) RemoveSymlink(name string) error {
	profileBin := filepath.Join(s.baseDir, "profiles", "default", "bin")
	symlinkPath := filepath.Join(profileBin, name)
	return os.Remove(symlinkPath)
}

// CreateSymlink creates a symlink in the profile bin directory
func (s *SymlinkManager) CreateSymlink(name, target string) error {
	profileBin := filepath.Join(s.baseDir, "profiles", "default", "bin")
	symlinkPath := filepath.Join(profileBin, name)
	
	// Remove existing symlink if it exists
	os.Remove(symlinkPath)
	
	return os.Symlink(target, symlinkPath)
}

// SymlinkExists checks if a symlink exists
func (s *SymlinkManager) SymlinkExists(name string) bool {
	profileBin := filepath.Join(s.baseDir, "profiles", "default", "bin")
	symlinkPath := filepath.Join(profileBin, name)
	
	info, err := os.Lstat(symlinkPath)
	if err != nil {
		return false
	}
	
	return info.Mode()&os.ModeSymlink != 0
}

// ListSymlinks returns all symlinks in the profile bin directory
func (s *SymlinkManager) ListSymlinks() ([]string, error) {
	profileBin := filepath.Join(s.baseDir, "profiles", "default", "bin")
	
	entries, err := os.ReadDir(profileBin)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile bin directory: %w", err)
	}
	
	var symlinks []string
	for _, entry := range entries {
		symlinkPath := filepath.Join(profileBin, entry.Name())
		info, err := os.Lstat(symlinkPath)
		if err != nil {
			continue
		}
		
		if info.Mode()&os.ModeSymlink != 0 {
			symlinks = append(symlinks, entry.Name())
		}
	}
	
	return symlinks, nil
}