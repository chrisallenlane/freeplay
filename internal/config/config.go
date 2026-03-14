package config

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
	// Stub — implemented in ticket #2
	return &Config{Port: 8080}, nil
}
