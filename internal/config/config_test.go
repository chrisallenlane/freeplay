package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "freeplay.toml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadBasic(t *testing.T) {
	dir := t.TempDir()
	romDir := filepath.Join(dir, "roms", "nes")
	os.MkdirAll(romDir, 0755)

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

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadMissingPath(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
core = "fceumm"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestLoadMissingCore(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[roms.NES]
path = "roms/nes"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing core")
	}
}

func TestLoadInvalidCoverArtAPI(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
cover_art_api = "invalid"
cover_art_api_key = "key"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid cover_art_api")
	}
}

func TestLoadCoverArtAPIMissingKey(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
cover_art_api = "igdb"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error when cover_art_api set without key")
	}
}

func TestLoadIGDBKeyMissingSeparator(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
cover_art_api = "igdb"
cover_art_api_key = "missingcolon"
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for IGDB key without colon separator")
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

func TestLoadInvalidPort(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
port = 99999
`)
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}
