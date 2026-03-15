package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}

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

func TestLoadPortBoundaries(t *testing.T) {
	for _, port := range []int{1, 65535} {
		t.Run(fmt.Sprintf("port=%d", port), func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, fmt.Sprintf(`
port = %d

[roms.NES]
path = "/roms/nes"
core = "fceumm"
`, port))
			cfg, err := Load(dir)
			if err != nil {
				t.Fatalf("port %d should be valid: %v", port, err)
			}
			if cfg.Port != port {
				t.Errorf("port = %d, want %d", cfg.Port, port)
			}
		})
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
		name      string
		config    string // empty means don't write a config file
		wantInErr string // substring that must appear in the error message
	}{
		{"missing file", "", "loading config"},
		{"missing path", "[roms.NES]\ncore = \"fceumm\"", "path is required"},
		{"missing core", "[roms.NES]\npath = \"roms/nes\"", "core is required"},
		{"invalid cover_art_api", "cover_art_api = \"invalid\"\ncover_art_api_key = \"key\"", "cover_art_api must be"},
		{"cover_art_api missing key", "cover_art_api = \"igdb\"", "cover_art_api_key is required"},
		{"IGDB key missing separator", "cover_art_api = \"igdb\"\ncover_art_api_key = \"missingcolon\"", "client_id:client_secret"},
		{"invalid port", "port = 99999", "port must be"},
		{"IGDB key empty client_id", "cover_art_api = \"igdb\"\ncover_art_api_key = \":secret\"", "client_id:client_secret"},
		{"IGDB key empty secret", "cover_art_api = \"igdb\"\ncover_art_api_key = \"clientid:\"", "client_id:client_secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.config != "" {
				writeConfig(t, dir, tt.config)
			}
			_, err := Load(dir)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantInErr)
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

func FuzzLoad(f *testing.F) {
	f.Add([]byte(`port = 8080
[roms.NES]
path = "/roms/nes"
core = "fceumm"
`))
	f.Add([]byte(`invalid toml {{{{`))
	f.Add([]byte(``))
	f.Add([]byte(`port = -1`))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "freeplay.toml"), data, 0o644); err != nil {
			t.Skip()
		}
		// Must not panic regardless of input
		_, _ = Load(dir)
	})
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
