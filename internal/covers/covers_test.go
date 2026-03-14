package covers

import (
	"path/filepath"
	"testing"
)

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
	}

	for _, tt := range tests {
		got := CleanName(tt.input)
		if got != tt.want {
			t.Errorf("CleanName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPath(t *testing.T) {
	got := Path("/data", "NES", "Mega Man")
	want := filepath.Join("/data", "covers", "NES", "Mega Man.png")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestFetchMissingNilFetcher(t *testing.T) {
	m := New(t.TempDir(), nil)
	// Should return immediately without panic
	m.FetchMissing([]GameEntry{{Console: "NES", Filename: "game.nes"}})
}
