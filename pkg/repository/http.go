package repository

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type HttpRepository struct {
	client   *http.Client
	cacheDir string
}

func (r *HttpRepository) Name() string {
	return "http"
}

func NewHttpRepository(cacheDir string) *HttpRepository {
	return &HttpRepository{
		client:   &http.Client{},
		cacheDir: cacheDir,
	}
}

func (r *HttpRepository) DownloadFile(ctx context.Context, url string, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return err
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
