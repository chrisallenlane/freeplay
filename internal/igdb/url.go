package igdb

import "strings"

// IGDBImageHost is the only host we accept for image URLs returned by
// the IGDB API. Exported so other packages (e.g. details.Cache) can
// enforce cross-host redirect blocking.
const IGDBImageHost = "images.igdb.com"

// igdbInfoURLPrefix is the canonical IGDB info-page URL prefix.
// IGDB game.url fields point here.
const igdbInfoURLPrefix = "https://www.igdb.com/"

// transformImageURL normalizes an IGDB image URL and rewrites its size
// slug. Returns "" if the input is not a valid images.igdb.com URL —
// callers MUST skip images for which this returns "". This is the
// server-side trust-boundary check for IGDB SSRF (see SEC-2).
func transformImageURL(u, size string) string {
	var canonical string
	switch {
	case strings.HasPrefix(u, "//"+IGDBImageHost+"/"):
		canonical = "https:" + u
	case strings.HasPrefix(u, "https://"+IGDBImageHost+"/"):
		canonical = u
	default:
		return ""
	}
	return strings.Replace(canonical, "t_thumb", size, 1)
}

// safeIGDBInfoURL returns u if it is a valid IGDB info-page URL, or ""
// otherwise. Prevents javascript:/data:/http: URLs from reaching the
// frontend via the "View on IGDB" link (see SEC-2 / H-2).
func safeIGDBInfoURL(u string) string {
	if strings.HasPrefix(u, igdbInfoURLPrefix) {
		return u
	}
	return ""
}
