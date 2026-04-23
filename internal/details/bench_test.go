package details

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/datadir"
	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// populateCacheOnDisk writes `count` games' worth of details.json files
// under datadir.IGDBCache(dataDir). Returns the console name used and a function
// that yields the rom filename for game index i.
func populateCacheOnDisk(tb testing.TB, dataDir string, count int) (string, func(int) string) {
	tb.Helper()
	console := "NES"
	romFor := func(i int) string { return fmt.Sprintf("Game%04d.nes", i) }
	for i := range count {
		_, cleanName := igdb.CleanFilename(romFor(i))
		dir := datadir.IGDBCacheGame(dataDir, console, cleanName)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			tb.Fatalf("mkdir: %v", err)
		}
		d := igdb.GameDetails{
			Name:     cleanName,
			Summary:  "A fake game for benchmarking.",
			CoverURL: "/cache/igdb/" + console + "/" + cleanName + "/cover.jpg",
		}
		data, err := json.Marshal(d)
		if err != nil {
			tb.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "details.json"), data, 0o600); err != nil {
			tb.Fatalf("write details: %v", err)
		}
	}
	return console, romFor
}

// BenchmarkCacheGet_Cold measures Get when each call reads details.json
// from disk. Without the in-memory layer (PERF-4) this is the steady-state
// cost on every rescan iteration. After PERF-4 only the first call per key
// pays this price.
func BenchmarkCacheGet_Cold(b *testing.B) {
	const games = 2000
	dir := b.TempDir()
	console, romFor := populateCacheOnDisk(b, dir, games)
	c := New(dir, nil)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if got := c.Get(console, romFor(i%games)); got == nil {
			b.Fatalf("Get returned nil for %s", romFor(i%games))
		}
		i++
	}
}

// BenchmarkCacheGet_Warm hits the same key repeatedly. Baseline today is
// identical to Cold (Get always re-reads disk); PERF-4 is expected to
// collapse this to a map lookup.
func BenchmarkCacheGet_Warm(b *testing.B) {
	const games = 2000
	dir := b.TempDir()
	console, romFor := populateCacheOnDisk(b, dir, games)
	c := New(dir, nil)
	// Prime the lookup — no-op today, meaningful once PERF-4 lands.
	if got := c.Get(console, romFor(0)); got == nil {
		b.Fatalf("prime: Get returned nil")
	}
	b.ReportAllocs()
	for b.Loop() {
		if got := c.Get(console, romFor(0)); got == nil {
			b.Fatalf("Get returned nil")
		}
	}
}
