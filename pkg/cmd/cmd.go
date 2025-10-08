package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/crbroughton/pkg-exploration/pkg/config"
	"github.com/crbroughton/pkg-exploration/pkg/containers"
	"github.com/crbroughton/pkg-exploration/pkg/docker"
	"github.com/crbroughton/pkg-exploration/pkg/manifest"
	"github.com/crbroughton/pkg-exploration/pkg/profile"
	"github.com/crbroughton/pkg-exploration/pkg/prune"
	"github.com/crbroughton/pkg-exploration/pkg/repository"
	"github.com/crbroughton/pkg-exploration/pkg/store"
	"github.com/crbroughton/pkg-exploration/pkg/symlinks"
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
		// Create Docker client
		dockerClient := docker.NewDefaultDockerClient()
		
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
			if err := ensureContainerVersion(dockerClient, name, containerCfg, containerDef); err != nil {
				log.Fatalf("  ✗ Failed to ensure container version: %v", err)
			}
			
			// Ensure Docker image is available (pull if needed)
			image := fmt.Sprintf("%s:%s", containerCfg.Image, containerCfg.Version)
			if err := ensureDockerImage(dockerClient, image); err != nil {
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

func ensureContainerVersion(dockerClient docker.DockerClient, containerName string, containerCfg config.ContainerConfig, _ *containers.ContainerDefinition) error {
	containerFullName := fmt.Sprintf("yourpm-%s", containerName)
	desiredImage := fmt.Sprintf("%s:%s", containerCfg.Image, containerCfg.Version)
	
	// Check if container exists
	if !dockerClient.Exists(containerFullName) {
		// Container doesn't exist, no need to update
		return nil
	}
	
	// Get current container image
	currentImage, err := dockerClient.GetContainerImage(containerFullName)
	if err != nil {
		// Container might not exist anymore, ignore error
		return nil
	}
	
	// If image changed, remove old container
	if currentImage != desiredImage {
		fmt.Printf("  📦 Updating container from %s to %s\n", currentImage, desiredImage)
		if err := dockerClient.Remove(containerFullName); err != nil {
			return fmt.Errorf("failed to remove old container: %w", err)
		}
	}
	
	return nil
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

func ensureDockerImage(dockerClient docker.DockerClient, image string) error {
	// Check if image exists locally first
	if dockerClient.ImageExists(image) {
		fmt.Printf("  ✓ Docker image %s already available\n", image)
		return nil
	}
	
	// Image doesn't exist locally, pull it
	fmt.Printf("  📥 Pulling Docker image %s...\n", image)
	return dockerClient.PullImage(image)
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
	
	pruneService := prune.NewDefaultPruneService()
	
	if cfg != nil && !aggressive {
		// Use selective pruning based on config
		opts := prune.PruneContainersOptions{
			Aggressive: false,
			Config:     cfg,
		}
		if err := pruneService.PruneContainers(opts); err != nil {
			log.Printf("Error: Failed to prune containers: %v", err)
		}
	} else if aggressive {
		// Remove all containers
		if err := pruneService.PruneAllContainers(); err != nil {
			log.Printf("Error: Failed to prune containers: %v", err)
		}
	} else {
		// No config available, can't do selective pruning
		fmt.Printf("  ⚠️  No config found, use --all to remove all yourpm containers\n")
	}
	
	// Clean up orphaned symlinks
	symlinkManager := symlinks.NewDefaultSymlinkManager()
	if err := symlinkManager.CleanupOrphanedSymlinks(); err != nil {
		fmt.Printf("     ⚠️  Failed to cleanup orphaned symlinks: %v\n", err)
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
	
	pruneService := prune.NewDefaultPruneService()
	
	// Clean up unused images
	if err := pruneService.PruneImages(aggressive); err != nil {
		log.Printf("Error: Failed to prune images: %v", err)
	}
	
	fmt.Printf("✓ Image cleanup complete\n")
}




