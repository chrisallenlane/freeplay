package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration.
type Config struct {
	Port        int               `toml:"port"`
	CoverArtAPI string            `toml:"cover_art_api"`
	CoverArtKey string            `toml:"cover_art_api_key"`
	ROMs        map[string]ROM    `toml:"roms"`
	BIOS        map[string]string `toml:"bios"`
}

// ROM describes a single console's ROM directory and emulator core.
type ROM struct {
	Path string `toml:"path"`
	Core string `toml:"core"`
}

// Load reads and validates freeplay.toml from the given data directory.
func Load(dataDir string) (*Config, error) {
	path := filepath.Join(dataDir, "freeplay.toml")

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}

	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	cfg.resolvePaths(dataDir)
	cfg.checkDirectories()

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}

	for name, rom := range c.ROMs {
		if rom.Path == "" {
			return fmt.Errorf("rom %q: path is required", name)
		}
		if rom.Core == "" {
			return fmt.Errorf("rom %q: core is required", name)
		}
	}

	switch c.CoverArtAPI {
	case "", "igdb":
		// valid
	default:
		return fmt.Errorf("cover_art_api must be \"igdb\" or empty; got %q", c.CoverArtAPI)
	}

	if c.CoverArtAPI != "" && c.CoverArtKey == "" {
		return fmt.Errorf("cover_art_api_key is required when cover_art_api is set")
	}

	if c.CoverArtAPI == "igdb" && c.CoverArtKey != "" {
		parts := strings.SplitN(c.CoverArtKey, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("cover_art_api_key for igdb must be in \"client_id:client_secret\" format")
		}
	}

	if c.ROMs == nil {
		c.ROMs = make(map[string]ROM)
	}
	if c.BIOS == nil {
		c.BIOS = make(map[string]string)
	}

	return nil
}

func (c *Config) resolvePaths(dataDir string) {
	for name, rom := range c.ROMs {
		if !filepath.IsAbs(rom.Path) {
			rom.Path = filepath.Join(dataDir, rom.Path)
		}
		c.ROMs[name] = rom
	}

	for name, path := range c.BIOS {
		if !filepath.IsAbs(path) {
			c.BIOS[name] = filepath.Join(dataDir, path)
		}
	}
}

func (c *Config) checkDirectories() {
	for name, rom := range c.ROMs {
		if _, err := os.Stat(rom.Path); os.IsNotExist(err) {
			slog.Warn("ROM directory does not exist", "console", name, "path", rom.Path)
		}
	}

	for name, path := range c.BIOS {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			slog.Warn("BIOS directory does not exist", "console", name, "path", path)
		}
	}
}
