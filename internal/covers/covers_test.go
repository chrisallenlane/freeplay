package covers

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestIntsToStrings(t *testing.T) {
	tests := []struct {
		input []int
		want  []string
	}{
		{[]int{18}, []string{"18"}},
		{[]int{18, 99}, []string{"18", "99"}},
		{[]int{}, []string{}},
	}
	for _, tt := range tests {
		got := intsToStrings(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("intsToStrings(%v) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("intsToStrings(%v)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestCleanName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Super Mario Bros (USA)", "Super Mario Bros"},
		{"Zelda [!]", "Zelda"},
		{"Sonic (USA) (Rev 1)", "Sonic"},
		{"Game", "Game"},
		{"(tag only)", ""},
		{"3-in-1 Super Mario Bros", "3-in-1 Super Mario Bros"},
		{"Mega Man 2 (U) [!]", "Mega Man 2"},
		{"Front Mission - Gun Hazard (ENG) # SNES", "Front Mission - Gun Hazard"},
		{"Ultraman - Towards the Future (U) [!]", "Ultraman - Towards the Future"},
		{"Nobunaga's Ambition - Lord of Darkness (U)", "Nobunaga's Ambition - Lord of Darkness"},
		{"Double Dragon V - The Shadow Falls (U)", "Double Dragon V - The Shadow Falls"},
		{"Final Fantasy 6 (ENG) # SNES", "Final Fantasy 6"},
	}

	for _, tt := range tests {
		got := CleanName(tt.input)
		if got != tt.want {
			t.Errorf("CleanName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNameVariants(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		// No transformations produce new variants
		{"Metroid", []string{"Metroid"}},
		// Dash to colon
		{"Game - Subtitle", []string{
			"Game - Subtitle",
			"Game: Subtitle",
			"Game-Subtitle",
			"Game",
		}},
		// Compound word (spaces removed)
		{"Sim City", []string{"Sim City", "SimCity"}},
		// All heuristics apply
		{"Nobunaga's Ambition - Lord of Darkness", []string{
			"Nobunaga's Ambition - Lord of Darkness",
			"Nobunaga's Ambition: Lord of Darkness",
			"Nobunaga'sAmbition-LordofDarkness",
			"Nobunaga's Ambition",
		}},
		// Subtitle with colon (no dash)
		{"SimCity: BuildIt", []string{
			"SimCity: BuildIt",
			"SimCity:BuildIt",
			"SimCity",
		}},
	}

	for _, tt := range tests {
		got := nameVariants(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("nameVariants(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("nameVariants(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestCoverPath(t *testing.T) {
	got := coverPath("/data", "NES", "Mega Man")
	want := filepath.Join("/data", "covers", "NES", "Mega Man.png")
	if got != want {
		t.Errorf("coverPath() = %q, want %q", got, want)
	}
}

func TestFetchMissingNilFetcher(t *testing.T) {
	m := New(t.TempDir(), nil)
	got := m.FetchMissing([]GameEntry{{Console: "NES", Filename: "game.nes"}})
	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0 (nil fetcher)", got)
	}
}

func FuzzCleanName(f *testing.F) {
	f.Add("Super Mario Bros (USA)")
	f.Add("Zelda [!]")
	f.Add("(tag only)")
	f.Add("")
	f.Add("Game (Rev 1) [!] (USA)")

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic
		got := CleanName(input)
		// Output should never be longer than input
		if len(got) > len(input) {
			t.Errorf("CleanName(%q) produced longer output %q", input, got)
		}
	})
}

// mockFetcher is a test double for the Fetcher interface.
type mockFetcher struct {
	img   image.Image
	err   error
	calls int
}

func (f *mockFetcher) Fetch(
	_ string,
	_ string,
	_ []int,
) (image.Image, error) {
	f.calls++
	return f.img, f.err
}

func TestFetchMissingSavesCovers(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fetcher := &mockFetcher{img: img}
	m := New(dir, fetcher)

	game := GameEntry{Console: "NES", Filename: "Super Mario Bros (USA).nes"}
	got := m.FetchMissing([]GameEntry{game})

	if got != 1 {
		t.Errorf("FetchMissing() = %d, want 1", got)
	}

	coverPath := coverPath(dir, "NES", "Super Mario Bros (USA)")
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("expected cover file at %q, got stat error: %v", coverPath, err)
	}
}

func TestFetchMissingSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	fetcher := &mockFetcher{img: image.NewRGBA(image.Rect(0, 0, 1, 1))}
	m := New(dir, fetcher)

	game := GameEntry{Console: "NES", Filename: "Super Mario Bros (USA).nes"}
	coverPath := coverPath(dir, "NES", "Super Mario Bros (USA)")

	// Pre-create the cover directory and file.
	if err := os.MkdirAll(filepath.Dir(coverPath), 0o755); err != nil {
		t.Fatalf("failed to create cover dir: %v", err)
	}
	if err := os.WriteFile(coverPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("failed to pre-create cover file: %v", err)
	}

	got := m.FetchMissing([]GameEntry{game})

	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0 (cover already exists)", got)
	}
	if fetcher.calls != 0 {
		t.Errorf("Fetch() called %d times, want 0", fetcher.calls)
	}
}

func TestFetchMissingFetchError(t *testing.T) {
	dir := t.TempDir()
	fetcher := &mockFetcher{err: errors.New("network error")}
	m := New(dir, fetcher)

	game := GameEntry{Console: "NES", Filename: "Super Mario Bros (USA).nes"}
	got := m.FetchMissing([]GameEntry{game})

	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0 (fetch failed)", got)
	}
}

func TestFetchMissingNilImage(t *testing.T) {
	dir := t.TempDir()
	fetcher := &mockFetcher{img: nil, err: nil}
	m := New(dir, fetcher)

	game := GameEntry{Console: "NES", Filename: "Super Mario Bros (USA).nes"}
	got := m.FetchMissing([]GameEntry{game})

	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0 (nil image)", got)
	}
}

// platformFallbackFetcher returns nil when platform IDs are provided,
// and an image when they are not, simulating an IGDB miss on a specific
// platform with a hit on the unconstrained search.
type platformFallbackFetcher struct {
	calls int
}

func (f *platformFallbackFetcher) Fetch(
	_ string,
	_ string,
	platformIDs []int,
) (image.Image, error) {
	f.calls++
	if len(platformIDs) > 0 {
		return nil, nil
	}
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}

func TestFetchMissingPlatformFallback(t *testing.T) {
	dir := t.TempDir()
	fetcher := &platformFallbackFetcher{}
	m := New(dir, fetcher)

	game := GameEntry{
		Console:         "NES",
		Filename:        "Q-Bert.nes",
		IGDBPlatformIDs: []int{18, 99},
	}
	got := m.FetchMissing([]GameEntry{game})

	if got != 1 {
		t.Errorf("FetchMissing() = %d, want 1", got)
	}
	if fetcher.calls != 2 {
		t.Errorf("Fetch() called %d times, want 2 (platform + fallback)", fetcher.calls)
	}

	coverPath := coverPath(dir, "NES", "Q-Bert")
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("expected cover file at %q, got stat error: %v", coverPath, err)
	}
}

func TestFetchMissingNoFallbackWithoutPlatformIDs(t *testing.T) {
	dir := t.TempDir()
	fetcher := &mockFetcher{img: nil, err: nil}
	m := New(dir, fetcher)

	game := GameEntry{Console: "NES", Filename: "Q-Bert.nes"}
	got := m.FetchMissing([]GameEntry{game})

	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0", got)
	}
	if fetcher.calls != 1 {
		t.Errorf("Fetch() called %d times, want 1 (no fallback without platform IDs)", fetcher.calls)
	}
}

func TestFetchMissingSkipsEmptyCleanName(t *testing.T) {
	dir := t.TempDir()
	fetcher := &mockFetcher{img: image.NewRGBA(image.Rect(0, 0, 1, 1))}
	m := New(dir, fetcher)

	// CleanName strips all tags, leaving an empty string.
	game := GameEntry{Console: "NES", Filename: "(tag only).nes"}
	got := m.FetchMissing([]GameEntry{game})

	if got != 0 {
		t.Errorf("FetchMissing() = %d, want 0 (empty clean name)", got)
	}
	if fetcher.calls != 0 {
		t.Errorf("Fetch() called %d times, want 0", fetcher.calls)
	}
}
