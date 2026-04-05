package details

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// TestFetchAllIdempotent verifies that calling FetchAll twice with the
// same entries returns 0 on the second call (because isCached returns true).
// This is the key property that ensures the onScanComplete re-scan loop
// in main.go terminates.
func TestFetchAllIdempotent(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*igdb.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	}

	// First call should save 1
	count1 := c.FetchAll(entries)
	if count1 != 1 {
		t.Fatalf("first FetchAll() = %d, want 1", count1)
	}

	// Second call should save 0 (already cached)
	count2 := c.FetchAll(entries)
	if count2 != 0 {
		t.Errorf("second FetchAll() = %d, want 0 (idempotent)", count2)
	}
}

// TestFetchAllAfterCacheCorruption verifies behavior when the cache
// directory exists but details.json is deleted between FetchAll calls.
// This simulates a transient filesystem issue that could cause the
// re-scan loop in main.go to spin: FetchAll saves, triggers re-scan,
// but isCached returns false on the next pass.
func TestFetchAllAfterCacheCorruption(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	var fetchCount atomic.Int32
	fetcher := &countingMockFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*igdb.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
		fetchCount: &fetchCount,
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	}

	// First call saves successfully
	count1 := c.FetchAll(entries)
	if count1 != 1 {
		t.Fatalf("first FetchAll() = %d, want 1", count1)
	}

	// Corrupt the cache: remove details.json
	jsonPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", "details.json",
	)
	if err := os.Remove(jsonPath); err != nil {
		t.Fatalf("removing details.json: %v", err)
	}

	// Also remove .notfound if present
	notFoundPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", ".notfound",
	)
	_ = os.Remove(notFoundPath)

	// Second call: isCached returns false, so FetchAll tries again
	count2 := c.FetchAll(entries)
	if count2 != 1 {
		t.Errorf(
			"FetchAll after cache corruption = %d, want 1 (re-fetched)",
			count2,
		)
	}

	t.Logf(
		"INFO: Cache corruption causes FetchAll to return saved > 0 again, "+
			"which would trigger another re-scan cycle in main.go. "+
			"Total fetches: %d",
		fetchCount.Load(),
	)
}

// TestFetchAllEmptyEntries verifies that FetchAll with an empty slice
// returns 0 and does not trigger any re-scan.
func TestFetchAllEmptyEntries(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Game": 1},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]igdb.GameEntry{})
	if count != 0 {
		t.Errorf("FetchAll([]) = %d, want 0", count)
	}
	if fetcher.searchCalls != 0 {
		t.Errorf(
			"SearchGame called %d times for empty entries, want 0",
			fetcher.searchCalls,
		)
	}
}

// TestFetchingFlagResetOnCompletion verifies that the Fetching() flag is
// properly reset after FetchAll completes.
func TestFetchingFlagResetOnCompletion(t *testing.T) {
	fetcher := &mockIGDBFetcher{}
	dir := t.TempDir()
	c := New(dir, fetcher)

	if c.Fetching() {
		t.Error("Fetching() should be false before FetchAll")
	}

	c.FetchAll([]igdb.GameEntry{
		{Console: "NES", Filename: "Game.nes"},
	})

	if c.Fetching() {
		t.Error("Fetching() should be false after FetchAll completes")
	}
}

// TestIsCachedConsistencyAfterSaveDetails verifies that isCached returns
// true immediately after saveDetails succeeds for the same game. This is
// the core invariant that ensures the re-scan loop terminates.
func TestIsCachedConsistencyAfterSaveDetails(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*igdb.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Before fetching, isCached should return false
	if c.isCached("NES", "Mega Man") {
		t.Fatal("isCached should be false before fetching")
	}

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	}

	count := c.FetchAll(entries)
	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// After fetching, isCached should return true
	if !c.isCached("NES", "Mega Man") {
		t.Error(
			"BUG: isCached returns false immediately after saveDetails " +
				"succeeded. This would cause an infinite re-scan loop " +
				"in main.go.",
		)
	}
}

// TestIsCachedAfterNotFound verifies that isCached returns true after
// writeNotFound is called (game not found on IGDB).
func TestIsCachedAfterNotFound(t *testing.T) {
	// Fetcher returns 0 for all searches (game not found)
	fetcher := &mockIGDBFetcher{}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Unknown Game.nes"},
	}

	c.FetchAll(entries)

	// After not-found marker, isCached should return true
	if !c.isCached("NES", "Unknown Game") {
		t.Error(
			"BUG: isCached returns false after writeNotFound. This " +
				"would cause an infinite re-scan loop for unknown games.",
		)
	}
}

// TestIsCachedAfterTransientSearchError verifies that isCached returns
// false after a transient search error. The game should be retried on
// the next pass, but this means FetchAll will return saved == 0 (no
// games were saved), so no re-scan is triggered.
func TestIsCachedAfterTransientSearchError(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchErr: errTransient,
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	}

	count := c.FetchAll(entries)

	// Transient error: saved should be 0
	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on transient error", count)
	}

	// isCached should still be false (no marker written)
	if c.isCached("NES", "Mega Man") {
		t.Error(
			"isCached should be false after transient error " +
				"(no marker written)",
		)
	}
}

// TestFetchingFlagConcurrentFetchAll verifies that when two FetchAll calls
// run concurrently on the same Cache, the Fetching() flag remains true until
// BOTH complete. Currently, FetchAll uses a simple Store(true)/defer
// Store(false) pattern which means the first goroutine to finish will set
// Fetching()=false while the second is still running.
//
// This can happen in practice when an HTTP rescan triggers onScanComplete
// (spawning a FetchAll goroutine) while a previous FetchAll goroutine from
// the initial scan is still in progress.
func TestFetchingFlagConcurrentFetchAll(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	fetcher := &mockIGDBFetcher{
		entered: entered,
		release: release,
	}

	c := New(t.TempDir(), fetcher)

	// Start first FetchAll with a game that will block in SearchGame
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		c.FetchAll([]igdb.GameEntry{
			{Console: "NES", Filename: "Game.nes"},
		})
	}()

	// Wait for first FetchAll to block in SearchGame
	<-entered

	if !c.Fetching() {
		t.Fatal("Fetching() should be true while FetchAll is blocked")
	}

	// Start a second FetchAll with no games (completes immediately).
	// It sets fetching=true then immediately sets fetching=false.
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		c.FetchAll([]igdb.GameEntry{})
	}()
	<-done2

	// BUG: The second FetchAll completed and set Fetching()=false,
	// even though the first FetchAll is still blocked in SearchGame.
	if !c.Fetching() {
		t.Error(
			"BUG: Fetching() returns false while a FetchAll call is still " +
				"in progress. A concurrent FetchAll that completed first " +
				"reset the flag prematurely. The Fetching() flag should use " +
				"reference counting or a mutex to handle concurrent calls.",
		)
	}

	// Cleanup: release the blocked FetchAll
	close(release)
	<-done1
}

// --- test helpers ---

var errTransient = errType("transient network error")

type errType string

func (e errType) Error() string { return string(e) }

// countingMockFetcher wraps mockIGDBFetcher and counts fetches atomically.
type countingMockFetcher struct {
	searchResults  map[string]int
	detailsResults map[int]*igdb.GameDetails
	fetchCount     *atomic.Int32
}

func (m *countingMockFetcher) SearchGame(
	gameName string, _ []int,
) (int, error) {
	if m.searchResults == nil {
		return 0, nil
	}
	return m.searchResults[gameName], nil
}

func (m *countingMockFetcher) FetchDetailsByID(
	gameID int,
) (*igdb.GameDetails, error) {
	m.fetchCount.Add(1)
	if m.detailsResults == nil {
		return nil, nil
	}
	return m.detailsResults[gameID], nil
}
