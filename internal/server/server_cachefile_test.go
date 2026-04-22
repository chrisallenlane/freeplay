package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestCacheFileHTMLContent_ExtensionMitigatesXSS verifies that even when a
// cached "image" file actually contains HTML content (because IGDB returned
// an HTML error page at an image URL), http.ServeFile uses the .jpg file
// extension to set Content-Type: image/jpeg, NOT text/html.
//
// This test validates the defense-in-depth: although downloadImage does not
// validate content type (saving arbitrary content as .jpg), the .jpg
// extension causes http.ServeFile to use extension-based MIME lookup rather
// than content sniffing. Combined with X-Content-Type-Options: nosniff, the
// browser will not interpret the content as HTML.
func TestCacheFileHTMLContent_ExtensionMitigatesXSS(t *testing.T) {
	srv, dir := testServer(t)

	// Simulate a cached "image" file that actually contains HTML.
	htmlPayload := `<html><body><script>alert('XSS')</script></body></html>`
	writeCacheFile(t, dir, "NES", "EvilGame", "cover.jpg", []byte(htmlPayload))

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/EvilGame/cover.jpg", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")

	// http.ServeFile uses extension-based MIME lookup for known extensions.
	// .jpg maps to image/jpeg, preventing content sniffing from detecting HTML.
	if strings.HasPrefix(ct, "text/html") {
		t.Errorf(
			"cached .jpg file with HTML content should NOT be served as text/html, "+
				"got Content-Type: %q. Extension-based MIME should override sniffing.",
			ct,
		)
	}

	// Verify it's served as image/jpeg
	if !strings.HasPrefix(ct, "image/jpeg") {
		t.Errorf(
			"cached .jpg file should be served as image/jpeg regardless of content, "+
				"got Content-Type: %q",
			ct,
		)
	}
}

// TestCacheFileSVGSniffing verifies that if a cached "image" file contains
// SVG content (which can include embedded JavaScript), it is not served as
// image/svg+xml. SVG files can execute JavaScript in the browser.
func TestCacheFileSVGSniffing(t *testing.T) {
	srv, dir := testServer(t)

	svgPayload := `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg">
  <script>alert('XSS via SVG')</script>
</svg>`
	writeCacheFile(t, dir, "NES", "SvgGame", "cover.jpg", []byte(svgPayload))

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/SvgGame/cover.jpg", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")

	// SVG can contain JavaScript. If the content is sniffed as XML or SVG,
	// it should not be served as such for a .jpg file.
	if strings.Contains(ct, "svg") || strings.Contains(ct, "xml") {
		t.Errorf(
			"SECURITY: cached .jpg file with SVG content is served as %q. "+
				"SVG can contain JavaScript. The server should force an "+
				"image content type for cached image files.",
			ct,
		)
	}
}

// TestCacheFileJPEGContent verifies that legitimate JPEG content is served
// with the correct content-type.
func TestCacheFileJPEGContent(t *testing.T) {
	srv, dir := testServer(t)

	// JPEG magic bytes (SOI marker)
	jpegData := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	writeCacheFile(t, dir, "NES", "GoodGame", "cover.jpg", jpegData)

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/GoodGame/cover.jpg", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "image/jpeg") {
		t.Errorf(
			"legitimate JPEG content should be served as image/jpeg, got %q",
			ct,
		)
	}
}

// TestCacheFileNosniffHeaderPresent verifies that X-Content-Type-Options:
// nosniff is present on cache file responses.
func TestCacheFileNosniffHeaderPresent(t *testing.T) {
	srv, dir := testServer(t)

	// JPEG magic bytes
	writeCacheFile(t, dir, "NES", "TestGame", "cover.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0})

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/TestGame/cover.jpg", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	nosniff := w.Header().Get("X-Content-Type-Options")
	if nosniff != "nosniff" {
		t.Errorf(
			"X-Content-Type-Options = %q, want %q",
			nosniff, "nosniff",
		)
	}
}

// TestCacheFileLongCacheHeader verifies that cached IGDB files are served
// with the immutable long-cache header.
func TestCacheFileLongCacheHeader(t *testing.T) {
	srv, dir := testServer(t)

	writeCacheFile(
		t, dir, "NES", "CacheTest", "screenshot_0.jpg",
		[]byte{0xFF, 0xD8, 0xFF, 0xE0},
	)

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/CacheTest/screenshot_0.jpg", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}

	cc := w.Header().Get("Cache-Control")
	if cc != longCacheValue {
		t.Errorf("Cache-Control = %q, want %q", cc, longCacheValue)
	}
}
