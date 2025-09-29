package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

type GithubRepository struct {
	client    *http.Client
	cacheDir  string
	manifests map[string]*GithubManifest
}

type GithubManifest struct {
	Repo string
}

func (r *GithubRepository) Name() string {
	return "github"
}

func NewGithubRepository(cacheDir string) *GithubRepository {
	return &GithubRepository{
		client:    &http.Client{},
		cacheDir:  cacheDir,
		manifests: loadGitHubManifests(),
	}
}

func (r *GithubRepository) DownloadFile(ctx context.Context, url string, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tempFile := dest + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(tempFile)
		return err
	}

	return os.Rename(tempFile, dest)
}
