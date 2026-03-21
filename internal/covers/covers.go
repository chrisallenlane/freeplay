// Package covers manages cover art fetching and caching.
package covers

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	tagPattern = regexp.MustCompile(`\s*[\(\[].*?[\)\]]`)
	hashSuffix = regexp.MustCompile(`\s+#\s+\S+$`)
)

// CleanName strips No-Intro tags and hash suffixes from a ROM filename
// for API search. Tags in parentheses/brackets (e.g. "(USA)", "[!]") and
// translation-patch suffixes (e.g. "# SNES") are removed.
func CleanName(nameWithoutExt string) string {
	name := tagPattern.ReplaceAllString(nameWithoutExt, "")
	name = hashSuffix.ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

// NameVariants returns search name variants ordered from highest to lowest
// confidence. Each variant represents a different heuristic for matching
// ROM filenames to IGDB game titles.
func NameVariants(cleanName string) []string {
	if cleanName == "" {
		return nil
	}

	seen := map[string]bool{cleanName: true}
	variants := []string{cleanName}

	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			variants = append(variants, s)
		}
	}

	// Dashes to colons: No-Intro uses " - " for subtitles, IGDB uses ": "
	add(strings.ReplaceAll(cleanName, " - ", ": "))

	// Spaces removed: catches compound-word titles (SimCity, SimAnt)
	add(strings.ReplaceAll(cleanName, " ", ""))

	// Subtitle dropped: catches regional subtitle mismatches
	if idx := strings.Index(cleanName, " - "); idx > 0 {
		add(strings.TrimSpace(cleanName[:idx]))
	} else if idx := strings.Index(cleanName, ": "); idx > 0 {
		add(strings.TrimSpace(cleanName[:idx]))
	}

	return variants
}

// CleanFilename splits a ROM filename into its base name (without extension)
// and the cleaned name suitable for IGDB searches.
func CleanFilename(filename string) (nameNoExt, cleanName string) {
	ext := filepath.Ext(filename)
	nameNoExt = strings.TrimSuffix(filename, ext)
	cleanName = CleanName(nameNoExt)
	return
}

// CoverPath returns the expected filesystem path for a game's cover art.
func CoverPath(dataDir, console, filenameWithoutExt string) string {
	return filepath.Join(dataDir, "covers", console, filenameWithoutExt+".png")
}

// GameEntry describes a game for cache population.
type GameEntry struct {
	Console         string
	Filename        string
	IGDBPlatformIDs []int
}
