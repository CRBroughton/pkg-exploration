package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/crbroughton/pkg-exploration/pkg/config"
	"github.com/crbroughton/pkg-exploration/pkg/manifest"
	"github.com/crbroughton/pkg-exploration/pkg/profile"
	"github.com/crbroughton/pkg-exploration/pkg/repository"
	"github.com/crbroughton/pkg-exploration/pkg/store"
)

func cmdInstall(args []string) {
	if len(args) < 3 {
		log.Fatal("Usage: yourpm install <name> <version> <url>")
	}

	name := args[0]
	version := args[1]
	url := args[2]

	// Setup paths
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")

	ctx := context.Background()

	// Download
	fmt.Printf("Downloading %s@%s...\n", name, version)
	repo := repository.NewGithubRepository(filepath.Join(baseDir, "cache"))

	filename := filepath.Base(url)
	cachePath := filepath.Join(baseDir, "cache", fmt.Sprintf("%s-%s-%s", name, version, filename))

	if err := repo.DownloadFile(ctx, url, cachePath); err != nil {
		log.Fatalf("Download failed: %v", err)
	}

	// Install to store
	fmt.Printf("Installing...\n")
	st := store.NewStore(filepath.Join(baseDir, "store"))
	storePath, err := st.InstallBinary(name, version, cachePath)
	if err != nil {
		log.Fatalf("Install failed: %v", err)
	}

	// Create symlink
	fmt.Printf("Linking...\n")
	prof := profile.NewProfile(filepath.Join(baseDir, "profiles", "default"))
	if err := prof.Link(storePath, []string{name}); err != nil {
		log.Fatalf("Link failed: %v", err)
	}

	profileBin := filepath.Join(baseDir, "profiles", "default", "bin")
	fmt.Printf("\n✓ Installed %s@%s\n", name, version)
	fmt.Printf("\nAdd to PATH:\n")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", profileBin)
}

func cmdSwitch(args []string) {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".yourpm")

	// Load manifest (what's available)
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

	fmt.Printf("Loading config from: %s\n", configPath)
	fmt.Printf("Applying environment: %s\n", cfg.Name)
	fmt.Printf("Packages to install: %d\n\n", len(cfg.Packages))

	ctx := context.Background()
	repo := repository.NewGithubRepository(filepath.Join(baseDir, "cache"))
	st := store.NewStore(filepath.Join(baseDir, "store"))
	prof := profile.NewProfile(filepath.Join(baseDir, "profiles", "default"))

	installedPaths := make(map[string]string)

	// Install each package
	for name, version := range cfg.Packages {
		fmt.Printf("📦 %s@%s\n", name, version)

		// Get URL from manifest
		url, err := mfst.GetURL(name, version)
		if err != nil {
			log.Fatalf("  ✗ Failed to get URL: %v", err)
		}

		// Get package definition for binaries
		pkgDef, _ := mfst.GetPackage(name)

		// Download
		filename := filepath.Base(url)
		cachePath := filepath.Join(baseDir, "cache", fmt.Sprintf("%s-%s-%s", name, version, filename))

		if err := repo.DownloadFile(ctx, url, cachePath); err != nil {
			log.Fatalf("  ✗ Download failed: %v", err)
		}
		fmt.Printf("  ✓ Downloaded\n")

		// Install
		storePath, err := st.InstallBinary(name, version, cachePath)
		if err != nil {
			log.Fatalf("  ✗ Install failed: %v", err)
		}
		fmt.Printf("  ✓ Installed\n")

		installedPaths[name] = storePath

		// Link
		if err := prof.Link(storePath, pkgDef.Binaries.Names); err != nil {
			log.Fatalf("  ✗ Link failed: %v", err)
		}
		fmt.Printf("  ✓ Linked\n\n")
	}

	profileBin := filepath.Join(baseDir, "profiles", "default", "bin")
	fmt.Printf("✓ Environment '%s' is now active\n\n", cfg.Name)
	fmt.Printf("Ensure this is in your PATH:\n")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", profileBin)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "install":
		cmdInstall(os.Args[2:])
	case "switch":
		cmdSwitch(os.Args[2:])
	default:
		log.Fatalf("Unknown command: %s", command)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  yourpm install <name> <version> <url>")
	fmt.Println("  yourpm switch [config-file]")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  yourpm install jq 1.7.1 https://github.com/.../jq-linux-amd64")
	fmt.Println("  yourpm switch ~/.yourpm/config.toml")
	fmt.Println("  yourpm switch  # Uses ~/.yourpm/config.toml by default")
}
