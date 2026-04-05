package details

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/covers"
)

// mockIGDBFetcher is a test double for igdbFetcher.
type mockIGDBFetcher struct {
	// searchResults maps gameName -> gameID (0 = not found)
	searchResults map[string]int
	// detailsResults maps gameID -> GameDetails (nil = not found)
	detailsResults map[int]*covers.GameDetails
	searchCalls    int
	detailsCalls   int

	// searchErr, if non-nil, is returned by every SearchGame call.
	searchErr error
	// detailsErr, if non-nil, is returned by every FetchDetailsByID call.
	detailsErr error
	// searchPlatformArgs records the platformIDs slice from each SearchGame
	// call, in call order.
	searchPlatformArgs [][]int

	// blockSearch, if non-nil, is used to synchronise the fetching-flag
	// test. SearchGame signals entered by closing (or sending on) the
	// channel, then blocks until release is closed.
	entered chan struct{}
	release chan struct{}
}

func (m *mockIGDBFetcher) SearchGame(
	gameName string, platformIDs []int,
) (int, error) {
	m.searchCalls++
	// Record a copy of platformIDs for later inspection.
	var ids []int
	if platformIDs != nil {
		ids = append([]int(nil), platformIDs...)
	}
	m.searchPlatformArgs = append(m.searchPlatformArgs, ids)

	// If blocking channels are set, signal entry and wait for release.
	if m.entered != nil {
		close(m.entered)
		<-m.release
	}

	if m.searchErr != nil {
		return 0, m.searchErr
	}
	if m.searchResults == nil {
		return 0, nil
	}
	return m.searchResults[gameName], nil
}

func (m *mockIGDBFetcher) FetchDetailsByID(
	gameID int,
) (*covers.GameDetails, error) {
	m.detailsCalls++
	if m.detailsErr != nil {
		return nil, m.detailsErr
	}
	if m.detailsResults == nil {
		return nil, nil
	}
	return m.detailsResults[gameID], nil
}

func FuzzCacheGet(f *testing.F) {
	f.Add("NES", "Mega Man.nes")
	f.Add("", "")
	f.Add("NES", "(tag only).nes")
	f.Add("../evil", "game.nes")

	f.Fuzz(func(t *testing.T, console, romFilename string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"Cache.Get panicked on (%q, %q): %v",
					console, romFilename, r,
				)
			}
		}()

		dir := t.TempDir()
		c := New(dir, nil)

		result := c.Get(console, romFilename)

		// Result must be nil or have a non-empty Name.
		if result != nil && result.Name == "" {
			t.Errorf(
				"Cache.Get(%q, %q) returned details with empty Name",
				console, romFilename,
			)
		}
	})
}

func FuzzCacheGetMalformedJSON(f *testing.F) {
	f.Add("NES", "Mega Man.nes", []byte(`{invalid json`))
	f.Add("NES", "Game.nes", []byte(`null`))
	f.Add("NES", "Game.nes", []byte(`[]`))
	f.Add("NES", "Game.nes", []byte(``))
	f.Add("NES", "Game.nes", []byte(`{"name":""}`))

	f.Fuzz(func(t *testing.T, console, romFilename string, jsonData []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"Cache.Get panicked on (%q, %q) with JSON %q: %v",
					console, romFilename, jsonData, r,
				)
			}
		}()

		dir := t.TempDir()
		c := New(dir, nil)

		// Derive the cache path the same way the implementation does, so
		// the written file is actually found by Get.
		_, cleanName := covers.CleanFilename(romFilename)
		if cleanName != "" && console != "" {
			cacheDir := filepath.Join(
				dir, "cache", "igdb", console, cleanName,
			)
			if err := os.MkdirAll(cacheDir, 0o755); err == nil {
				// Ignore write errors — the fuzz input may produce a path
				// that is unwritable on this OS; Get must still not panic.
				_ = os.WriteFile(
					filepath.Join(cacheDir, "details.json"),
					jsonData, 0o644,
				)
			}
		}

		// The only invariant here is no panic; arbitrary JSON bytes may
		// produce a non-nil result with an empty Name field, and that is
		// acceptable behaviour for the malformed-input path.
		_ = c.Get(console, romFilename)
	})
}

func TestGet_CacheHit(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	details := &covers.GameDetails{
		Name:    "Mega Man",
		Summary: "A platformer.",
	}
	cacheDir := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(details)
	if err := os.WriteFile(
		filepath.Join(cacheDir, "details.json"), data, 0o644,
	); err != nil {
		t.Fatal(err)
	}

	got := c.Get("NES", "Mega Man (USA).nes")
	if got == nil {
		t.Fatal("expected non-nil result for cached game")
	}
	if got.Name != "Mega Man" {
		t.Errorf("Name = %q, want %q", got.Name, "Mega Man")
	}
	if got.Summary != "A platformer." {
		t.Errorf("Summary = %q, want %q", got.Summary, "A platformer.")
	}
}

func TestGet_CacheMiss(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	got := c.Get("NES", "Nonexistent Game.nes")
	if got != nil {
		t.Errorf("expected nil for uncached game, got %+v", got)
	}
}

func TestGet_EmptyCleanName(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	got := c.Get("NES", "(tag only).nes")
	if got != nil {
		t.Errorf("expected nil for empty clean name, got %+v", got)
	}
}

func TestFetchAll_NilFetcher(t *testing.T) {
	c := New(t.TempDir(), nil)
	got := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	})
	if got != 0 {
		t.Errorf("FetchAll with nil fetcher = %d, want 0", got)
	}
}

func TestFetchAll_PopulatesCache(t *testing.T) {
	// Set up a fake image server so downloadImage succeeds
	imgServer := startFakeImageServer(t)

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {
				Name:     "Mega Man",
				Summary:  "A platformer.",
				CoverURL: coverURL,
			},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	if count != 1 {
		t.Errorf("FetchAll() = %d, want 1", count)
	}

	// details.json should exist
	jsonPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", "details.json",
	)
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("expected details.json at %q, got: %v", jsonPath, err)
	}

	// Cover thumbnail should exist at standard cover path
	coverPath := covers.CoverPath(dir, "NES", "Mega Man (USA)")
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("expected cover at %q, got: %v", coverPath, err)
	}
}

func TestFetchAll_SkipsExisting(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man"},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Pre-create the cache entry
	cacheDir := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, "details.json"),
		[]byte(`{"name":"Mega Man"}`), 0o644,
	); err != nil {
		t.Fatal(err)
	}

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 (already cached)", count)
	}
	if fetcher.searchCalls != 0 {
		t.Errorf(
			"SearchGame called %d times, want 0 (skipped)",
			fetcher.searchCalls,
		)
	}
}

func TestFetchAll_NotFoundMarker(t *testing.T) {
	fetcher := &mockIGDBFetcher{} // all searches return 0

	dir := t.TempDir()
	c := New(dir, fetcher)

	// First call: no match, writes .notfound
	c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Unknown Game.nes"},
	})

	notFoundPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Unknown Game", ".notfound",
	)
	if _, err := os.Stat(notFoundPath); err != nil {
		t.Errorf("expected .notfound marker at %q", notFoundPath)
	}

	// Reset call counts
	fetcher.searchCalls = 0

	// Second call: should be skipped due to .notfound marker
	c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Unknown Game.nes"},
	})

	if fetcher.searchCalls != 0 {
		t.Errorf(
			"SearchGame called %d times on second run, want 0",
			fetcher.searchCalls,
		)
	}
}

func TestFetchAll_RegionalVariantsShareCache(t *testing.T) {
	// Set up a fake image server
	imgServer := startFakeImageServer(t)

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {Name: "Mega Man", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Two regional variants of the same game
	entries := []covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
		{Console: "NES", Filename: "Mega Man (Japan).nes"},
	}

	count := c.FetchAll(entries)

	// Only one IGDB fetch, but both entries processed
	if count != 1 {
		t.Errorf("FetchAll() = %d, want 1 (one new cache entry)", count)
	}
	if fetcher.detailsCalls != 1 {
		t.Errorf(
			"FetchDetailsByID called %d times, want 1",
			fetcher.detailsCalls,
		)
	}

	// Both ROMs should have cover thumbnails
	for _, name := range []string{"Mega Man (USA)", "Mega Man (Japan)"} {
		cp := covers.CoverPath(dir, "NES", name)
		if _, err := os.Stat(cp); err != nil {
			t.Errorf("expected cover at %q, got: %v", cp, err)
		}
	}
}

func TestFetchAll_FetchingFlag(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fetcher := &mockIGDBFetcher{
		entered: entered,
		release: release,
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Not fetching before start.
	if c.Fetching() {
		t.Error("expected Fetching()=false before FetchAll")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		c.FetchAll([]covers.GameEntry{
			{Console: "NES", Filename: "Game.nes"},
		})
	}()

	// Wait until SearchGame is entered — FetchAll must have set the flag.
	<-entered
	if !c.Fetching() {
		t.Error("expected Fetching()=true while FetchAll is running")
	}

	// Unblock SearchGame and wait for FetchAll to return.
	close(release)
	<-done

	// Not fetching after completion.
	if c.Fetching() {
		t.Error("expected Fetching()=false after FetchAll completes")
	}
}

func TestFetchAll_URLsRewrittenToLocalPaths(t *testing.T) {
	imgServer := startFakeImageServer(t)

	imgURL := imgServer.URL + "/img.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {
				Name:        "Mega Man",
				CoverURL:    imgURL,
				Screenshots: []string{imgURL},
				Artworks:    []string{imgURL},
			},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man.nes"},
	})

	got := c.Get("NES", "Mega Man.nes")
	if got == nil {
		t.Fatal("expected cached details")
	}
	if !strings.HasPrefix(got.CoverURL, "/cache/igdb/") {
		t.Errorf("CoverURL not rewritten: %q", got.CoverURL)
	}
	if len(got.Screenshots) != 1 {
		t.Fatalf("Screenshots len = %d, want 1", len(got.Screenshots))
	}
	if !strings.HasPrefix(got.Screenshots[0], "/cache/igdb/") {
		t.Errorf("Screenshot URL not rewritten: %q", got.Screenshots[0])
	}
	if len(got.Artworks) != 1 {
		t.Fatalf("Artworks len = %d, want 1", len(got.Artworks))
	}
	if !strings.HasPrefix(got.Artworks[0], "/cache/igdb/") {
		t.Errorf("Artwork URL not rewritten: %q", got.Artworks[0])
	}
}

// TestFetchAll_NameVariantFallback verifies that when the primary search name
// returns no result but a colon-substituted variant does, FetchAll still
// caches the game. This exercises the covers.NameVariants fallback path where
// No-Intro " - " subtitle separators are tried as IGDB ": " separators.
func TestFetchAll_NameVariantFallback(t *testing.T) {
	imgServer := startFakeImageServer(t)

	coverURL := imgServer.URL + "/cover.jpg"

	// "Game - Subtitle" returns 0; "Game: Subtitle" (the colon variant
	// generated by NameVariants) returns 42.
	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{
			"Game: Subtitle": 42,
		},
		detailsResults: map[int]*covers.GameDetails{
			42: {Name: "Game: Subtitle", CoverURL: coverURL},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Game - Subtitle.nes"},
	})

	if count != 1 {
		t.Errorf("FetchAll() = %d, want 1 (name variant fallback)", count)
	}
}

// TestFetchAll_DetailsError verifies that when FetchDetailsByID returns an
// error, FetchAll returns 0 and no details.json is written.
func TestFetchAll_DetailsError(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsErr:    errors.New("IGDB API unavailable"),
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on details error", count)
	}

	jsonPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", "details.json",
	)
	if _, err := os.Stat(jsonPath); err == nil {
		t.Errorf(
			"expected no details.json after details fetch error, but found one",
		)
	}
}

// TestFetchAll_DetailsNil verifies that when FetchDetailsByID returns
// (nil, nil), FetchAll writes a .notfound marker and returns 0.
func TestFetchAll_DetailsNil(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		// detailsResults is nil so FetchDetailsByID returns (nil, nil).
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 when details are nil", count)
	}

	notFoundPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", ".notfound",
	)
	if _, err := os.Stat(notFoundPath); err != nil {
		t.Errorf(
			"expected .notfound marker at %q, got: %v",
			notFoundPath, err,
		)
	}
}

// TestFetchAll_PlatformConstrainedSearch verifies that when a game entry
// carries IGDBPlatformIDs, the first SearchGame call is made with those IDs
// (platform-constrained) before falling back to an unconstrained search.
func TestFetchAll_PlatformConstrainedSearch(t *testing.T) {
	// searchResults is empty so all searches return 0. We only care that the
	// constrained call was made first.
	fetcher := &mockIGDBFetcher{}

	dir := t.TempDir()
	c := New(dir, fetcher)

	c.FetchAll([]covers.GameEntry{
		{
			Console:         "NES",
			Filename:        "Mega Man (USA).nes",
			IGDBPlatformIDs: []int{18},
		},
	})

	if len(fetcher.searchPlatformArgs) == 0 {
		t.Fatal("SearchGame was never called")
	}

	// The very first call must be platform-constrained.
	first := fetcher.searchPlatformArgs[0]
	if len(first) == 0 {
		t.Errorf("first SearchGame call had no platformIDs, want [18]")
	} else if first[0] != 18 {
		t.Errorf(
			"first SearchGame platformIDs = %v, want [18]",
			first,
		)
	}

	// A subsequent call must be unconstrained (nil platformIDs).
	foundUnconstrained := false
	for _, ids := range fetcher.searchPlatformArgs[1:] {
		if ids == nil {
			foundUnconstrained = true
			break
		}
	}
	if !foundUnconstrained {
		t.Errorf(
			"no unconstrained SearchGame call found; all calls: %v",
			fetcher.searchPlatformArgs,
		)
	}
}

// TestFetchAll_SearchError verifies that when SearchGame returns an error,
// FetchAll returns 0, no details.json is written, and no .notfound marker is
// written (errors are transient and must not permanently suppress retries).
func TestFetchAll_SearchError(t *testing.T) {
	fetcher := &mockIGDBFetcher{
		searchErr: errors.New("network timeout"),
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	if count != 0 {
		t.Errorf("FetchAll() = %d, want 0 on search error", count)
	}

	base := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man")

	if _, err := os.Stat(filepath.Join(base, "details.json")); err == nil {
		t.Errorf("expected no details.json after search error, but found one")
	}

	if _, err := os.Stat(filepath.Join(base, ".notfound")); err == nil {
		t.Errorf(
			"expected no .notfound marker after transient search error, " +
				"but found one",
		)
	}
}

// TestFetchAll_ImageDownloadFailure verifies that when the image server
// returns 404, FetchAll still succeeds (count=1, details.json written) and
// the CoverURL is cleared rather than left as a remote URL.
func TestFetchAll_ImageDownloadFailure(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}),
	)
	defer imgServer.Close()

	coverURL := imgServer.URL + "/cover.jpg"

	fetcher := &mockIGDBFetcher{
		searchResults: map[string]int{"Mega Man": 17},
		detailsResults: map[int]*covers.GameDetails{
			17: {
				Name:        "Mega Man",
				Summary:     "A platformer.",
				CoverURL:    coverURL,
				Screenshots: []string{imgServer.URL + "/ss0.jpg"},
				Artworks:    []string{imgServer.URL + "/art0.jpg"},
			},
		},
	}

	dir := t.TempDir()
	c := New(dir, fetcher)

	count := c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Mega Man (USA).nes"},
	})

	// A missing image is not fatal — the overall fetch must still succeed.
	if count != 1 {
		t.Errorf(
			"FetchAll() = %d, want 1 even when image download fails",
			count,
		)
	}

	// details.json should still be written.
	jsonPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Mega Man", "details.json",
	)
	if _, err := os.Stat(jsonPath); err != nil {
		t.Errorf("expected details.json at %q, got: %v", jsonPath, err)
	}

	// Failed downloads must clear the URL rather than leave a remote URL
	// that the frontend would try to load directly.
	got := c.Get("NES", "Mega Man (USA).nes")
	if got == nil {
		t.Fatal("expected cached details after image-download failure")
	}
	if got.CoverURL != "" {
		t.Errorf("CoverURL should be empty after download failure, got %q", got.CoverURL)
	}
	if len(got.Screenshots) != 0 {
		t.Errorf("Screenshots should be empty after download failure, got %v", got.Screenshots)
	}
	if len(got.Artworks) != 0 {
		t.Errorf("Artworks should be empty after download failure, got %v", got.Artworks)
	}
}
