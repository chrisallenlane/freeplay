package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/config"
)

func setupTestDir(t *testing.T) (string, *config.Config) {
	t.Helper()
	dir := t.TempDir()

	nesDir := filepath.Join(dir, "roms", "nes")
	genDir := filepath.Join(dir, "roms", "genesis")
	os.MkdirAll(nesDir, 0o755)
	os.MkdirAll(genDir, 0o755)

	os.WriteFile(filepath.Join(nesDir, "Mega Man.zip"), []byte("rom"), 0o644)
	os.WriteFile(filepath.Join(nesDir, "Zelda.zip"), []byte("rom"), 0o644)
	os.WriteFile(filepath.Join(genDir, "Sonic.gen"), []byte("rom"), 0o644)

	// Create a cover for Sonic
	coverDir := filepath.Join(dir, "covers", "Genesis")
	os.MkdirAll(coverDir, 0o755)
	os.WriteFile(filepath.Join(coverDir, "Sonic.png"), []byte("img"), 0o644)

	cfg := &config.Config{
		Port: 8080,
		ROMs: map[string]config.ROM{
			"NES":     {Path: nesDir, Core: "fceumm"},
			"Genesis": {Path: genDir, Core: "genesis_plus_gx"},
		},
		BIOS: map[string]string{},
	}

	return dir, cfg
}

func TestScanFindsGames(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.Catalog()
	if len(cat.Games) != 3 {
		t.Fatalf("got %d games, want 3", len(cat.Games))
	}
	if len(cat.Consoles) != 2 {
		t.Fatalf("got %d consoles, want 2", len(cat.Consoles))
	}
}

func TestScanSortOrder(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.Catalog()
	// Consoles should be alphabetical
	if cat.Consoles[0] != "Genesis" || cat.Consoles[1] != "NES" {
		t.Errorf("consoles not sorted: %v", cat.Consoles)
	}

	// Genesis game first, then NES games sorted
	if cat.Games[0].Console != "Genesis" {
		t.Errorf("first game should be Genesis, got %s", cat.Games[0].Console)
	}
	if cat.Games[1].Filename != "Mega Man.zip" {
		t.Errorf("second game should be Mega Man.zip, got %s", cat.Games[1].Filename)
	}
}

func TestScanSkipsSubdirectories(t *testing.T) {
	dir, cfg := setupTestDir(t)
	// Add a subdirectory inside NES roms
	os.MkdirAll(filepath.Join(cfg.ROMs["NES"].Path, "subdir"), 0o755)

	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.Catalog()
	for _, g := range cat.Games {
		if g.Filename == "subdir" {
			t.Error("subdirectory should be skipped")
		}
	}
}

func TestScanCoverDetection(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.Catalog()
	for _, g := range cat.Games {
		if g.Console == "Genesis" && g.Filename == "Sonic.gen" {
			if !g.HasCover {
				t.Error("Sonic should have cover")
			}
		}
		if g.Console == "NES" {
			if g.HasCover {
				t.Errorf("NES game %s should not have cover", g.Filename)
			}
		}
	}
}

func TestScanEmptyBeforeFirstScan(t *testing.T) {
	_, cfg := setupTestDir(t)
	s := New(cfg, "")

	cat := s.Catalog()
	if len(cat.Games) != 0 {
		t.Errorf("expected empty games before scan, got %d", len(cat.Games))
	}
	if len(cat.Consoles) != 0 {
		t.Errorf("expected empty consoles before scan, got %d", len(cat.Consoles))
	}
}

func TestScanMissingDirectory(t *testing.T) {
	cfg := &config.Config{
		ROMs: map[string]config.ROM{
			"NES": {Path: "/nonexistent/path", Core: "fceumm"},
		},
		BIOS: map[string]string{},
	}
	s := New(cfg, "")
	s.ScanBlocking()

	cat := s.Catalog()
	if len(cat.Games) != 0 {
		t.Errorf("expected no games for missing dir, got %d", len(cat.Games))
	}
}

func TestScanConcurrentPrevention(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	// Lock the mutex to simulate a running scan
	s.mu.Lock()
	ran := s.Scan()
	s.mu.Unlock()

	if ran {
		t.Error("Scan should have returned false when mutex is held")
	}
}

func TestCatalogJSON(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	data, err := s.CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON error: %v", err)
	}

	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(cat.Games) != 3 {
		t.Errorf("got %d games, want 3", len(cat.Games))
	}
}

func TestCatalogJSONEmpty(t *testing.T) {
	_, cfg := setupTestDir(t)
	s := New(cfg, "")

	data, err := s.CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON error: %v", err)
	}

	var cat Catalog
	json.Unmarshal(data, &cat)
	if len(cat.Games) != 0 {
		t.Errorf("expected empty games, got %d", len(cat.Games))
	}
}

func TestScanBIOSDetection(t *testing.T) {
	dir, cfg := setupTestDir(t)
	cfg.BIOS["NES"] = "/some/bios/dir"

	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.Catalog()
	for _, g := range cat.Games {
		if g.Console == "NES" && !g.HasBios {
			t.Errorf("NES game %s should have HasBios=true", g.Filename)
		}
		if g.Console == "Genesis" && g.HasBios {
			t.Errorf("Genesis game %s should have HasBios=false", g.Filename)
		}
	}
}

func TestScanCallback(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	var callbackGames []Game
	s.SetOnScanComplete(func(games []Game) {
		callbackGames = games
	})

	s.ScanBlocking()

	if len(callbackGames) != 3 {
		t.Errorf("callback got %d games, want 3", len(callbackGames))
	}
}

func TestScanReturnsTrue(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)

	if !s.Scan() {
		t.Error("Scan should return true when no scan is in progress")
	}

	cat := s.Catalog()
	if len(cat.Games) != 3 {
		t.Errorf("got %d games, want 3", len(cat.Games))
	}
}
