package details

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/covers"
)

// platformAwareMockFetcher extends mockIGDBFetcher to make SearchGame
// results dependent on whether platformIDs is nil (unconstrained) or
// non-nil (constrained). This is needed to test the two-phase search
// logic in cache.search().
type platformAwareMockFetcher struct {
	// constrainedResults maps gameName -> gameID for constrained searches
	constrainedResults map[string]int
	// unconstrainedResults maps gameName -> gameID for unconstrained searches
	unconstrainedResults map[string]int
	// detailsResults maps gameID -> GameDetails
	detailsResults map[int]*covers.GameDetails

	searchCalls        int
	detailsCalls       int
	searchPlatformArgs [][]int

	// searchErrOn, if non-nil, maps call index (0-based) -> error.
	// If a call index is present, that call returns the mapped error.
	searchErrOn map[int]error
}

func (m *platformAwareMockFetcher) SearchGame(
	gameName string, platformIDs []int,
) (int, error) {
	callIdx := m.searchCalls
	m.searchCalls++

	var ids []int
	if platformIDs != nil {
		ids = append([]int(nil), platformIDs...)
	}
	m.searchPlatformArgs = append(m.searchPlatformArgs, ids)

	if m.searchErrOn != nil {
		if err, ok := m.searchErrOn[callIdx]; ok {
			return 0, err
		}
	}

	if platformIDs != nil {
		if m.constrainedResults != nil {
			return m.constrainedResults[gameName], nil
		}
	} else {
		if m.unconstrainedResults != nil {
			return m.unconstrainedResults[gameName], nil
		}
	}
	return 0, nil
}

func (m *platformAwareMockFetcher) FetchDetailsByID(
	gameID int,
) (*covers.GameDetails, error) {
	m.detailsCalls++
	if m.detailsResults == nil {
		return nil, nil
	}
	return m.detailsResults[gameID], nil
}

// TestSearch_ConstrainedMatchSkipsUnconstrained verifies that when the
// platform-constrained search finds a match, the unconstrained search
// is not attempted. This is the early-return path at line 169 of
// cache.go that was identified as having limited test coverage.
func TestSearch_ConstrainedMatchSkipsUnconstrained(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &platformAwareMockFetcher{
		constrainedResults: map[string]int{"Mega Man": 17},
		// unconstrainedResults deliberately has a different ID. If the
		// unconstrained path is reached, FetchDetailsByID would get a
		// different ID, and we can detect it.
		unconstrainedResults: map[string]int{"Mega Man": 99},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
			99: {Name: "Mega Man Wrong", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Mega Man.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// Verify only constrained calls were made (no unconstrained calls).
	for i, ids := range fetcher.searchPlatformArgs {
		if ids == nil {
			t.Errorf(
				"SearchGame call %d was unconstrained (platformIDs=nil), "+
					"but constrained search found a match on the first variant. "+
					"No unconstrained calls should have been made.",
				i,
			)
		}
	}

	// Verify the correct game was cached (from constrained, not unconstrained).
	got := c.Get("NES", "Mega Man.nes")
	if got == nil {
		t.Fatal("expected cached details")
	}
	if got.Name != "Mega Man" {
		t.Errorf("Name = %q, want %q (constrained result)", got.Name, "Mega Man")
	}
}

// TestSearch_ConstrainedMatchOnSecondVariant verifies that when the first
// variant fails to match in constrained search but the second variant
// matches, the search stops and returns that match without trying
// unconstrained search.
func TestSearch_ConstrainedMatchOnSecondVariant(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	// "Game - Subtitle" (first variant) returns 0 in constrained search.
	// "Game: Subtitle" (second variant) returns 42 in constrained search.
	fetcher := &platformAwareMockFetcher{
		constrainedResults: map[string]int{
			"Game: Subtitle": 42,
		},
		unconstrainedResults: map[string]int{
			"Game: Subtitle": 99,
		},
		detailsResults: map[int]*covers.GameDetails{
			42: {Name: "Game: Subtitle", CoverURL: coverURL},
			99: {Name: "Wrong Game", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Game - Subtitle.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// Verify no unconstrained calls were made.
	for i, ids := range fetcher.searchPlatformArgs {
		if ids == nil {
			t.Errorf(
				"SearchGame call %d was unconstrained, but constrained "+
					"search found a match on the second variant",
				i,
			)
		}
	}

	// Exactly 2 constrained calls should be made (first variant miss,
	// second variant hit).
	if fetcher.searchCalls != 2 {
		t.Errorf(
			"SearchGame called %d times, want 2 "+
				"(first variant miss + second variant hit)",
			fetcher.searchCalls,
		)
	}
}

// TestSearch_ErrorOnConstrainedAbortsSearch verifies that when a
// constrained search returns an error, the function returns that error
// immediately without trying remaining variants or falling through to
// unconstrained search. The caller (fetchOne) must not write .notfound
// in this case.
func TestSearch_ErrorOnConstrainedAbortsSearch(t *testing.T) {
	fetcher := &platformAwareMockFetcher{
		// The first call (constrained, first variant) errors.
		searchErrOn: map[int]error{
			0: errors.New("network timeout"),
		},
		// Even though unconstrained would find a match, it should never
		// be reached.
		unconstrainedResults: map[string]int{"Mega Man": 42},
		detailsResults: map[int]*covers.GameDetails{
			42: {Name: "Mega Man"},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Mega Man.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on search error", count)
	}

	// Only 1 call should have been made (the one that errored).
	if fetcher.searchCalls != 1 {
		t.Errorf(
			"SearchGame called %d times, want 1 (abort after error)",
			fetcher.searchCalls,
		)
	}

	// No .notfound marker should be written for transient errors.
	base := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
	if _, err := os.Stat(filepath.Join(base, ".notfound")); err == nil {
		t.Error(
			"expected no .notfound marker after transient search error, " +
				"but found one",
		)
	}

	// No details.json should exist either.
	if _, err := os.Stat(filepath.Join(base, "details.json")); err == nil {
		t.Error("expected no details.json after search error, but found one")
	}
}

// TestSearch_NoPlatformIDs_OnlyUnconstrainedCalls verifies that when
// platformIDs is empty, only unconstrained search calls are made (the
// constrained loop is skipped entirely).
func TestSearch_NoPlatformIDs_OnlyUnconstrainedCalls(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &platformAwareMockFetcher{
		constrainedResults:   map[string]int{"Mega Man": 99},
		unconstrainedResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:  "NES",
			Filename: "Mega Man.nes",
			// No IGDBPlatformIDs — constrained path should be skipped
		},
	})

	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// All calls should be unconstrained.
	for i, ids := range fetcher.searchPlatformArgs {
		if ids != nil {
			t.Errorf(
				"SearchGame call %d had platformIDs=%v, "+
					"expected nil (unconstrained only)",
				i, ids,
			)
		}
	}
}

// TestSearch_ConstrainedMiss_UnconstrainedHit verifies the full
// two-phase fallback: all constrained variants miss, then unconstrained
// search finds the game.
func TestSearch_ConstrainedMiss_UnconstrainedHit(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &platformAwareMockFetcher{
		// Constrained search finds nothing
		constrainedResults: map[string]int{},
		// Unconstrained search finds the game
		unconstrainedResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Mega Man.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// Should have both constrained and unconstrained calls.
	hasConstrained := false
	hasUnconstrained := false
	for _, ids := range fetcher.searchPlatformArgs {
		if ids != nil {
			hasConstrained = true
		} else {
			hasUnconstrained = true
		}
	}
	if !hasConstrained {
		t.Error("expected at least one constrained search call")
	}
	if !hasUnconstrained {
		t.Error("expected at least one unconstrained search call")
	}
}

// TestSearch_ConstrainedErrorOnSecondVariant verifies that when the
// first constrained variant returns "not found" (id=0) but the second
// constrained variant returns an error, the search aborts with error.
// This tests that errors on non-first variants are properly handled.
func TestSearch_ConstrainedErrorOnSecondVariant(t *testing.T) {
	fetcher := &platformAwareMockFetcher{
		// First call (constrained, first variant) returns 0 (not found).
		// Second call (constrained, second variant) returns error.
		searchErrOn: map[int]error{
			1: errors.New("server error"),
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Game - Subtitle.nes", // has multiple variants
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on error", count)
	}

	// Exactly 2 calls: first variant OK (id=0), second variant error.
	if fetcher.searchCalls != 2 {
		t.Errorf(
			"SearchGame called %d times, want 2",
			fetcher.searchCalls,
		)
	}

	// No .notfound marker.
	base := filepath.Join(dir, "cache", "igdb", "NES", "Game - Subtitle")
	if _, err := os.Stat(filepath.Join(base, ".notfound")); err == nil {
		t.Error("expected no .notfound after search error")
	}
}

// TestSearch_AllVariantsNotFound_WritesNotFound verifies that when all
// variants (both constrained and unconstrained) return 0, the .notfound
// marker is written and the total number of API calls equals
// variants * 2 (constrained + unconstrained).
func TestSearch_AllVariantsNotFound_WritesNotFound(t *testing.T) {
	fetcher := &platformAwareMockFetcher{
		constrainedResults:   map[string]int{},
		unconstrainedResults: map[string]int{},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Unknown - Game.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0", count)
	}

	// Verify .notfound marker exists.
	base := filepath.Join(dir, "cache", "igdb", "NES", "Unknown - Game")
	if _, err := os.Stat(filepath.Join(base, ".notfound")); err != nil {
		t.Errorf("expected .notfound marker at %q, got: %v", base, err)
	}

	// Count the calls. "Unknown - Game" produces these variants:
	// 1. "Unknown - Game" (original)
	// 2. "Unknown: Game" (dash-to-colon)
	// 3. "Unknown-Game" (spaces removed)
	// 4. "Unknown" (subtitle dropped)
	// Total: 4 variants * 2 (constrained + unconstrained) = 8 calls
	variants := covers.NameVariants("Unknown - Game")
	expectedCalls := len(variants) * 2
	if fetcher.searchCalls != expectedCalls {
		t.Errorf(
			"SearchGame called %d times, want %d "+
				"(%d variants * 2 phases)",
			fetcher.searchCalls, expectedCalls, len(variants),
		)
	}
}

// TestSearch_EmptyPlatformIDsSlice verifies that an empty (non-nil)
// platformIDs slice is treated the same as nil: the constrained search
// phase is skipped.
func TestSearch_EmptyPlatformIDsSlice(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &platformAwareMockFetcher{
		constrainedResults:   map[string]int{"Mega Man": 99},
		unconstrainedResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Mega Man.nes",
			IGDBPlatformIDs: []int{}, // empty slice, not nil
		},
	})

	if count != 1 {
		t.Fatalf("FetchAll() = %d, want 1", count)
	}

	// All calls should be unconstrained since empty slice is treated
	// as "no platform IDs".
	for i, ids := range fetcher.searchPlatformArgs {
		if ids != nil {
			t.Errorf(
				"SearchGame call %d had platformIDs=%v, "+
					"expected nil (empty slice should skip constrained)",
				i, ids,
			)
		}
	}
}

// TestSearch_ErrorOnUnconstrainedAfterConstrainedMiss verifies that
// when the constrained search completes with all misses, then the
// unconstrained search hits an error, the error is returned and no
// .notfound marker is written.
func TestSearch_ErrorOnUnconstrainedAfterConstrainedMiss(t *testing.T) {
	// "Metroid" produces 1 variant. With platformIDs=[18]:
	// Call 0: constrained "Metroid" -> returns 0 (miss)
	// Call 1: unconstrained "Metroid" -> errors
	fetcher := &platformAwareMockFetcher{
		constrainedResults: map[string]int{},
		searchErrOn: map[int]error{
			1: errors.New("connection reset"),
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Metroid.nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on error", count)
	}

	// 2 calls: 1 constrained (miss) + 1 unconstrained (error)
	if fetcher.searchCalls != 2 {
		t.Errorf("SearchGame called %d times, want 2", fetcher.searchCalls)
	}

	// First call should be constrained, second unconstrained.
	if len(fetcher.searchPlatformArgs) >= 1 && fetcher.searchPlatformArgs[0] == nil {
		t.Error("first call should be constrained (non-nil platformIDs)")
	}
	if len(fetcher.searchPlatformArgs) >= 2 && fetcher.searchPlatformArgs[1] != nil {
		t.Error("second call should be unconstrained (nil platformIDs)")
	}

	// No .notfound marker.
	base := filepath.Join(dir, "cache", "igdb", "NES", "Metroid")
	if _, err := os.Stat(filepath.Join(base, ".notfound")); err == nil {
		t.Error("expected no .notfound after unconstrained error")
	}
}
