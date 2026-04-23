package details

import (
	"os"
	"path/filepath"
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

	fetcher := newGameFetcher("Mega Man", 17, igdb.GameDetails{
		Name: "Mega Man", CoverURL: coverURL,
	})

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

// TestFetchAllAfterCacheCorruption verifies that on-disk corruption
// between FetchAll calls does NOT cause an infinite re-scan spin.
// The in-memory layer (PERF-4) memoizes the post-saveDetails state,
// so a subsequent FetchAll with the same entries returns saved=0
// regardless of whether details.json is still on disk. This is the
// invariant that lets the scan→fetch→rescan loop in internal/library
// terminate. A fresh Cache instance (e.g. after restart) would
// re-fetch — that path is covered by TestFetchAllIdempotent.
func TestFetchAllAfterCacheCorruption(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := newGameFetcher("Mega Man", 17, igdb.GameDetails{
		Name: "Mega Man", CoverURL: coverURL,
	})

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

	// Corrupt the cache: remove details.json from disk. The in-memory
	// layer should still report the game as cached on the next pass.
	jsonPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", "details.json",
	)
	if err := os.Remove(jsonPath); err != nil {
		t.Fatalf("removing details.json: %v", err)
	}

	// Second call: in-memory cache treats the game as cached; no fetch.
	count2 := c.FetchAll(entries)
	if count2 != 0 {
		t.Errorf(
			"FetchAll after on-disk corruption = %d, want 0 "+
				"(in-memory cache prevents re-fetch until restart)",
			count2,
		)
	}
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

	fetcher := newGameFetcher("Mega Man", 17, igdb.GameDetails{
		Name: "Mega Man", CoverURL: coverURL,
	})

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
// BOTH complete. FetchAll uses atomic reference counting (Add(1)/Add(-1)),
// so a second call completing early must not reset the flag while the first
// call is still in progress.
func TestFetchingFlagConcurrentFetchAll(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	fetcher := &mockIGDBFetcher{
		entered: entered,
		release: release,
	}

	c := New(t.TempDir(), fetcher)

	// Start first FetchAll with a game that will block in SearchGame.
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		c.FetchAll([]igdb.GameEntry{
			{Console: "NES", Filename: "Game.nes"},
		})
	}()

	// Wait for first FetchAll to block in SearchGame.
	<-entered

	if !c.Fetching() {
		t.Fatal("Fetching() should be true while first FetchAll is blocked")
	}

	// Start a second FetchAll with no games; it completes immediately.
	// With reference counting, this increments then decrements the counter,
	// but the counter remains > 0 because the first FetchAll is still running.
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		c.FetchAll([]igdb.GameEntry{})
	}()
	<-done2

	// Fetching() must still be true: the first FetchAll has not finished.
	if !c.Fetching() {
		t.Error(
			"Fetching() returned false while first FetchAll is still running. " +
				"Reference counting (Add/Add) must keep the flag true until " +
				"all concurrent FetchAll calls complete.",
		)
	}

	// Release the blocked FetchAll and wait for it to finish.
	close(release)
	<-done1

	// Now both FetchAll calls have completed; Fetching() must be false.
	if c.Fetching() {
		t.Error(
			"Fetching() should be false after all FetchAll calls complete",
		)
	}
}

// --- test helpers ---

var errTransient = errType("transient network error")

type errType string

func (e errType) Error() string { return string(e) }
