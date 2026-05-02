package datadir

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSafePathSegment(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"Mega Man", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../evil", false},
		{"a/b", false},
		{"a\\b", false},
		{"a\x00b", false},
		{"normal.nes", true},
		{"foo..bar", false}, // substring traversal
	}
	for _, tt := range tests {
		if got := SafePathSegment(tt.in); got != tt.want {
			t.Errorf("SafePathSegment(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPathInside(t *testing.T) {
	parent := "/data/covers"
	tests := []struct {
		child string
		want  bool
	}{
		{"/data/covers", true},
		{"/data/covers/NES", true},
		{"/data/covers/NES/Mega Man.png", true},
		{"/data/covers/../etc/passwd", false},
		{"/data/cover", false},
		{"/data/coversNES", false},
	}
	for _, tt := range tests {
		if got := PathInside(tt.child, parent); got != tt.want {
			t.Errorf("PathInside(%q, %q) = %v, want %v", tt.child, parent, got, tt.want)
		}
	}
}

// TestPathInsideNormalizesParent verifies that PathInside filepath.Cleans
// the parent argument, not just the child. An unclean parent like
// "/data/covers/." must still correctly recognize "/data/covers/NES"
// as inside it.
func TestPathInsideNormalizesParent(t *testing.T) {
	tests := []struct {
		child  string
		parent string
		want   bool
	}{
		{"/data/covers/NES", "/data/covers/.", true},
		{"/data/covers/NES", "/data//covers", true},
		{"/data/covers", "/data/covers/.", true},
		{"/data/other", "/data/covers/.", false},
	}
	for _, tt := range tests {
		if got := PathInside(tt.child, tt.parent); got != tt.want {
			t.Errorf("PathInside(%q, %q) = %v, want %v", tt.child, tt.parent, got, tt.want)
		}
	}
}

// FuzzPathInside probes the trust-boundary path-traversal guard.
// SafePathSegment is fuzzed in internal/server/server_test.go;
// PathInside is the second half of the same defense, and a regression
// in either invariant (cleaned-prefix relationship; reflexivity)
// silently widens the file-serving attack surface.
func FuzzPathInside(f *testing.F) {
	f.Add("/data/covers", "/data/covers")
	f.Add("/data/covers/NES", "/data/covers")
	f.Add("/data/covers/../etc/passwd", "/data/covers")
	f.Add("/data/cover", "/data/covers")
	f.Add("/data/coversNES", "/data/covers")
	f.Add("/data/covers/.", "/data/covers")
	f.Add("/data//covers", "/data/covers")
	f.Add("", "")
	f.Add("a/b/c", "a")
	f.Add("a", "a/b/c")

	f.Fuzz(func(t *testing.T, child, parent string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf(
					"PathInside panicked on (%q, %q): %v",
					child, parent, r,
				)
			}
		}()

		got := PathInside(child, parent)

		// Reflexivity: any path is inside itself, even after Clean
		// rewrites it. Skip for the tautological self-call we just
		// made; do a fresh check with parent/parent.
		if !PathInside(parent, parent) {
			t.Errorf("PathInside(%q, %q) reflexivity broken", parent, parent)
		}

		// If PathInside reports true, the cleaned child must equal
		// the cleaned parent or have it as a directory prefix. If
		// this invariant breaks, callers like serveSecureFile would
		// happily serve files outside their trusted root.
		if got {
			cleanChild := filepath.Clean(child)
			cleanParent := filepath.Clean(parent)
			if cleanChild != cleanParent &&
				!strings.HasPrefix(cleanChild, cleanParent+string(filepath.Separator)) {
				t.Errorf(
					"PathInside(%q, %q) = true but cleaned forms (%q, %q) lack prefix relationship",
					child, parent, cleanChild, cleanParent,
				)
			}
		}
	})
}

func TestLayoutConstructors(t *testing.T) {
	const dataDir = "/data"
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Covers", Covers(dataDir), filepath.FromSlash("/data/covers")},
		{"CoversConsole", CoversConsole(dataDir, "NES"), filepath.FromSlash("/data/covers/NES")},
		{"CoverFile", CoverFile(dataDir, "NES", "Mega Man"), filepath.FromSlash("/data/covers/NES/Mega Man.png")},
		{"Manuals", Manuals(dataDir), filepath.FromSlash("/data/manuals")},
		{"ManualsConsole", ManualsConsole(dataDir, "NES"), filepath.FromSlash("/data/manuals/NES")},
		{"SavePath", SavePath(dataDir, "NES", "Mega Man", "sram"), filepath.FromSlash("/data/saves/NES/Mega Man/sram")},
		{"IGDBCache", IGDBCache(dataDir), filepath.FromSlash("/data/cache/igdb")},
		{"IGDBCacheGame", IGDBCacheGame(dataDir, "NES", "Mega Man"), filepath.FromSlash("/data/cache/igdb/NES/Mega Man")},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}
