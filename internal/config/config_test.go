package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "freeplay.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBasic(t *testing.T) {
	dir := t.TempDir()
	romDir := filepath.Join(dir, "roms", "nes")
	os.MkdirAll(romDir, 0o755)

	writeConfig(t, dir, `
port = 9090

[roms.NES]
path = "roms/nes"
core = "fceumm"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Port)
	}
	rom, ok := cfg.ROMs["NES"]
	if !ok {
		t.Fatal("NES rom entry missing")
	}
	if rom.Core != "fceumm" {
		t.Errorf("core = %q, want fceumm", rom.Core)
	}
	if rom.Path != filepath.Join(dir, "roms", "nes") {
		t.Errorf("path = %q, want resolved path", rom.Path)
	}
}

func TestLoadDefaultPort(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
path = "/absolute/path"
core = "fceumm"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Port)
	}
}

func TestLoadAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
path = "/absolute/roms"
core = "fceumm"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ROMs["NES"].Path != "/absolute/roms" {
		t.Errorf("absolute path was modified: %q", cfg.ROMs["NES"].Path)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		config string // empty means don't write a config file
	}{
		{"missing file", ""},
		{"missing path", "[roms.NES]\ncore = \"fceumm\""},
		{"missing core", "[roms.NES]\npath = \"roms/nes\""},
		{"invalid cover_art_api", "cover_art_api = \"invalid\"\ncover_art_api_key = \"key\""},
		{"cover_art_api missing key", "cover_art_api = \"igdb\""},
		{"IGDB key missing separator", "cover_art_api = \"igdb\"\ncover_art_api_key = \"missingcolon\""},
		{"invalid port", "port = 99999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.config != "" {
				writeConfig(t, dir, tt.config)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadBIOSResolution(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
path = "/roms/nes"
core = "fceumm"

[bios]
PlayStation = "bios/ps1"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BIOS["PlayStation"] != filepath.Join(dir, "bios", "ps1") {
		t.Errorf("bios path = %q, want resolved path", cfg.BIOS["PlayStation"])
	}
}

func TestLoadIGDBPlatformIDs(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
path = "/roms/nes"
core = "fceumm"
igdb_platform_ids = [18, 99]

[roms.Genesis]
path = "/roms/genesis"
core = "genesis_plus_gx"
`)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	nes := cfg.ROMs["NES"]
	if len(nes.IGDBPlatformIDs) != 2 || nes.IGDBPlatformIDs[0] != 18 || nes.IGDBPlatformIDs[1] != 99 {
		t.Errorf("NES IGDBPlatformIDs = %v, want [18 99]", nes.IGDBPlatformIDs)
	}
	gen := cfg.ROMs["Genesis"]
	if len(gen.IGDBPlatformIDs) != 0 {
		t.Errorf("Genesis IGDBPlatformIDs should be empty, got %v", gen.IGDBPlatformIDs)
	}
}
