package details

import (
	"path/filepath"
	"strings"
)

// memKey builds the in-memory cache key for (console, cleanName).
func memKey(console, cleanName string) string {
	return console + "/" + cleanName
}

// pathInside reports whether the cleaned child path is rooted under
// the cleaned parent directory. The same check used by server.serveSecureFile.
func pathInside(child, parent string) bool {
	cleanChild := filepath.Clean(child)
	cleanParent := filepath.Clean(parent)
	return cleanChild == cleanParent ||
		strings.HasPrefix(cleanChild, cleanParent+string(filepath.Separator))
}

// safePathSegment reports whether s is safe to use as a single path
// segment inside a trusted directory. Rejects empty, ".", "..",
// anything containing a path separator, and NUL bytes. Callers should
// skip (tombstone) offending inputs rather than trying to sanitize —
// a ROM whose filename produces an unsafe segment is an attacker
// signal, not an ergonomic concern.
func safePathSegment(s string) bool {
	return s != "" && s != "." && s != ".." &&
		!strings.ContainsAny(s, `/\`+"\x00")
}
