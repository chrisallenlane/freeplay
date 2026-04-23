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
	if err := os.MkdirAll(nesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(nesDir, "Mega Man.zip"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nesDir, "Zelda.zip"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "Sonic.gen"), []byte("rom"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a cover for Sonic
	coverDir := filepath.Join(dir, "covers", "Genesis")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, "Sonic.png"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Port: 8080,
		ROMs: map[string]config.ROM{
			"NES":     {Path: nesDir, Core: "fceumm"},
			"Genesis": {Path: genDir, Core: "genesis_plus_gx"},
		},
	}

	return dir, cfg
}

func TestScanFindsGames(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.catalog.Load()
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

	cat := s.catalog.Load()
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
	if err := os.MkdirAll(filepath.Join(cfg.ROMs["NES"].Path, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.catalog.Load()
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

	cat := s.catalog.Load()
	foundSonic := false
	foundNES := false
	for _, g := range cat.Games {
		if g.Console == "Genesis" && g.Filename == "Sonic.gen" {
			foundSonic = true
			if !g.HasCover {
				t.Error("Sonic should have cover")
			}
		}
		if g.Console == "NES" {
			foundNES = true
			if g.HasCover {
				t.Errorf("NES game %s should not have cover", g.Filename)
			}
		}
	}
	if !foundSonic {
		t.Error("Sonic.gen not found in catalog")
	}
	if !foundNES {
		t.Error("no NES games found in catalog")
	}
}

func TestScanManualDetection(t *testing.T) {
	dir, cfg := setupTestDir(t)

	// Create a manual for Mega Man
	manualDir := filepath.Join(dir, "manuals", "NES")
	if err := os.MkdirAll(manualDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manualDir, "Mega Man.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.catalog.Load()
	for _, g := range cat.Games {
		if g.Console == "NES" && g.Filename == "Mega Man.zip" {
			if !g.HasManual {
				t.Error("Mega Man should have manual")
			}
		}
		if g.Console == "NES" && g.Filename == "Zelda.zip" {
			if g.HasManual {
				t.Error("Zelda should not have manual")
			}
		}
		if g.Console == "Genesis" && g.Filename == "Sonic.gen" {
			if g.HasManual {
				t.Error("Sonic should not have manual")
			}
		}
	}
}

func TestScanEmptyBeforeFirstScan(t *testing.T) {
	_, cfg := setupTestDir(t)
	s := New(cfg, "")

	cat := s.catalog.Load()
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
	}
	s := New(cfg, "")
	s.ScanBlocking()

	cat := s.catalog.Load()
	if len(cat.Games) != 0 {
		t.Errorf("expected no games for missing dir, got %d", len(cat.Games))
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
	if err := json.Unmarshal(data, &cat); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(cat.Games) != 0 {
		t.Errorf("expected empty games, got %d", len(cat.Games))
	}
}

func TestScanBIOSDetection(t *testing.T) {
	dir, cfg := setupTestDir(t)
	nes := cfg.ROMs["NES"]
	nes.Bios = "/some/bios/SCPH1001.BIN"
	cfg.ROMs["NES"] = nes

	s := New(cfg, dir)
	s.ScanBlocking()

	cat := s.catalog.Load()
	for _, g := range cat.Games {
		if g.Console == "NES" && !g.HasBios {
			t.Errorf("NES game %s should have HasBios=true", g.Filename)
		}
		if g.Console == "Genesis" && g.HasBios {
			t.Errorf("Genesis game %s should have HasBios=false", g.Filename)
		}
	}
}

func TestEnrichMetadata(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	// Pre-scan: verify games have no metadata yet.
	for _, g := range s.catalog.Load().Games {
		if g.IGDBName != "" {
			t.Errorf(
				"expected empty IGDBName before enrichment for %q, got %q",
				g.Filename, g.IGDBName,
			)
		}
	}

	// Lookup that returns full metadata for "Mega Man.zip" only.
	lookup := func(console, romFilename string) *GameMeta {
		if console == "NES" && romFilename == "Mega Man.zip" {
			return &GameMeta{
				Name:             "Mega Man",
				Developers:       []string{"Capcom"},
				Publishers:       []string{"Capcom"},
				FirstReleaseDate: "1987-12-17",
			}
		}
		return nil
	}
	s.EnrichMetadata(lookup)

	cat := s.catalog.Load()
	if len(cat.Games) != 3 {
		t.Fatalf("expected 3 games after enrichment, got %d", len(cat.Games))
	}
	// Consoles list must be preserved by EnrichMetadata — it's used by
	// the frontend for the console-filter dropdown. Kills a mutation
	// that would replace cat.Consoles with []string{} at the store site.
	if len(cat.Consoles) == 0 {
		t.Errorf("EnrichMetadata dropped Consoles list; got empty")
	}
	// The accessor Scanner.Catalog() must return the same catalog the
	// internal atomic pointer holds — kills a mutation that returns nil.
	if got := s.Catalog(); got == nil {
		t.Errorf("Scanner.Catalog() returned nil after ScanBlocking + EnrichMetadata")
	} else if len(got.Games) != len(cat.Games) {
		t.Errorf("Scanner.Catalog() games length = %d, want %d",
			len(got.Games), len(cat.Games))
	}

	found := false
	for _, g := range cat.Games {
		if g.Console == "NES" && g.Filename == "Mega Man.zip" {
			found = true
			if g.IGDBName != "Mega Man" {
				t.Errorf("IGDBName = %q, want %q", g.IGDBName, "Mega Man")
			}
			if len(g.Developers) != 1 || g.Developers[0] != "Capcom" {
				t.Errorf("Developers = %v, want [Capcom]", g.Developers)
			}
			if len(g.Publishers) != 1 || g.Publishers[0] != "Capcom" {
				t.Errorf("Publishers = %v, want [Capcom]", g.Publishers)
			}
			if g.Year != 1987 {
				t.Errorf("Year = %d, want 1987", g.Year)
			}
			// Other fields must be preserved.
			if g.Core != "fceumm" {
				t.Errorf("Core = %q, want %q", g.Core, "fceumm")
			}
		} else {
			// All other games must still have empty metadata.
			if g.IGDBName != "" {
				t.Errorf(
					"game %q / %q should have empty IGDBName, got %q",
					g.Console, g.Filename, g.IGDBName,
				)
			}
			if len(g.Developers) != 0 {
				t.Errorf(
					"game %q / %q should have empty Developers, got %v",
					g.Console, g.Filename, g.Developers,
				)
			}
			if len(g.Publishers) != 0 {
				t.Errorf(
					"game %q / %q should have empty Publishers, got %v",
					g.Console, g.Filename, g.Publishers,
				)
			}
			if g.Year != 0 {
				t.Errorf(
					"game %q / %q Year = %d, want 0",
					g.Console, g.Filename, g.Year,
				)
			}
		}
	}
	if !found {
		t.Error("Mega Man.zip not found in catalog after enrichment")
	}

	// Verify enrichment is reflected in CatalogJSON output.
	data, err := s.CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON error: %v", err)
	}
	var serialized Catalog
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, g := range serialized.Games {
		if g.Console == "NES" && g.Filename == "Mega Man.zip" {
			if g.IGDBName != "Mega Man" {
				t.Errorf(
					"CatalogJSON IGDBName for Mega Man.zip = %q, want %q",
					g.IGDBName, "Mega Man",
				)
			}
		}
	}
}

func TestEnrichMetadataMissingEntry(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	// Lookup that never finds anything — all fields must stay zero.
	s.EnrichMetadata(func(_, _ string) *GameMeta { return nil })

	for _, g := range s.catalog.Load().Games {
		if g.IGDBName != "" || len(g.Developers) != 0 ||
			len(g.Publishers) != 0 || g.Year != 0 {
			t.Errorf(
				"game %q / %q: expected all metadata empty, got %+v",
				g.Console, g.Filename, g,
			)
		}
	}
}

func TestCatalogJSONOmitempty(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	// Enrich only one game so the rest have empty optional fields.
	s.EnrichMetadata(func(console, romFilename string) *GameMeta {
		if console == "NES" && romFilename == "Mega Man.zip" {
			return &GameMeta{
				Name:             "Mega Man",
				Developers:       []string{"Capcom"},
				Publishers:       []string{"Capcom"},
				FirstReleaseDate: "1987-12-17",
			}
		}
		return nil
	})

	data, err := s.CatalogJSON()
	if err != nil {
		t.Fatalf("CatalogJSON error: %v", err)
	}

	// Parse into a generic structure so we can inspect raw JSON keys.
	type rawCatalog struct {
		Games []map[string]any `json:"games"`
	}
	var raw rawCatalog
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, gm := range raw.Games {
		console, _ := gm["console"].(string)
		filename, _ := gm["filename"].(string)
		if console == "NES" && filename == "Mega Man.zip" {
			// Enriched game must carry the new fields.
			if _, ok := gm["developers"]; !ok {
				t.Error("Mega Man.zip missing 'developers' key in JSON")
			}
			if _, ok := gm["publishers"]; !ok {
				t.Error("Mega Man.zip missing 'publishers' key in JSON")
			}
			if _, ok := gm["year"]; !ok {
				t.Error("Mega Man.zip missing 'year' key in JSON")
			}
		} else {
			// Un-enriched games must NOT carry the new fields (omitempty).
			if _, ok := gm["developers"]; ok {
				t.Errorf("%s/%s has unexpected 'developers' key", console, filename)
			}
			if _, ok := gm["publishers"]; ok {
				t.Errorf("%s/%s has unexpected 'publishers' key", console, filename)
			}
			if _, ok := gm["year"]; ok {
				t.Errorf("%s/%s has unexpected 'year' key", console, filename)
			}
		}
	}
}

// TestHasGameAfterEnrichMetadata pins the invariant that HasGame's
// O(1) lookup index (Catalog.gameSet) is carried across every path
// that publishes a new Catalog. EnrichMetadata constructs a fresh
// Catalog post-scan; a regression where it forgot to rebuild the
// index would make save-upload handlers 404 on every request in
// production, since the library pipeline always calls EnrichMetadata
// after the initial scan.
func TestHasGameAfterEnrichMetadata(t *testing.T) {
	dir, cfg := setupTestDir(t)
	s := New(cfg, dir)
	s.ScanBlocking()

	// Sanity-check: scanned games are present before enrichment.
	if !s.HasGame("NES", "Mega Man.zip") {
		t.Fatal("pre-enrichment HasGame(NES, Mega Man.zip) = false, want true")
	}

	// Run EnrichMetadata with a stub lookup — it must preserve the index.
	s.EnrichMetadata(func(_, _ string) *GameMeta { return nil })

	for _, g := range s.catalog.Load().Games {
		if !s.HasGame(g.Console, g.Filename) {
			t.Errorf(
				"post-enrichment HasGame(%q, %q) = false, want true",
				g.Console, g.Filename,
			)
		}
	}
}

func TestParseYear(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"full ISO 8601", "2020-12-10", 2020},
		{"year only", "1985", 1985},
		{"empty string", "", 0},
		{"garbage input", "abcd-12-10", 0},
		{"too short", "199", 0},
		{"partial garbage", "20xx", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseYear(tt.input)
			if got != tt.want {
				t.Errorf("parseYear(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
