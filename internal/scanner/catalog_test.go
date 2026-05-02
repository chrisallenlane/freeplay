package scanner

import "testing"

func TestStripExt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal extension", "Mega Man.nes", "Mega Man"},
		{"multiple dots", "Some.Game.zip", "Some.Game"},
		{"no extension", "RomNoExt", "RomNoExt"},
		{"dotfile leading dot only", ".bashrc", ".bashrc"},
		{"dot at index zero", ".hidden", ".hidden"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripExt(tt.input)
			if got != tt.want {
				t.Errorf("stripExt(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestHasGameSlug tests the O(1) slug index built by newCatalog.
// Cases cover the core contract: a slug matches its source ROM, the full
// filename does not (it's a slug lookup, not a filename lookup), unknown
// slugs and wrong consoles are rejected, and edge cases (no extension,
// leading-dot filenames) are handled by the > 0 rule in stripExt.
func TestHasGameSlug(t *testing.T) {
	games := []Game{
		{Console: "NES", Filename: "Mega Man.nes"},
		{Console: "NES", Filename: "RomNoExt"},
		{Console: "NES", Filename: ".bashrc"},
	}
	cat := newCatalog([]string{"NES"}, games)

	// Wrap in a minimal Scanner so we can call HasGameSlug.
	s := &Scanner{}
	s.catalog.Store(cat)

	tests := []struct {
		name    string
		console string
		slug    string
		want    bool
	}{
		{"slug matches NES rom", "NES", "Mega Man", true},
		{"full filename is not a slug", "NES", "Mega Man.nes", false},
		{"unknown slug", "NES", "Unknown", false},
		{"wrong console", "SNES", "Mega Man", false},
		{"no-extension rom slug equals filename", "NES", "RomNoExt", true},
		{"dotfile slug equals filename (leading dot, > 0 rule)", "NES", ".bashrc", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.HasGameSlug(tt.console, tt.slug)
			if got != tt.want {
				t.Errorf(
					"HasGameSlug(%q, %q) = %v, want %v",
					tt.console, tt.slug, got, tt.want,
				)
			}
		})
	}
}
