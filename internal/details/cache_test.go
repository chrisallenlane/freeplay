package details

import (
	"encoding/json"
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
}

func (m *mockIGDBFetcher) SearchGame(
	gameName string, _ []int,
) (int, error) {
	m.searchCalls++
	if m.searchResults == nil {
		return 0, nil
	}
	return m.searchResults[gameName], nil
}

func (m *mockIGDBFetcher) FetchDetailsByID(
	gameID int,
) (*covers.GameDetails, error) {
	m.detailsCalls++
	if m.detailsResults == nil {
		return nil, nil
	}
	return m.detailsResults[gameID], nil
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
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

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
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

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
	// A fetcher that checks the Fetching() flag while running
	fetcher := &mockIGDBFetcher{} // returns 0 for all searches

	dir := t.TempDir()
	c := New(dir, fetcher)

	// Not fetching before start
	if c.Fetching() {
		t.Error("expected Fetching()=false before FetchAll")
	}

	c.FetchAll([]covers.GameEntry{
		{Console: "NES", Filename: "Game.nes"},
	})

	// Not fetching after completion
	if c.Fetching() {
		t.Error("expected Fetching()=false after FetchAll completes")
	}
}

func TestFetchAll_URLsRewrittenToLocalPaths(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	defer imgServer.Close()

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
	if len(got.Screenshots) > 0 &&
		!strings.HasPrefix(got.Screenshots[0], "/cache/igdb/") {
		t.Errorf("Screenshot URL not rewritten: %q", got.Screenshots[0])
	}
}
