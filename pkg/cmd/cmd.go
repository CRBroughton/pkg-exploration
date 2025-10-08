package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/crbroughton/pkg-exploration/pkg/config"
	"github.com/crbroughton/pkg-exploration/pkg/containers"
	"github.com/crbroughton/pkg-exploration/pkg/manifest"
	"github.com/crbroughton/pkg-exploration/pkg/profile"
	"github.com/crbroughton/pkg-exploration/pkg/repository"
	"github.com/crbroughton/pkg-exploration/pkg/store"
)

func Switch(args []string) {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")

	manifestPath := filepath.Join(baseDir, "manifest.toml")
	mfst, err := manifest.LoadManifest(manifestPath)
	if err != nil {
		log.Fatalf("Failed to load manifest: %v\n", err)
		log.Fatalf("Make sure %s exists", manifestPath)
	}

	// Load config (what user wants)
	// Default to ~/.yourpm/config.toml, but allow override
	configPath := filepath.Join(baseDir, "config.toml")
	if len(args) > 0 {
		configPath = args[0]
		// Make path absolute if it's relative
		if !filepath.IsAbs(configPath) {
			pwd, _ := os.Getwd()
			configPath = filepath.Join(pwd, configPath)
		}
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", configPath, err)
	}

	// If using a custom config file, copy it to the default location
	// so container-exec can find it
	defaultConfigPath := filepath.Join(baseDir, "config.toml")
	if len(args) > 0 && configPath != defaultConfigPath {
		if err := copyFile(configPath, defaultConfigPath); err != nil {
			log.Fatalf("Failed to copy config to default location: %v", err)
		}
		fmt.Printf("Copied config from %s to %s\n", configPath, defaultConfigPath)
	}

	fmt.Printf("Loading config from: %s\n", configPath)
	fmt.Printf("Applying environment: %s\n", cfg.Name)
	fmt.Printf("Packages to install: %d\n", len(cfg.Packages))
	fmt.Printf("Containers to setup: %d\n\n", len(cfg.Containers))

	ctx := context.Background()
	repo := repository.NewHttpRepository(filepath.Join(baseDir, "cache"))
	st := store.NewStore(filepath.Join(baseDir, "store"))
	prof := profile.NewProfile(filepath.Join(baseDir, "profiles", "default"))

	installedPaths := make(map[string]string)

	// Install each package
	for name, version := range cfg.Packages {
		fmt.Printf("📦 %s@%s\n", name, version)

		url, err := mfst.GetURL(name, version)
		if err != nil {
			log.Fatalf("  ✗ Failed to get URL: %v", err)
		}

		pkgDef, _ := mfst.GetPackage(name)

		// Start the download
		filename := filepath.Base(url)
		cachePath := filepath.Join(baseDir, "cache", fmt.Sprintf("%s-%s-%s", name, version, filename))

		if err := repo.DownloadFile(ctx, url, cachePath); err != nil {
			log.Fatalf("  ✗ Download failed: %v", err)
		}
		fmt.Printf("  ✓ Downloaded\n")

		// Install - pass binary names so it knows what to search for
		storePath, err := st.Install(name, version, cachePath, pkgDef.Binaries.Names)
		if err != nil {
			log.Fatalf("  ✗ Install failed: %v", err)
		}
		fmt.Printf("  ✓ Installed\n")

		installedPaths[name] = storePath

		// Do the symlinking stuff
		if err := prof.Link(storePath, pkgDef.Binaries.Names); err != nil {
			log.Fatalf("  ✗ Link failed: %v", err)
		}
		fmt.Printf("  ✓ Linked\n\n")
	}

	// Handle containers
	if len(cfg.Containers) > 0 {
		// Load container manifest
		containerManifestPath := filepath.Join(baseDir, "containers.toml")
		containerMfst, err := containers.LoadContainerManifest(containerManifestPath)
		if err != nil {
			log.Fatalf("Failed to load container manifest from %s: %v", containerManifestPath, err)
		}

		// Install each container
		for name, containerCfg := range cfg.Containers {
			fmt.Printf("🐳 %s@%s\n", name, containerCfg.Version)

			containerDef, err := containerMfst.GetContainer(name)
			if err != nil {
				log.Fatalf("  ✗ Container not found in manifest: %v", err)
			}

			// Create container store path
			containerStorePath := filepath.Join(baseDir, "store", fmt.Sprintf("%s-%s", name, containerCfg.Version))
			
			if err := os.MkdirAll(containerStorePath, 0755); err != nil {
				log.Fatalf("  ✗ Failed to create container store path: %v", err)
			}

			// Build the container executor if it doesn't exist
			execPath := filepath.Join(baseDir, "bin", "container-exec")
			if _, err := os.Stat(execPath); os.IsNotExist(err) {
				if err := buildContainerExec(execPath); err != nil {
					log.Fatalf("  ✗ Failed to build container executor: %v", err)
				}
			}

			// Create symlinks to the container executor for each command
			for _, command := range containerDef.Commands {
				symlinkPath := filepath.Join(containerStorePath, command)
				
				// Remove existing file/symlink
				os.Remove(symlinkPath)
				
				// Create symlink to container executor
				if err := os.Symlink(execPath, symlinkPath); err != nil {
					log.Fatalf("  ✗ Failed to create symlink for %s: %v", command, err)
				}
			}

			// Link the commands
			if err := prof.Link(containerStorePath, containerDef.Commands); err != nil {
				log.Fatalf("  ✗ Link failed: %v", err)
			}
			
			// Check and update container if version changed
			if err := ensureContainerVersion(name, containerCfg, containerDef); err != nil {
				log.Fatalf("  ✗ Failed to ensure container version: %v", err)
			}
			
			// Ensure Docker image is available (pull if needed)
			image := fmt.Sprintf("%s:%s", containerCfg.Image, containerCfg.Version)
			if err := ensureDockerImage(image); err != nil {
				log.Fatalf("  ✗ Failed to ensure Docker image: %v", err)
			}
			
			fmt.Printf("  ✓ Container setup complete\n\n")
		}
	}

	profileBin := filepath.Join(baseDir, "profiles", "default", "bin")
	fmt.Printf("✓ Environment '%s' is now active\n\n", cfg.Name)
	fmt.Printf("Ensure this is in your PATH:\n")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", profileBin)
}

func buildContainerExec(outputPath string) error {
	// Create bin directory
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	// Build the container executor
	cmd := exec.CommandContext(context.Background(), "go", "build", 
		"-o", outputPath, 
		"./cmd/container-exec")
	
	return cmd.Run()
}

func ensureContainerVersion(containerName string, containerCfg config.ContainerConfig, _ *containers.ContainerDefinition) error {
	containerFullName := fmt.Sprintf("yourpm-%s", containerName)
	desiredImage := fmt.Sprintf("%s:%s", containerCfg.Image, containerCfg.Version)
	
	// Check if container exists
	if !containerExists(containerFullName) {
		// Container doesn't exist, no need to update
		return nil
	}
	
	// Get current container image
	currentImage, err := getContainerImage(containerFullName)
	if err != nil {
		// Container might not exist anymore, ignore error
		return nil
	}
	
	// If image changed, remove old container
	if currentImage != desiredImage {
		fmt.Printf("  📦 Updating container from %s to %s\n", currentImage, desiredImage)
		if err := removeContainer(containerFullName); err != nil {
			return fmt.Errorf("failed to remove old container: %w", err)
		}
	}
	
	return nil
}

func containerExists(containerName string) bool {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), containerName)
}

func getContainerImage(containerName string) (string, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.Config.Image}}", containerName)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func removeContainer(containerName string) error {
	cmd := exec.Command("docker", "rm", "-f", containerName)
	return cmd.Run()
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func ensureDockerImage(image string) error {
	// Check if image exists locally first
	if imageExistsLocally(image) {
		fmt.Printf("  ✓ Docker image %s already available\n", image)
		return nil
	}
	
	// Image doesn't exist locally, pull it
	fmt.Printf("  📥 Pulling Docker image %s...\n", image)
	cmd := exec.Command("docker", "pull", image)
	// Show docker pull output to user
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func imageExistsLocally(image string) bool {
	cmd := exec.Command("docker", "image", "inspect", image)
	err := cmd.Run()
	return err == nil
}

func PruneContainers(args []string) {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")
	
	// Load current config to determine which containers to keep
	configPath := filepath.Join(baseDir, "config.toml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Printf("Warning: Could not load config from %s: %v", configPath, err)
		log.Printf("Proceeding with prune, but will not protect active containers")
	}
	
	// Check for --all flag
	aggressive := false
	for _, arg := range args {
		if arg == "--all" {
			aggressive = true
			break
		}
	}
	
	fmt.Printf("🐳 Pruning containers...\n\n")
	
	if cfg != nil {
		// Stop and remove containers that are not in current config
		if err := pruneContainers(cfg, aggressive); err != nil {
			log.Printf("Error: Failed to prune containers: %v", err)
		}
	} else {
		// No config available, can only do aggressive cleanup
		if aggressive {
			if err := pruneAllYourpmContainers(); err != nil {
				log.Printf("Error: Failed to prune containers: %v", err)
			}
		} else {
			fmt.Printf("  ⚠️  No config found, use --all to remove all yourpm containers\n")
		}
	}
	
	fmt.Printf("✓ Container cleanup complete\n")
}

func PruneImages(args []string) {
	// Check for --all flag
	aggressive := false
	for _, arg := range args {
		if arg == "--all" {
			aggressive = true
			break
		}
	}
	
	fmt.Printf("🖼️  Pruning images...\n\n")
	
	// Clean up unused images
	if err := pruneImages(aggressive); err != nil {
		log.Printf("Error: Failed to prune images: %v", err)
	}
	
	fmt.Printf("✓ Image cleanup complete\n")
}

func pruneContainers(cfg *config.Config, aggressive bool) error {
	// Get all yourpm containers
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name=yourpm-", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	if strings.TrimSpace(string(output)) == "" {
		fmt.Printf("  ✓ No yourpm containers found\n")
		// Still run cleanup even if no containers to remove
		if err := cleanupOrphanedSymlinks(); err != nil {
			fmt.Printf("     ⚠️  Failed to cleanup orphaned symlinks: %v\n", err)
		}
		return nil
	}
	
	containerNames := strings.Split(strings.TrimSpace(string(output)), "\n")
	activeContainers := make(map[string]bool)
	
	// Build map of containers that should be kept (from current config)
	for containerName := range cfg.Containers {
		activeContainers[fmt.Sprintf("yourpm-%s", containerName)] = true
	}
	
	removedCount := 0
	for _, containerName := range containerNames {
		containerName = strings.TrimSpace(containerName)
		if containerName == "" {
			continue
		}
		
		if !aggressive && activeContainers[containerName] {
			fmt.Printf("  ✓ Keeping active container: %s\n", containerName)
			continue
		}
		
		fmt.Printf("  🗑️  Removing container: %s\n", containerName)
		removeCmd := exec.Command("docker", "rm", "-f", containerName)
		if err := removeCmd.Run(); err != nil {
			fmt.Printf("     ⚠️  Failed to remove %s: %v\n", containerName, err)
		} else {
			removedCount++
		}
	}
	
	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d containers\n", removedCount)
	} else {
		fmt.Printf("  ✓ No containers to remove\n")
	}
	
	// Clean up orphaned symlinks
	if err := cleanupOrphanedSymlinks(); err != nil {
		fmt.Printf("     ⚠️  Failed to cleanup orphaned symlinks: %v\n", err)
	}
	
	return nil
}

func pruneAllYourpmContainers() error {
	// Get all yourpm containers
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name=yourpm-", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}
	
	if strings.TrimSpace(string(output)) == "" {
		fmt.Printf("  ✓ No yourpm containers found\n")
		// Still run cleanup even if no containers to remove
		if err := cleanupOrphanedSymlinks(); err != nil {
			fmt.Printf("     ⚠️  Failed to cleanup orphaned symlinks: %v\n", err)
		}
		return nil
	}
	
	containerNames := strings.Split(strings.TrimSpace(string(output)), "\n")
	removedCount := 0
	
	for _, containerName := range containerNames {
		containerName = strings.TrimSpace(containerName)
		if containerName == "" {
			continue
		}
		
		fmt.Printf("  🗑️  Removing container: %s\n", containerName)
		removeCmd := exec.Command("docker", "rm", "-f", containerName)
		if err := removeCmd.Run(); err != nil {
			fmt.Printf("     ⚠️  Failed to remove %s: %v\n", containerName, err)
		} else {
			removedCount++
		}
	}
	
	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d containers\n", removedCount)
	} else {
		fmt.Printf("  ✓ No containers to remove\n")
	}
	
	// Clean up orphaned symlinks
	if err := cleanupOrphanedSymlinks(); err != nil {
		fmt.Printf("     ⚠️  Failed to cleanup orphaned symlinks: %v\n", err)
	}
	
	return nil
}

func pruneImages(aggressive bool) error {
	if aggressive {
		fmt.Printf("  🗑️  Removing all unused images...\n")
		// Remove all unused images
		cmd := exec.Command("docker", "image", "prune", "-a", "-f")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	} else {
		fmt.Printf("  🗑️  Removing dangling images...\n")
		// Remove only dangling images
		cmd := exec.Command("docker", "image", "prune", "-f")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
}

func cleanupOrphanedSymlinks() error {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")
	profileBin := filepath.Join(baseDir, "profiles", "default", "bin")
	
	// Check if profile bin directory exists
	if _, err := os.Stat(profileBin); os.IsNotExist(err) {
		return nil // Nothing to clean up
	}
	
	// Load current config to know which containers are active
	configPath := filepath.Join(baseDir, "config.toml")
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
		symlinkPath := filepath.Join(profileBin, entry.Name())
		
		// Check if it's a symlink
		info, err := os.Lstat(symlinkPath)
		if err != nil {
			continue
		}
		
		if info.Mode()&os.ModeSymlink != 0 {
			// Get the symlink target
			target, err := os.Readlink(symlinkPath)
			if err != nil {
				continue
			}
			
			shouldRemove := false
			
			// Check if the symlink target exists
			if _, err := os.Stat(symlinkPath); os.IsNotExist(err) {
				shouldRemove = true
			} else {
				// Check if it points to a container store directory that's no longer active
				storePath := filepath.Join(baseDir, "store")
				if strings.HasPrefix(target, storePath) {
					// Extract container name from store path
					// Format: ~/.yourpm/store/containername-version/command
					relativePath, err := filepath.Rel(storePath, target)
					if err == nil {
						parts := strings.Split(relativePath, string(filepath.Separator))
						if len(parts) > 0 {
							// Extract container name from directory name
							storeDirName := parts[0]
							
							// Check if this looks like a container store directory
							// Container stores contain symlinks to container-exec
							containerExecPath := filepath.Join(baseDir, "bin", "container-exec")
							targetFilePath := filepath.Join(storePath, storeDirName, parts[len(parts)-1])
							if linkTarget, err := os.Readlink(targetFilePath); err == nil && linkTarget == containerExecPath {
								// This is a container store directory, check if container is still active
								for containerName := range cfg.Containers {
									if strings.HasPrefix(storeDirName, containerName+"-") {
										goto keepSymlink // Container is active, keep symlink
									}
								}
								// No matching active container found, should remove
								shouldRemove = true
							}
						}
					}
				}
			}
			
			if shouldRemove {
				fmt.Printf("  🗑️  Removing orphaned symlink: %s\n", entry.Name())
				if err := os.Remove(symlinkPath); err != nil {
					fmt.Printf("     ⚠️  Failed to remove %s: %v\n", entry.Name(), err)
				} else {
					removedCount++
				}
			}
		}
		
		keepSymlink:
	}
	
	if removedCount > 0 {
		fmt.Printf("  ✓ Removed %d orphaned symlinks\n", removedCount)
	}
	
	return nil
}

