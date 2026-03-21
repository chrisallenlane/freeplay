package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/config"
)

// TestServeSecureFile_EmptyFilename verifies that requesting an empty filename
// (which filepath.Clean normalizes to ".") results in fullPath == baseDir.
// This exercises the fullPath == baseDir branch in serveSecureFile.
// Because baseDir is a directory, serveFile returns 404.
func TestServeSecureFile_EmptyFilename(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The {rest...} wildcard in /covers/{rest...} captures the path after
	// /covers/. An empty rest means requesting /covers/ itself.
	// We call serveSecureFile directly to test the branch.
	req := httptest.NewRequest("GET", "/covers/", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, "")

	if w.Code != http.StatusNotFound {
		t.Errorf("empty filename: got status %d, want 404", w.Code)
	}
}

// TestServeSecureFile_DotFilename verifies that "." as the filename triggers
// the fullPath == baseDir branch and returns 404 (since baseDir is a directory).
func TestServeSecureFile_DotFilename(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/.", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, ".")

	if w.Code != http.StatusNotFound {
		t.Errorf("dot filename: got status %d, want 404", w.Code)
	}
}

// TestServeSecureFile_DotDotBlocked verifies that ".." in the cleaned path
// is blocked before the HasPrefix check is reached.
func TestServeSecureFile_DotDotBlocked(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		file string
	}{
		{"simple dotdot", "../etc/passwd"},
		{"nested dotdot", "foo/../../etc/passwd"},
		{"only dotdot", ".."},
		{"quadruple dots", "...."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/covers/"+tc.file, nil)
			w := httptest.NewRecorder()
			srv.serveSecureFile(w, req, coverDir, tc.file)

			if w.Code != http.StatusNotFound {
				t.Errorf("file=%q: got status %d, want 404", tc.file, w.Code)
			}
		})
	}
}

// TestServeSecureFile_TrailingSeparatorInBaseDir demonstrates that if baseDir
// has a trailing path separator, the HasPrefix defense becomes
// HasPrefix(path, "/base//") which rejects ALL valid files. This can happen
// when a user specifies an absolute ROM path with a trailing slash in the
// TOML config file (e.g., path = "/mnt/roms/NES/"), because resolvePaths
// does not clean absolute paths.
func TestServeSecureFile_TrailingSeparatorInBaseDir(t *testing.T) {
	dir := t.TempDir()

	romDir := filepath.Join(dir, "roms", "NES")
	if err := os.MkdirAll(romDir, 0o755); err != nil {
		t.Fatal(err)
	}
	romFile := filepath.Join(romDir, "MegaMan.nes")
	if err := os.WriteFile(romFile, []byte("romdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate a user config with a trailing slash on the path
	cfg := &config.Config{
		Port: 8080,
		ROMs: map[string]config.ROM{
			"NES": {
				Path: romDir + "/", // trailing separator
				Core: "fceumm",
			},
		},
	}

	srv, err := New(cfg, dir, testFrontendFS, testEmulatorjsFS, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/roms/NES/MegaMan.nes", nil)
	w := httptest.NewRecorder()
	srv.handler.ServeHTTP(w, req)

	// BUG: When baseDir has a trailing slash, the HasPrefix check becomes
	// HasPrefix("/path/to/NES/MegaMan.nes", "/path/to/NES//") which is
	// always false. This causes valid ROM requests to return 404.
	if w.Code != http.StatusOK {
		t.Errorf(
			"trailing separator in baseDir: got status %d, want 200; "+
				"the HasPrefix defense incorrectly rejects valid files when "+
				"baseDir ends with a path separator",
			w.Code,
		)
	}
}

// TestServeSecureFile_SubdirectoryTraversal verifies that path traversal using
// subdirectory manipulation is blocked.
func TestServeSecureFile_SubdirectoryTraversal(t *testing.T) {
	srv, dir := testServer(t)

	// Create a file outside the covers directory
	secretFile := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Try to escape via sub/../.. patterns
	cases := []string{
		"sub/../../secret.txt",
		"a/b/c/../../../../secret.txt",
	}

	for _, file := range cases {
		req := httptest.NewRequest("GET", "/covers/"+file, nil)
		w := httptest.NewRecorder()
		srv.serveSecureFile(w, req, coverDir, file)

		if w.Code == http.StatusOK {
			t.Errorf(
				"path traversal via %q should not return 200 (body=%q)",
				file, w.Body.String(),
			)
		}
	}
}

// TestServeSecureFile_NullByteInFilename verifies that a null byte in the
// filename does not cause a panic or unexpected behavior.
func TestServeSecureFile_NullByteInFilename(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Null bytes in filenames should not cause a panic.
	// On Linux, the OS will reject them at the syscall level.
	req := httptest.NewRequest("GET", "/covers/evil", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, "evil\x00.png")

	// Should get 404 (file doesn't exist or OS rejects null byte), not 5xx
	if w.Code >= 500 {
		t.Errorf("null byte in filename: got status %d, want non-5xx", w.Code)
	}
}

// TestServeSecureFile_AbsolutePathInFile verifies that passing an absolute
// path as the file argument does not escape the base directory.
// filepath.Join(baseDir, "/etc/passwd") => baseDir + "/etc/passwd" on Linux
// (Join ignores leading slashes in non-first arguments on Unix).
func TestServeSecureFile_AbsolutePathInFile(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/etc/passwd", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, "/etc/passwd")

	// The file doesn't exist under covers, so 404 is expected.
	// The important thing is it does NOT serve /etc/passwd.
	if w.Code == http.StatusOK {
		t.Errorf("absolute path in file should not serve content outside baseDir")
	}
}

// TestServeSecureFile_SlashAsFilename verifies that "/" as the filename is
// handled safely (filepath.Clean("/") => "/", filepath.Join normalizes it
// to baseDir).
func TestServeSecureFile_SlashAsFilename(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, "/")

	// "/" cleans to "/" and Join(base, "/") produces base, which is a
	// directory, so serveFile returns 404.
	if w.Code != http.StatusNotFound {
		t.Errorf("slash filename: got status %d, want 404", w.Code)
	}
}

// TestServeSecureFile_ValidSubdirectoryFile verifies that files in
// subdirectories under baseDir are served correctly.
func TestServeSecureFile_ValidSubdirectoryFile(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	subDir := filepath.Join(coverDir, "NES")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "game.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/NES/game.png", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, "NES/game.png")

	if w.Code != http.StatusOK {
		t.Errorf("valid subdir file: got status %d, want 200", w.Code)
	}
	if w.Body.String() != "png" {
		t.Errorf("body = %q, want %q", w.Body.String(), "png")
	}
}

// TestServeSecureFile_HiddenFileAllowed verifies that dot-prefixed files
// (like .gitkeep) are not incorrectly blocked.
func TestServeSecureFile_HiddenFileAllowed(t *testing.T) {
	srv, dir := testServer(t)

	coverDir := filepath.Join(dir, "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(coverDir, ".gitkeep"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/covers/.gitkeep", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, coverDir, ".gitkeep")

	// Hidden files should be servable (they pass all security checks).
	if w.Code != http.StatusOK {
		t.Errorf("hidden file: got status %d, want 200", w.Code)
	}
}
