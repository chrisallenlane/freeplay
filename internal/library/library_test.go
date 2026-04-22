package library

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// stubCache counts FetchAll calls and returns a configurable saved value.
type stubCache struct {
	mu       sync.Mutex
	calls    atomic.Int32
	savedSeq []int // saved returned per call; last value is reused if calls > len
	details  map[string]*igdb.GameDetails
}

func (c *stubCache) Get(console, rom string) *igdb.GameDetails {
	if c.details == nil {
		return nil
	}
	return c.details[console+"/"+rom]
}

func (c *stubCache) FetchAll(_ []igdb.GameEntry) int {
	c.mu.Lock()
	defer c.mu.Unlock()
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
