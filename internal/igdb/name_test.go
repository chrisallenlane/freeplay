package igdb

import (
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
		{"Front Mission - Gun Hazard (ENG) # SNES", "Front Mission - Gun Hazard"},
		{"Ultraman - Towards the Future (U) [!]", "Ultraman - Towards the Future"},
		{
			"Nobunaga's Ambition - Lord of Darkness (U)",
			"Nobunaga's Ambition - Lord of Darkness",
		},
		{
			"Double Dragon V - The Shadow Falls (U)",
			"Double Dragon V - The Shadow Falls",
		},
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
		got := NameVariants(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf(
				"NameVariants(%q) = %v, want %v",
				tt.input, got, tt.want,
			)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf(
					"NameVariants(%q)[%d] = %q, want %q",
					tt.input, i, got[i], tt.want[i],
				)
			}
		}
	}
}

func TestCleanFilename(t *testing.T) {
	tests := []struct {
		input     string
		wantNoExt string
		wantClean string
	}{
		{
			input:     "Super Mario Bros (USA).zip",
			wantNoExt: "Super Mario Bros (USA)",
			wantClean: "Super Mario Bros",
		},
		{
			// Extensionless filename: nothing to strip.
			input:     "Zelda",
			wantNoExt: "Zelda",
			wantClean: "Zelda",
		},
		{
			// Multiple dots: only the last extension is removed.
			input:     "Mega Man 2.rev1.nes",
			wantNoExt: "Mega Man 2.rev1",
			wantClean: "Mega Man 2.rev1",
		},
		{
			// Empty string: both outputs are empty.
			input:     "",
			wantNoExt: "",
			wantClean: "",
		},
	}

	for _, tt := range tests {
		gotNoExt, gotClean := CleanFilename(tt.input)
		if gotNoExt != tt.wantNoExt {
			t.Errorf(
				"CleanFilename(%q) nameNoExt = %q, want %q",
				tt.input, gotNoExt, tt.wantNoExt,
			)
		}
		if gotClean != tt.wantClean {
			t.Errorf(
				"CleanFilename(%q) cleanName = %q, want %q",
				tt.input, gotClean, tt.wantClean,
			)
		}
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
			t.Errorf(
				"CleanName(%q) produced longer output %q",
				input, got,
			)
		}
	})
}

func FuzzNameVariants(f *testing.F) {
	f.Add("Metroid")
	f.Add("Game - Subtitle")
	f.Add("Sim City")
	f.Add("SimCity: BuildIt")
	f.Add("")
	f.Add(" - ")
	f.Add(": ")
	f.Add("   ")

	f.Fuzz(func(t *testing.T, input string) {
		result := NameVariants(input)
		if input == "" {
			if len(result) != 0 {
				t.Error("NameVariants(\"\") should return nil")
			}
			return
		}
		if len(result) == 0 {
			t.Error(
				"NameVariants must return at least one element for non-empty input",
			)
		}
		if result[0] != input {
			t.Errorf(
				"first element must be the input itself, got %q",
				result[0],
			)
		}
		for i, v := range result {
			if v == "" {
				t.Errorf("NameVariants[%d] is empty string", i)
			}
		}
	})
}
