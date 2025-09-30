package manifest

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Manifest struct {
	Packages map[string]PackageDefinition `toml:"packages"`
}

type PackageDefinition struct {
	Repo        string            `toml:"repo"`
	Description string            `toml:"description"`
	Binaries    BinaryInfo        `toml:"binaries"`
	URLs        map[string]string `toml:"urls"`
}

type BinaryInfo struct {
	Names []string `toml:"names"`
}

func LoadManifest(path string) (*Manifest, error) {
	var m Manifest
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	return &m, nil
}

func (m *Manifest) GetPackage(name string) (*PackageDefinition, error) {
	pkg, ok := m.Packages[name]
	if !ok {
		return nil, fmt.Errorf("package %s not found in manifest", name)
	}
	return &pkg, nil
}

func (m *Manifest) GetURL(name, version string) (string, error) {
	pkg, err := m.GetPackage(name)
	if err != nil {
		return "", err
	}

	// Get platform-specific URL
	platform := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	urlTemplate, ok := pkg.URLs[platform]
	if !ok {
		return "", fmt.Errorf("platform %s not supported for %s", platform, name)
	}

	// Replace {version} placeholder (this might have to change because repos probably have different patterns?)
	url := strings.ReplaceAll(urlTemplate, "{version}", version)
	return url, nil
}
