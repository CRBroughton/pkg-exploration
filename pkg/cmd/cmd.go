package cmd

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

	profileBin := filepath.Join(baseDir, "profiles", "default", "bin")
	fmt.Printf("✓ Environment '%s' is now active\n\n", cfg.Name)
	fmt.Printf("Ensure this is in your PATH:\n")
	fmt.Printf("  export PATH=\"%s:$PATH\"\n", profileBin)
}
