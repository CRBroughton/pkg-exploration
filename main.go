package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/crbroughton/pkg-exploration/pkg/repository"
	"github.com/crbroughton/pkg-exploration/pkg/repository/profile"
	"github.com/crbroughton/pkg-exploration/pkg/repository/store"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: yourpm install <name> <version> <url>")
		fmt.Println("Example: yourpm install jq 1.7.1 https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64")
		os.Exit(1)
	}

	name := os.Args[2]
	version := os.Args[3]
	url := os.Args[4]

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
