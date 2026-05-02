package datadir

import (
	"path/filepath"
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
