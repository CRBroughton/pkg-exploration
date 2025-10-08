package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Name       string                     `toml:"name"`
	Packages   map[string]string          `toml:"packages"`
	Containers map[string]ContainerConfig `toml:"containers"`
}

type ContainerConfig struct {
	Image   string `toml:"image"`
	Version string `toml:"version"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.Name == "" {
		return nil, fmt.Errorf("config.name is required")
	}

	return &cfg, nil
}

func (c *Config) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(c)
}
