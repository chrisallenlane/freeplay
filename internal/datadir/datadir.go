// Package datadir owns the filesystem layout of the operator's data
// directory. Every package that reads or writes files under the data
// directory goes through these constructors so the layout has a single
// owner.
//
// Layout:
//
//	<dataDir>/
//	  covers/<console>/<name>.png
//	  manuals/<console>/<name>.pdf
//	  saves/<console>/<game>/<type>
//	  cache/igdb/<console>/<cleanName>/{details.json,.notfound,cover.jpg,screenshot_N.jpg,...}
//
// The package also hosts two path-safety primitives used at every
// trust boundary that constructs paths from user-influenced segments.
package datadir

import (
	"path/filepath"
	"strings"
)

// Covers returns <dataDir>/covers.
func Covers(dataDir string) string {
	return filepath.Join(dataDir, "covers")
}

// CoversConsole returns <dataDir>/covers/<console>.
func CoversConsole(dataDir, console string) string {
	return filepath.Join(Covers(dataDir), console)
}

// CoverFile returns <dataDir>/covers/<console>/<nameNoExt>.png — the
// canonical location of a game's grid-thumbnail cover.
func CoverFile(dataDir, console, nameNoExt string) string {
	return filepath.Join(CoversConsole(dataDir, console), nameNoExt+".png")
}

// Manuals returns <dataDir>/manuals.
func Manuals(dataDir string) string {
	return filepath.Join(dataDir, "manuals")
}

// ManualsConsole returns <dataDir>/manuals/<console>.
func ManualsConsole(dataDir, console string) string {
	return filepath.Join(Manuals(dataDir), console)
}

// SavePath returns the canonical on-disk location for a save-state or
// SRAM blob: <dataDir>/saves/<console>/<game>/<saveType>.
func SavePath(dataDir, console, game, saveType string) string {
	return filepath.Join(dataDir, "saves", console, game, saveType)
}

// IGDBCache returns <dataDir>/cache/igdb — the root of the IGDB
// metadata+image cache tree.
func IGDBCache(dataDir string) string {
	return filepath.Join(dataDir, "cache", "igdb")
}

// IGDBCacheGame returns the per-game subdirectory under the IGDB
// cache: <dataDir>/cache/igdb/<console>/<cleanName>.
func IGDBCacheGame(dataDir, console, cleanName string) string {
	return filepath.Join(IGDBCache(dataDir), console, cleanName)
}

// SafePathSegment reports whether s is safe to use as a single path
// segment inside a trusted directory. Rejects empty, ".", "..", any
// traversal substring, any path separator, and NUL bytes. Callers
// should skip (tombstone) offending inputs rather than trying to
// sanitize — a ROM whose filename produces an unsafe segment is an
// attacker signal, not an ergonomic concern.
func SafePathSegment(s string) bool {
	return s != "" && s != "." && s != ".." &&
		!strings.Contains(s, "..") &&
		!strings.ContainsAny(s, `/\`+"\x00")
}

// PathInside reports whether the cleaned child path is rooted under
// the cleaned parent directory. Defense-in-depth check at every site
// that resolves a user-influenced path against a trusted root.
func PathInside(child, parent string) bool {
	cleanChild := filepath.Clean(child)
	cleanParent := filepath.Clean(parent)
	return cleanChild == cleanParent ||
		strings.HasPrefix(cleanChild, cleanParent+string(filepath.Separator))
}
