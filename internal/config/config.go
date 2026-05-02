// Package config handles loading and validating the freeplay configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Sentinel errors for the validation rules in Load. Wrapped via
// fmt.Errorf "%w: <details>" so callers can match a specific failure
// mode with errors.Is and tests don't couple to the human-readable
// part of the error message.
var (
	ErrLoadingConfig    = errors.New("loading config")
	ErrInvalidPort      = errors.New("port must be between 1 and 65535")
	ErrROMPathRequired  = errors.New("rom path is required")
	ErrROMCoreRequired  = errors.New("rom core is required")
	ErrInvalidCoverAPI  = errors.New(`cover_art_api must be "igdb" or empty`)
	ErrCoverKeyRequired = errors.New("cover_art_api_key is required when cover_art_api is set")
	ErrInvalidIGDBKey   = errors.New(`cover_art_api_key for igdb must be in "client_id:client_secret" format`)
)

// Config holds the application configuration.
type Config struct {
	Port        int            `toml:"port"`
	CoverArtAPI string         `toml:"cover_art_api"`
	CoverArtKey string         `toml:"cover_art_api_key"`
	ROMs        map[string]ROM `toml:"roms"`
}

// ROM describes a single console's ROM directory and emulator core.
type ROM struct {
	Path            string `toml:"path"`
	Core            string `toml:"core"`
	Bios            string `toml:"bios"`
	IGDBPlatformIDs []int  `toml:"igdb_platform_ids"`
}

// Load reads and validates freeplay.toml from the given data directory.
func Load(dataDir string) (*Config, error) {
	path := filepath.Join(dataDir, "freeplay.toml")

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrLoadingConfig, path, err)
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
		return fmt.Errorf("%w: got %d", ErrInvalidPort, c.Port)
	}

	for name, rom := range c.ROMs {
		if rom.Path == "" {
			return fmt.Errorf("rom %q: %w", name, ErrROMPathRequired)
		}
		if rom.Core == "" {
			return fmt.Errorf("rom %q: %w", name, ErrROMCoreRequired)
		}
	}

	switch c.CoverArtAPI {
	case "", "igdb":
		// valid
	default:
		return fmt.Errorf("%w: got %q", ErrInvalidCoverAPI, c.CoverArtAPI)
	}

	if c.CoverArtAPI != "" && c.CoverArtKey == "" {
		return ErrCoverKeyRequired
	}

	if c.CoverArtAPI == "igdb" {
		parts := strings.SplitN(c.CoverArtKey, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return ErrInvalidIGDBKey
		}
	}

	if c.ROMs == nil {
		c.ROMs = make(map[string]ROM)
	}

	return nil
}

func (c *Config) resolvePaths(dataDir string) {
	for name, rom := range c.ROMs {
		if !filepath.IsAbs(rom.Path) {
			rom.Path = filepath.Join(dataDir, rom.Path)
		}
		if rom.Bios != "" && !filepath.IsAbs(rom.Bios) {
			rom.Bios = filepath.Join(dataDir, rom.Bios)
		}
		c.ROMs[name] = rom
	}
}

func (c *Config) checkDirectories() {
	for name, rom := range c.ROMs {
		if _, err := os.Stat(rom.Path); os.IsNotExist(err) {
			slog.Warn("ROM directory does not exist", "console", name, "path", rom.Path)
		}
		if rom.Bios != "" {
			if _, err := os.Stat(rom.Bios); os.IsNotExist(err) {
				slog.Warn("BIOS file does not exist", "console", name, "path", rom.Bios)
			}
		}
	}
}
