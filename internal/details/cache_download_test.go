package details

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// TestDownloadImage_HTMLContentRejected verifies that downloadImage rejects
// responses with non-image Content-Type headers. If the remote server returns
// HTML content at an image URL (e.g., a CDN error page), downloadImage should
// return an error rather than saving the content as a .jpg file.
func TestDownloadImage_HTMLContentRejected(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body>error</body></html>`))
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	cacheDir := filepath.Join(dir, "test-cache")
	urlBase := "/cache/igdb/NES/EvilGame"

	_, _, err := c.downloadImage(
		imgServer.URL+"/cover.jpg",
		cacheDir, urlBase, "cover.jpg",
	)
	if err == nil {
		t.Fatal("downloadImage should reject non-image Content-Type")
	}
	if !strings.Contains(err.Error(), "unexpected content-type") {
		t.Errorf("error should mention content-type, got: %v", err)
	}

	// Verify no file was saved
	coverPath := filepath.Join(cacheDir, "cover.jpg")
	if _, statErr := os.Stat(coverPath); statErr == nil {
		t.Errorf("non-image content should not be saved to %q", coverPath)
	}
}

// TestDownloadImage_NonOKStatusCode verifies that downloadImage returns an
// error when the server returns a non-200 status code.
func TestDownloadImage_NonOKStatusCode(t *testing.T) {
	codes := []int{
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusForbidden,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	}

	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			imgServer := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "error", code)
				}),
			)
			defer imgServer.Close()

			dir := t.TempDir()
			c := New(dir, nil)

			_, _, err := c.downloadImage(
				imgServer.URL+"/cover.jpg",
				filepath.Join(dir, "cache"),
				"/cache/igdb/NES/Game",
				"cover.jpg",
			)
			if err == nil {
				t.Errorf("downloadImage should return error for status %d", code)
			}
			if err != nil && !strings.Contains(err.Error(), fmt.Sprintf("status %d", code)) {
				t.Errorf("error %q should mention status code %d", err, code)
			}
		})
	}
}

// TestDownloadImage_ConnectionError verifies that downloadImage returns an
// error when the connection fails.
func TestDownloadImage_ConnectionError(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)
	c.client = &http.Client{Timeout: 200 * time.Millisecond}

	_, _, err := c.downloadImage(
		"http://192.0.2.1:1/cover.jpg", // RFC 5737 TEST-NET, unreachable
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	if err == nil {
		t.Error("downloadImage should return error for unreachable host")
	}
}

// TestDownloadImage_EmptyBody verifies behavior when the server returns
// 200 OK with an empty body and no Content-Type header.
func TestDownloadImage_EmptyBody(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			// No body written, no Content-Type set
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	_, _, err := c.downloadImage(
		imgServer.URL+"/cover.jpg",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	// Empty body with no Content-Type should be rejected
	if err == nil {
		t.Fatal("downloadImage should reject response without image/* Content-Type")
	}
}

// TestDownloadImage_LimitReader verifies that downloadImage limits the
// downloaded content to 20 MiB. Content beyond 20 MiB should be silently
// truncated (io.LimitReader stops at the limit).
func TestDownloadImage_LimitReader(t *testing.T) {
	// Serve slightly more than 20 MiB
	const limitBytes = 20 << 20 // 20 MiB
	const overSize = limitBytes + 1024

	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			data := make([]byte, overSize)
			for i := range data {
				data[i] = 0xFF
			}
			_, _ = w.Write(data)
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	localPath, _, err := c.downloadImage(
		imgServer.URL+"/cover.jpg",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	if err != nil {
		t.Fatalf("downloadImage returned error: %v", err)
	}

	info, err := os.Stat(localPath)
	if err != nil {
		t.Fatalf("stat saved file: %v", err)
	}

	if info.Size() > limitBytes {
		t.Errorf(
			"file size %d exceeds LimitReader cap of %d bytes",
			info.Size(), limitBytes,
		)
	}

	// The file should be exactly 20 MiB (the limit)
	if info.Size() != limitBytes {
		t.Errorf(
			"file size = %d, want exactly %d (LimitReader cap)",
			info.Size(), limitBytes,
		)
	}
}

// TestDownloadImage_RedirectFollowed verifies that downloadImage follows
// HTTP redirects (since it uses http.Client.Get which follows redirects
// by default).
func TestDownloadImage_RedirectFollowed(t *testing.T) {
	// Final destination server
	finalServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG header
		}),
	)
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, finalServer.URL+"/final.jpg", http.StatusMovedPermanently)
		}),
	)
	defer redirectServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	localPath, _, err := c.downloadImage(
		redirectServer.URL+"/cover.jpg",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	if err != nil {
		t.Fatalf("downloadImage should follow redirects, got error: %v", err)
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}

	// Should have the JPEG header from the final destination
	if len(data) != 4 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Errorf("expected JPEG header from redirect target, got %v", data)
	}
}

// TestDownloadImage_URLPath verifies that the returned local URL path
// is properly constructed with URL-escaped components.
func TestDownloadImage_URLPath(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("data"))
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	_, localURL, err := c.downloadImage(
		imgServer.URL+"/img.jpg",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game Name",
		"screenshot_0.jpg",
	)
	if err != nil {
		t.Fatalf("downloadImage returned error: %v", err)
	}

	// The filename should be URL-escaped in the returned URL
	want := "/cache/igdb/NES/Game Name/screenshot_0.jpg"
	if localURL != want {
		t.Errorf("localURL = %q, want %q", localURL, want)
	}
}

// TestSaveDetails_HTMLCoverRejected verifies that when IGDB returns HTML
// content at a cover image URL (e.g., a CDN error page), saveDetails does
// not save the HTML as cover.jpg. The cover download should fail due to
// content-type validation, and CoverURL should be cleared.
func TestSaveDetails_HTMLCoverRejected(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!DOCTYPE html><html><body>error</body></html>`))
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name:     "Evil Game",
		CoverURL: imgServer.URL + "/t_original/cover.jpg",
	}

	err := c.saveDetails("NES", "Evil Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	// CoverURL should be cleared (download failed)
	if details.CoverURL != "" {
		t.Errorf("CoverURL should be empty after failed download, got %q", details.CoverURL)
	}

	// cover.jpg should not exist
	coverPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Evil Game", "cover.jpg",
	)
	if _, err := os.Stat(coverPath); err == nil {
		t.Errorf("cover.jpg should not exist at %q — HTML content should be rejected", coverPath)
	}
}

// TestDownloadImage_SlowServer verifies that downloadImage respects the
// client timeout. The default client has a 30-second timeout; we create
// a cache with a shorter timeout for testing.
func TestDownloadImage_SlowServer(t *testing.T) {
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(2 * time.Second)
			_, _ = w.Write([]byte("delayed data"))
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)
	// Override with a very short timeout
	c.client = &http.Client{Timeout: 100 * time.Millisecond}

	_, _, err := c.downloadImage(
		imgServer.URL+"/cover.jpg",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	// Should fail due to timeout during body read
	if err == nil {
		t.Error("downloadImage should return error for slow server with short timeout")
	}
}

// TestSaveDetails_ScreenshotAndArtworkImageErrors verifies that when
// screenshot and artwork image downloads fail, they are silently dropped
// from the details (not left as remote URLs).
func TestSaveDetails_ScreenshotAndArtworkImageErrors(t *testing.T) {
	// Server that returns 500 for all requests
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name: "Test Game",
		Screenshots: []string{
			imgServer.URL + "/ss0.jpg",
			imgServer.URL + "/ss1.jpg",
		},
		Artworks: []string{
			imgServer.URL + "/art0.jpg",
		},
	}

	err := c.saveDetails("NES", "Test Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	// Screenshots and artworks should be empty (failed downloads dropped)
	if len(details.Screenshots) != 0 {
		t.Errorf(
			"Screenshots should be empty after download failures, got %v",
			details.Screenshots,
		)
	}
	if len(details.Artworks) != 0 {
		t.Errorf(
			"Artworks should be empty after download failures, got %v",
			details.Artworks,
		)
	}
}

// TestSaveDetails_PartialScreenshotDownloads verifies that when some
// screenshot downloads succeed and others fail, only successful ones
// are kept in the details.
func TestSaveDetails_PartialScreenshotDownloads(t *testing.T) {
	requestCount := 0
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requestCount++
			if requestCount%2 == 0 {
				http.Error(w, "error", http.StatusInternalServerError)
			} else {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write([]byte("imagedata"))
			}
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name: "Test Game",
		Screenshots: []string{
			imgServer.URL + "/ss0.jpg", // request 1: succeeds
			imgServer.URL + "/ss1.jpg", // request 2: fails (500)
			imgServer.URL + "/ss2.jpg", // request 3: succeeds
		},
	}

	err := c.saveDetails("NES", "Test Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	// Only the successful downloads should remain
	if len(details.Screenshots) != 2 {
		t.Errorf(
			"Screenshots len = %d, want 2 (2 succeeded, 1 failed)",
			len(details.Screenshots),
		)
	}

	// All remaining URLs should be local paths
	for i, url := range details.Screenshots {
		if !strings.HasPrefix(url, "/cache/igdb/") {
			t.Errorf(
				"Screenshots[%d] = %q, want local /cache/igdb/ path",
				i, url,
			)
		}
	}
}

// TestSaveDetails_CoverThumbDownloadIgnored verifies that a failed
// cover_thumb.jpg download is silently ignored (the main cover still
// gets its local URL assigned).
func TestSaveDetails_CoverThumbDownloadIgnored(t *testing.T) {
	requestCount := 0
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			if strings.Contains(r.URL.Path, "t_cover_big") {
				// Thumbnail request fails
				http.Error(w, "not found", http.StatusNotFound)
			} else {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write([]byte("coverdata"))
			}
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name:     "Test Game",
		CoverURL: imgServer.URL + "/t_original/cover.jpg",
	}

	err := c.saveDetails("NES", "Test Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	// The main cover URL should still be rewritten to local path
	if !strings.HasPrefix(details.CoverURL, "/cache/igdb/") {
		t.Errorf(
			"CoverURL should be rewritten to local path even when thumb fails, got %q",
			details.CoverURL,
		)
	}

	// cover.jpg should exist
	coverPath := filepath.Join(
		dir, "cache", "igdb", "NES", "Test Game", "cover.jpg",
	)
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("expected cover.jpg at %q, got: %v", coverPath, err)
	}
}

// TestDownloadImage_AtomicWriteConsistency verifies that if the write fails
// partway through, no partial file is left behind (atomicfile.Write uses
// a temp file + rename pattern).
func TestDownloadImage_AtomicWriteConsistency(t *testing.T) {
	// Serve a body that errors partway through
	imgServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(http.StatusOK)
			// Write partial data then close connection
			_, _ = w.Write([]byte("partial"))
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				if conn != nil {
					_ = conn.Close()
				}
			}
		}),
	)
	defer imgServer.Close()

	dir := t.TempDir()
	c := New(dir, nil)

	cacheDir := filepath.Join(dir, "cache")
	localPath := filepath.Join(cacheDir, "cover.jpg")

	_, _, err := c.downloadImage(
		imgServer.URL+"/cover.jpg",
		cacheDir,
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	// Whether this errors depends on how io.Copy interacts with the
	// truncated response body. Either way, verify no file is left
	// at the destination path if there was an error.
	if err != nil {
		if _, statErr := os.Stat(localPath); statErr == nil {
			t.Errorf(
				"downloadImage returned error %v but left partial file at %q",
				err, localPath,
			)
		}
	}
}

// TestSaveDetails_WritesDetailsJSON verifies that saveDetails writes a
// valid details.json file that can be round-tripped back.
func TestSaveDetails_WritesDetailsJSON(t *testing.T) {
	imgServer := startFakeImageServer(t)

	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name:        "Test Game",
		Summary:     "A test game.",
		CoverURL:    imgServer.URL + "/t_original/cover.jpg",
		Screenshots: []string{imgServer.URL + "/ss0.jpg"},
		Artworks:    []string{imgServer.URL + "/art0.jpg"},
	}

	err := c.saveDetails("NES", "Test Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	// Read back via Get
	got := c.Get("NES", "Test Game.nes")
	if got == nil {
		t.Fatal("expected cached details after saveDetails")
	}
	if got.Name != "Test Game" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Game")
	}
	if got.Summary != "A test game." {
		t.Errorf("Summary = %q, want %q", got.Summary, "A test game.")
	}
}

// TestDownloadImage_InvalidURL verifies that downloadImage returns an
// error for an invalid URL.
func TestDownloadImage_InvalidURL(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	_, _, err := c.downloadImage(
		"://invalid-url",
		filepath.Join(dir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
	if err == nil {
		t.Error("downloadImage should return error for invalid URL")
	}
}

// TestSaveDetails_EmptyCoverURL verifies that saveDetails handles an
// empty CoverURL gracefully (no download attempt).
func TestSaveDetails_EmptyCoverURL(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	details := &igdb.GameDetails{
		Name: "No Cover Game",
	}

	err := c.saveDetails("NES", "No Cover Game", details)
	if err != nil {
		t.Fatalf("saveDetails returned error: %v", err)
	}

	if details.CoverURL != "" {
		t.Errorf("CoverURL should remain empty, got %q", details.CoverURL)
	}

	// details.json should still be written
	got := c.Get("NES", "No Cover Game.nes")
	if got == nil {
		t.Fatal("expected cached details even without cover")
	}
	if got.Name != "No Cover Game" {
		t.Errorf("Name = %q, want %q", got.Name, "No Cover Game")
	}
}

// TestEnsureCoverThumbnail_NoCoverThumb verifies that ensureCoverThumbnail
// silently returns when no cover_thumb.jpg exists.
func TestEnsureCoverThumbnail_NoCoverThumb(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	// Should not panic or error when cover_thumb.jpg doesn't exist
	c.ensureCoverThumbnail("NES", "Test Game", "Test Game")

	// The cover path should not exist
	coverPath := coverPath(dir, "NES", "Test Game")
	if _, err := os.Stat(coverPath); err == nil {
		t.Errorf("cover should not exist when no cover_thumb.jpg exists")
	}
}

// TestEnsureCoverThumbnail_AlreadyExists verifies that ensureCoverThumbnail
// does not overwrite an existing cover file.
func TestEnsureCoverThumbnail_AlreadyExists(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	// Create the cover thumbnail source
	cacheDir := c.cacheDir("NES", "Test Game")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(cacheDir, "cover_thumb.jpg"),
		[]byte("new thumb data"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// Pre-create the destination cover
	coverPath := coverPath(dir, "NES", "Test Game")
	if err := os.MkdirAll(filepath.Dir(coverPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(coverPath, []byte("existing cover"), 0o644); err != nil {
		t.Fatal(err)
	}

	c.ensureCoverThumbnail("NES", "Test Game", "Test Game")

	// The existing cover should NOT be overwritten
	data, err := os.ReadFile(coverPath)
	if err != nil {
		t.Fatalf("reading cover: %v", err)
	}
	if string(data) != "existing cover" {
		t.Errorf(
			"ensureCoverThumbnail overwrote existing cover. got %q, want %q",
			string(data), "existing cover",
		)
	}
}

// TestWriteNotFound_CreatesMarker verifies that writeNotFound creates
// the .notfound marker file.
func TestWriteNotFound_CreatesMarker(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	c.writeNotFound("NES", "Unknown Game")

	path := filepath.Join(
		dir, "cache", "igdb", "NES", "Unknown Game", ".notfound",
	)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected .notfound marker at %q, got: %v", path, err)
	}
	if info.Size() != 0 {
		t.Errorf(".notfound marker should be empty, got %d bytes", info.Size())
	}
}

// TestWriteNotFound_ContentIsEmpty verifies that the .notfound marker
// contains an empty string (as written by writeNotFound).
func TestWriteNotFound_ContentIsEmpty(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, nil)

	c.writeNotFound("NES", "Unknown Game")

	path := filepath.Join(
		dir, "cache", "igdb", "NES", "Unknown Game", ".notfound",
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading .notfound: %v", err)
	}
	if len(data) != 0 {
		t.Errorf(".notfound content should be empty, got %q", string(data))
	}
}
