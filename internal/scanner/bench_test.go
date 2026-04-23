package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// buildLargeConfig creates a temp dataDir with the requested number of games
// distributed evenly across consoles. Returns a config and the dataDir path.
// Used to give benchmarks a realistic working set without real ROM data.
func buildLargeConfig(tb testing.TB, games, consoles int) (*config.Config, string) {
	tb.Helper()
	dir := tb.TempDir()
	cfg := &config.Config{ROMs: make(map[string]config.ROM)}
	perConsole := games / consoles
	for c := range consoles {
		consoleName := fmt.Sprintf("Console%d", c)
		romDir := filepath.Join(dir, "roms", consoleName)
		if err := os.MkdirAll(romDir, 0o750); err != nil {
			tb.Fatalf("mkdir: %v", err)
		}
		for g := range perConsole {
			f := filepath.Join(romDir, fmt.Sprintf("Game%04d.rom", g))
			if err := os.WriteFile(f, nil, 0o600); err != nil {
				tb.Fatalf("write rom: %v", err)
			}
		}
		cfg.ROMs[consoleName] = config.ROM{Path: romDir, Core: "stub"}
	}
	return cfg, dir
}

func BenchmarkScan_2000_Games(b *testing.B) {
	cfg, dir := buildLargeConfig(b, 2000, 10)
	s := New(cfg, dir)
	b.ReportAllocs()
	for b.Loop() {
		s.ScanBlocking()
	}
}

func BenchmarkEnrichMetadata(b *testing.B) {
	cfg, dir := buildLargeConfig(b, 2000, 10)
	s := New(cfg, dir)
	s.ScanBlocking()
	lookup := func(_, rom string) *GameMeta {
		return &GameMeta{
			Name:             rom,
			Developers:       []string{"Dev"},
			Publishers:       []string{"Pub"},
			FirstReleaseDate: "2000-01-01",
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		s.EnrichMetadata(lookup)
	}
}

func BenchmarkCatalogJSON(b *testing.B) {
	cfg, dir := buildLargeConfig(b, 2000, 10)
	s := New(cfg, dir)
	s.ScanBlocking()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := s.CatalogJSON(); err != nil {
			b.Fatalf("CatalogJSON: %v", err)
		}
	}
}
