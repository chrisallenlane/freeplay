package details

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// --- Probes around the FetchDetailsByID-error path (Phase 2). ---
//
// fetchOne's phase 2 error branch returns false without writing a
// .notfound marker AND without memoizing anything in the in-memory
// cache. The rationale is "transient errors must not tombstone." This
// file probes the consequences of that choice:
//
//   - Within a single FetchAll, does the same game's FetchDetailsByID
//     get retried when other games in the batch succeed (triggering
//     iter 1)?
//   - Across pipeline invocations (operator rescans), is there any
//     bound on how many times a permanently-broken ID can be hit?
//   - What error types does the caller see, and is there any way for
//     the caller to distinguish "transient" (5xx, network) from
//     "permanent" (4xx, malformed query) at this layer?

// idTrackingFetcher counts SearchGame/FetchDetailsByID calls so tests
// can audit phase-1/phase-2 amplification across multiple FetchAll
// invocations.
type idTrackingFetcher struct {
	// searchHits maps gameName -> gameID for SearchGame results.
	searchHits map[string]int
	// detailsErr is the error returned by every FetchDetailsByID call.
	detailsErr error

	searchCalls  atomic.Int32
	detailsCalls atomic.Int32
}

func (m *idTrackingFetcher) SearchGame(name string, _ []int) (int, error) {
	m.searchCalls.Add(1)
	if id, ok := m.searchHits[name]; ok {
		return id, nil
	}
	return 0, nil
}

func (m *idTrackingFetcher) FetchDetailsByID(_ int) (*igdb.GameDetails, error) {
	m.detailsCalls.Add(1)
	if m.detailsErr != nil {
		return nil, m.detailsErr
	}
	return nil, nil
}

// TestFetchOne_DetailsErrorRetriedAcrossFetchAllCalls confirms (or
// invalidates) the assessor's hypothesis: when FetchDetailsByID fails
// persistently, subsequent FetchAll invocations re-fetch the broken
// ID with no backoff or cap. This is the cross-pipeline-run case
// (operator rescans the library N times).
func TestFetchOne_DetailsErrorRetriedAcrossFetchAllCalls(t *testing.T) {
	fetcher := &idTrackingFetcher{
		searchHits: map[string]int{"Mega Man": 17},
		detailsErr: errors.New("IGDB internal error"),
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	}

	const rescans = 5
	for i := 0; i < rescans; i++ {
		c.FetchAll(entries)
	}

	// Document observed behaviour: every FetchAll re-fetches the
	// broken ID. That's exactly the quota-leak the assessor flagged.
	got := fetcher.detailsCalls.Load()
	if got != int32(rescans) {
		t.Errorf(
			"FetchDetailsByID called %d times across %d FetchAll runs, "+
				"want %d (one hit per rescan)",
			got, rescans, rescans,
		)
	}

	// Pin that no .notfound is written and no in-memory tombstone is
	// recorded — the design treats every error as transient.
	nf := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man", ".notfound")
	if _, err := os.Stat(nf); err == nil {
		t.Error(
			"unexpected .notfound after FetchDetailsByID error — phase 2 " +
				"should not tombstone (transient-error contract)",
		)
	}
	if c.isCached("NES", "Mega Man") {
		t.Error(
			"isCached returned true after FetchDetailsByID error — " +
				"in-memory cache should not memoize a transient failure",
		)
	}
}

// TestFetchOne_DetailsErrorAmplifiedByMixedBatch probes a subtler
// case: when a batch contains both a succeeding game and a phase-2
// erroring game, the successful save makes saved > 0 in iter 0, so
// FetchAll (called from runPipelineLocked) loops to iter 1. The
// erroring game has no tombstone and no memoization, so its phase 2
// is hit again in iter 1.
//
// Because the loop construct lives in library.runPipelineLocked, not
// in FetchAll itself, this test runs FetchAll TWICE — matching what
// the library does when saved > 0 — and asserts that the bad ID is
// hit on each pass.
func TestFetchOne_DetailsErrorAmplifiedByMixedBatch(t *testing.T) {
	imgServer := startFakeImageServer(t)

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{
			"Good Game": 1,
			"Bad Game":  2,
		},
		// Game 1 (Good) returns a real result; game 2 (Bad) returns
		// an error. detailsErr applies to all FetchDetailsByID calls,
		// so we need a per-ID variant. Use a custom fetcher.
	}
	// Drop into a custom fetcher for per-ID error injection.
	custom := &perIDFetcher{
		mockIGDBFetcher: fetcher,
		errByID: map[int]error{
			2: errors.New("IGDB 500"),
		},
		okByID: map[int]*igdb.GameDetails{
			1: {Name: "Good Game", CoverURL: imgServer.URL + "/c.jpg"},
		},
	}

	dir := t.TempDir()
	c := New(dir, custom)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Good Game.nes"},
		{Console: "NES", Filename: "Bad Game.nes"},
	}

	// First FetchAll: Good Game succeeds, Bad Game errors in phase 2.
	saved1 := c.FetchAll(entries)
	if saved1 != 1 {
		t.Errorf("first FetchAll = %d, want 1 (Good Game saved)", saved1)
	}
	hitsAfter1 := custom.detailsCallsByID[2]

	// Second FetchAll: simulates the library's iter-1 because saved>0.
	// Good Game is now cached (isCached=true) and is skipped — no
	// search, no details. Bad Game has no tombstone and no memo, so
	// fetchOne re-runs and FetchDetailsByID(2) is called again.
	saved2 := c.FetchAll(entries)
	if saved2 != 0 {
		t.Errorf("second FetchAll = %d, want 0 (Bad Game still errors)", saved2)
	}
	hitsAfter2 := custom.detailsCallsByID[2]

	if hitsAfter2 <= hitsAfter1 {
		t.Errorf(
			"FetchDetailsByID(2) hits did not increase between passes: "+
				"after pass 1 = %d, after pass 2 = %d. Pass 2 should "+
				"have re-tried the broken ID because no tombstone exists.",
			hitsAfter1, hitsAfter2,
		)
	}
	if got := custom.detailsCallsByID[1]; got != 1 {
		t.Errorf(
			"Good Game's FetchDetailsByID hits = %d, want 1 "+
				"(should be cached after pass 1, skipped on pass 2)",
			got,
		)
	}
}

// perIDFetcher wraps mockIGDBFetcher to inject per-ID details errors
// and per-ID details results. The base mock only supports a single
// detailsErr applied to every call.
type perIDFetcher struct {
	*mockIGDBFetcher
	errByID          map[int]error
	okByID           map[int]*igdb.GameDetails
	detailsCallsByID map[int]int
}

func (p *perIDFetcher) FetchDetailsByID(id int) (*igdb.GameDetails, error) {
	if p.detailsCallsByID == nil {
		p.detailsCallsByID = make(map[int]int)
	}
	p.detailsCallsByID[id]++
	if err, ok := p.errByID[id]; ok {
		return nil, err
	}
	if d, ok := p.okByID[id]; ok {
		return d, nil
	}
	return nil, nil
}

// TestFetchOne_DetailsErrorThenSuccessNoStaleTombstone documents the
// recovery path: a phase-2 error followed by a successful FetchAll
// (FetchDetailsByID returns a real result) must end with the game
// fully cached, no .notfound. This is the happy half of the
// "transient error" contract — without it, the no-tombstone policy
// would be inert.
func TestFetchOne_DetailsErrorThenSuccessNoStaleTombstone(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsErr:    errors.New("temporary network glitch"),
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	entries := []igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	}

	// First pass: phase 2 errors.
	if saved := c.FetchAll(entries); saved != 0 {
		t.Errorf("first FetchAll = %d, want 0 (error)", saved)
	}

	// Heal the fetcher and rerun.
	fetcher.detailsErr = nil
	fetcher.detailsResults = map[int]*igdb.GameDetails{
		17: {Name: "Mega Man", CoverURL: coverURL},
	}

	if saved := c.FetchAll(entries); saved != 1 {
		t.Errorf("second FetchAll = %d, want 1 (recovered)", saved)
	}

	// details.json present, .notfound absent.
	gameDir := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
	if _, err := os.Stat(filepath.Join(gameDir, "details.json")); err != nil {
		t.Errorf("details.json missing after recovery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(gameDir, ".notfound")); err == nil {
		t.Error("stale .notfound after recovery — phase-2 error must not tombstone")
	}
}

// TestFetchOne_DetailsErrorTypeIsIndistinguishable confirms the
// information loss at the fetchOne→IGDB boundary: a network error,
// an HTTP 5xx, an HTTP 4xx, and a malformed-payload error all flow
// through the same `slog.Warn + return false` branch. There is no
// mechanism for fetchOne to treat any of them as "permanent" and
// tombstone the game. This test documents the contract — if anyone
// adds error classification, they should update this test.
func TestFetchOne_DetailsErrorTypeIsIndistinguishable(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"plain", errors.New("connection reset")},
		{"api 500", &igdb.APIStatusError{Endpoint: "IGDB", Status: 500, Body: "boom"}},
		{"api 400", &igdb.APIStatusError{Endpoint: "IGDB", Status: 400, Body: "bad query"}},
		{"api 403", &igdb.APIStatusError{Endpoint: "IGDB", Status: 403, Body: "denied"}},
		{"api 429", &igdb.APIStatusError{Endpoint: "IGDB", Status: 429, Body: "rate limited"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &mockIGDBFetcher{
				searchResults: map[string]int{"Game": 1},
				detailsErr:    tc.err,
			}

			dir := t.TempDir()
			c := New(dir, fetcher)

			c.FetchAll([]igdb.GameEntry{
				{Console: "NES", Filename: "Game.nes"},
			})

			// Every shape produces the same on-disk state: no
			// details.json, no .notfound. fetchOne does not branch
			// on error kind.
			base := filepath.Join(dir, "cache", "igdb", "NES", "Game")
			if _, err := os.Stat(filepath.Join(base, "details.json")); err == nil {
				t.Errorf("%s: details.json written despite error", tc.name)
			}
			if _, err := os.Stat(filepath.Join(base, ".notfound")); err == nil {
				t.Errorf(
					"%s: .notfound written despite error — fetchOne now "+
						"distinguishes error kinds; update this test",
					tc.name,
				)
			}
		})
	}
}

// TestFetchOne_SaveDetailsErrorAlsoUntombstoned exercises the phase-3
// failure mode: a successful search + successful details fetch, but
// saveDetails returns an error (e.g., disk full). Same shape as the
// phase-2 path — no .notfound, no memoization, retried next pass.
//
// We force saveDetails to fail by blocking the cache directory with a
// regular file at a parent path. atomicfile.Write's MkdirAll then
// errors with "not a directory."
func TestFetchOne_SaveDetailsErrorAlsoUntombstoned(t *testing.T) {
	imgServer := startFakeImageServer(t)

	fetcher := newGameFetcher("Mega Man", 17, igdb.GameDetails{
		Name:     "Mega Man",
		CoverURL: imgServer.URL + "/cover.jpg",
	})

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Plant a regular file at <dir>/cache so MkdirAll fails when
	// atomicfile.Write tries to create <dir>/cache/igdb/NES/Mega Man.
	cacheRoot := filepath.Join(dir, "cache")
	if err := os.WriteFile(cacheRoot, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("planting blocker: %v", err)
	}

	saved := c.FetchAll([]igdb.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	})
	if saved != 0 {
		t.Errorf("FetchAll = %d, want 0 on saveDetails error", saved)
	}

	// .notfound cannot exist (the cache root itself is a regular file).
	// In-memory cache also must not have memoized success.
	if c.isCached("NES", "Mega Man") {
		t.Error(
			"isCached=true after saveDetails error — phase 3 must not " +
				"memoize a failed save (would skip retry while file " +
				"is still missing)",
		)
	}
}

// TestFetchOne_PhaseTwoSwallowsNonNilDetailsOnError pins the
// (nonNilDetails, err) contract: even if a future FetchDetailsByID
// returned both a value AND an error, fetchOne must drop the value
// and treat the call as a failure. This guards against an accidental
// future refactor of FetchDetailsByID where a partial success
// (e.g., parse error AFTER decoding some fields) sets both return
// values.
func TestFetchOne_PhaseTwoSwallowsNonNilDetailsOnError(t *testing.T) {
	imgServer := startFakeImageServer(t)
	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &nonNilOnErrorFetcher{
		searchHits: map[string]int{"Game": 1},
		details: &igdb.GameDetails{
			Name:     "Partially Parsed",
			CoverURL: coverURL,
		},
		err: errors.New("partial parse"),
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	saved := c.FetchAll([]igdb.GameEntry{
		{Console: "NES", Filename: "Game.nes"},
	})
	if saved != 0 {
		t.Errorf("FetchAll = %d, want 0 (error path)", saved)
	}

	// details.json must NOT have been written despite a non-nil details.
	jsonPath := filepath.Join(dir, "cache", "igdb", "NES", "Game", "details.json")
	if _, err := os.Stat(jsonPath); err == nil {
		t.Error(
			"details.json written despite FetchDetailsByID returning err; " +
				"fetchOne must check err before details",
		)
	}
}

type nonNilOnErrorFetcher struct {
	searchHits map[string]int
	details    *igdb.GameDetails
	err        error
}

func (f *nonNilOnErrorFetcher) SearchGame(name string, _ []int) (int, error) {
	return f.searchHits[name], nil
}

func (f *nonNilOnErrorFetcher) FetchDetailsByID(_ int) (*igdb.GameDetails, error) {
	return f.details, f.err
}
