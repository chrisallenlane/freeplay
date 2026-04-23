package library

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// stubCache counts FetchAll calls and returns a configurable saved value.
type stubCache struct {
	mu             sync.Mutex
	calls          atomic.Int32
	savedSeq       []int // saved returned per call; last value is reused if calls > len
	details        map[string]*igdb.GameDetails
	lastEntriesLen int // length of the last entries slice passed to FetchAll
}

func (c *stubCache) Get(console, rom string) *igdb.GameDetails {
	if c.details == nil {
		return nil
	}
	return c.details[console+"/"+rom]
}

func (c *stubCache) FetchAll(entries []igdb.GameEntry) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastEntriesLen = len(entries)
	idx := int(c.calls.Add(1)) - 1
	if idx >= len(c.savedSeq) {
		idx = len(c.savedSeq) - 1
	}
	if len(c.savedSeq) == 0 {
		return 0
	}
	return c.savedSeq[idx]
}

func newScanner(t *testing.T) *scanner.Scanner {
	t.Helper()
	dir := t.TempDir()
	romDir := filepath.Join(dir, "NES")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(romDir, "game.nes"), []byte("rom"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		ROMs: map[string]config.ROM{
			"NES": {Path: romDir, Core: "fceumm"},
		},
	}
	return scanner.New(cfg, dir)
}

// TestRunPipelineSingleIterationWhenFetchAllReturnsZero verifies that the
// pipeline stops after one iteration when no new IGDB data was fetched.
func TestRunPipelineSingleIterationWhenFetchAllReturnsZero(t *testing.T) {
	scn := newScanner(t)
	cache := &stubCache{savedSeq: []int{0}}
	lib := New(scn, cache)

	lib.RunPipeline()

	if got := cache.calls.Load(); got != 1 {
		t.Errorf("FetchAll calls = %d, want 1", got)
	}
}

// TestRunPipelineLoopsWhenFetchAllReturnsPositive verifies the pipeline
// re-runs when FetchAll reports progress, then stops on the first zero.
func TestRunPipelineLoopsWhenFetchAllReturnsPositive(t *testing.T) {
	scn := newScanner(t)
	cache := &stubCache{savedSeq: []int{2, 1, 0}}
	lib := New(scn, cache)

	lib.RunPipeline()

	if got := cache.calls.Load(); got != 3 {
		t.Errorf("FetchAll calls = %d, want 3 (loop until 0)", got)
	}
}

// TestRunPipelineBoundedAtMaxIterations is the guarantee the old
// scanner-callback design lacked: if FetchAll keeps reporting new work,
// the loop terminates after maxRescanIterations rather than spinning.
func TestRunPipelineBoundedAtMaxIterations(t *testing.T) {
	scn := newScanner(t)
	cache := &stubCache{savedSeq: []int{1}} // always returns 1
	lib := New(scn, cache)

	lib.RunPipeline()

	if got := cache.calls.Load(); got != int32(maxRescanIterations) {
		t.Errorf(
			"FetchAll calls = %d, want %d (bounded loop)",
			got, maxRescanIterations,
		)
	}
	// Assert the concrete value of maxRescanIterations is 3. This pins
	// the loop's worst-case runtime so bumping the constant is a
	// deliberate choice rather than a silent regression.
	if maxRescanIterations != 3 {
		t.Errorf("maxRescanIterations = %d, want 3", maxRescanIterations)
	}
	// Also assert that FetchAll received exactly as many entries as
	// the catalog has games — guards against make([]GameEntry, N+1)
	// slip that would pass zero-value entries through the pipeline.
	wantEntries := len(scn.Catalog().Games)
	if cache.lastEntriesLen != wantEntries {
		t.Errorf(
			"FetchAll entries length = %d, want %d",
			cache.lastEntriesLen, wantEntries,
		)
	}
}

// TestTriggerRescanReturnsFalseWhenBusy verifies the 409-mapping contract.
func TestTriggerRescanReturnsFalseWhenBusy(t *testing.T) {
	scn := newScanner(t)
	cache := &stubCache{savedSeq: []int{0}}
	lib := New(scn, cache)

	// Hold the pipeline lock so TriggerRescan observes "busy".
	lib.mu.Lock()
	defer lib.mu.Unlock()

	if lib.TriggerRescan() {
		t.Error("TriggerRescan should return false when pipeline is busy")
	}
}

// TestNilCacheNoPanic verifies the library tolerates a nil details cache
// (the production configuration when IGDB is not set up).
func TestNilCacheNoPanic(t *testing.T) {
	scn := newScanner(t)
	lib := New(scn, nil)

	lib.RunPipeline() // must not panic or loop
}

// rescanTestTimeout bounds how long a test will wait for a background
// pipeline goroutine started via TriggerRescan to reach a given call count.
const rescanTestTimeout = 2 * time.Second

// waitForFetchAllCalls polls the stubCache until its call counter reaches
// want, or fatally fails the test if the deadline passes.
func waitForFetchAllCalls(t *testing.T, cache *stubCache, want int32) {
	t.Helper()
	deadline := time.Now().Add(rescanTestTimeout)
	for time.Now().Before(deadline) {
		if cache.calls.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("FetchAll calls = %d, want %d after %s", cache.calls.Load(), want, rescanTestTimeout)
}

// TestTriggerRescanHappyPath verifies the 200-mapping contract: an idle
// library returns true from TriggerRescan and runs the pipeline in a
// background goroutine. Complements TestTriggerRescanReturnsFalseWhenBusy,
// which covers the 409 path.
func TestTriggerRescanHappyPath(t *testing.T) {
	scn := newScanner(t)
	cache := &stubCache{savedSeq: []int{0}}
	lib := New(scn, cache)

	if !lib.TriggerRescan() {
		t.Fatal("TriggerRescan should return true when pipeline is idle")
	}
	waitForFetchAllCalls(t, cache, 1)

	// With the lock released by the goroutine's defer, a fresh
	// TriggerRescan should succeed again.
	if !lib.TriggerRescan() {
		t.Error("second TriggerRescan should return true after first pipeline finished")
	}
	// Drain the second goroutine to avoid leaking it into sibling tests.
	waitForFetchAllCalls(t, cache, 2)
}

// TestMetaLookupEnrichesCatalog verifies that metaLookup correctly maps
// GameDetails fields into the scanner catalog (IGDBName, Developers,
// Publishers, Year). A field-rename or reorder would silently drop
// enrichment data without this regression guard.
func TestMetaLookupEnrichesCatalog(t *testing.T) {
	scn := newScanner(t)

	// The newScanner helper places a ROM at "NES/game.nes". metaLookup
	// passes the raw romFilename through to stubCache.Get, so key with
	// that exact string.
	cache := &stubCache{
		savedSeq: []int{0},
		details: map[string]*igdb.GameDetails{
			"NES/game.nes": {
				Name:             "Test Game",
				Developers:       []string{"Test Dev"},
				Publishers:       []string{"Test Pub"},
				FirstReleaseDate: "1985-09-13",
			},
		},
	}
	lib := New(scn, cache)

	lib.RunPipeline()

	cat := scn.Catalog()
	if len(cat.Games) != 1 {
		t.Fatalf("catalog has %d games, want 1", len(cat.Games))
	}
	g := cat.Games[0]
	if g.IGDBName != "Test Game" {
		t.Errorf("IGDBName = %q, want %q", g.IGDBName, "Test Game")
	}
	if len(g.Developers) != 1 || g.Developers[0] != "Test Dev" {
		t.Errorf("Developers = %v, want [Test Dev]", g.Developers)
	}
	if len(g.Publishers) != 1 || g.Publishers[0] != "Test Pub" {
		t.Errorf("Publishers = %v, want [Test Pub]", g.Publishers)
	}
	if g.Year != 1985 {
		t.Errorf("Year = %d, want 1985", g.Year)
	}
}
