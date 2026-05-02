package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/scanner"
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
	srv.serveSecureFile(w, req, coverDir, "", longCacheImmutable)

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
	srv.serveSecureFile(w, req, coverDir, ".", longCacheImmutable)

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
			srv.serveSecureFile(w, req, coverDir, tc.file, longCacheImmutable)

			if w.Code != http.StatusNotFound {
				t.Errorf("file=%q: got status %d, want 404", tc.file, w.Code)
			}
		})
	}
}

// TestServeSecureFile_TrailingSeparatorInBaseDir verifies that a ROM path
// configured with a trailing path separator (e.g., path = "/mnt/roms/NES/"
// in the TOML config) still serves valid files. filepath.Clean inside
// serveSecureFile normalizes the trailing separator before the HasPrefix
// check, so the prefix defense does not reject valid files.
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

	cfg := &config.Config{
		Port: 8080,
		ROMs: map[string]config.ROM{
			"NES": {
				Path: romDir + "/", // trailing separator
				Core: "fceumm",
			},
		},
	}

	scn := scanner.New(cfg, dir)
	srv, err := New(cfg, dir, testFrontendFS, testEmulatorjsFS, nil, scn, nil)
	if err != nil {
		t.Fatal(err)
	}

	w := doRequest(t, srv, http.MethodGet, "/roms/NES/MegaMan.nes", nil)

	if w.Code != http.StatusOK {
		t.Errorf("trailing separator in baseDir: got status %d, want 200", w.Code)
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
		srv.serveSecureFile(w, req, coverDir, file, longCacheImmutable)

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
	srv.serveSecureFile(w, req, coverDir, "evil\x00.png", longCacheImmutable)

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
	srv.serveSecureFile(w, req, coverDir, "/etc/passwd", longCacheImmutable)

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
	srv.serveSecureFile(w, req, coverDir, "/", longCacheImmutable)

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
	srv.serveSecureFile(w, req, coverDir, "NES/game.png", longCacheImmutable)

	if w.Code != http.StatusOK {
		t.Errorf("valid subdir file: got status %d, want 200", w.Code)
	}
	if w.Body.String() != "png" {
		t.Errorf("body = %q, want %q", w.Body.String(), "png")
	}
}

// TestServeSecureFile_EncodedDotDotBlocked exercises the bypass class the
// existing FuzzServeSecureFile seed doesn't cover well: a URL that
// percent-encodes the '/' so http.ServeMux's path-cleaning step does NOT
// strip the traversal segment before the handler is invoked. The encoded
// rest reaches the handler containing literal "../" and must still be
// rejected — first by http.ServeFile's r.URL.Path check (400) when the
// decoded path contains "..", or by PathInside as defense-in-depth when it
// stays in baseDir. Either way: never 200 with content from outside.
//
// The NotFound and BadRequest both encode "did not serve outside file"; we
// pin the negative behavior (no escape) since the precise status varies
// with which layer fires first.
func TestServeSecureFile_EncodedDotDotBlocked(t *testing.T) {
	srv, dir := testServer(t)

	// Plant content outside covers that we definitely never want served
	// through the covers handler.
	outside := filepath.Join(dir, "outside-secret.txt")
	writeTestFile(t, outside, []byte("OUTSIDE-SECRET-CONTENT"))

	// And a real cover, so a path that lexically resolves inside still
	// has *something* to return without being a false-positive 404.
	writeTestFile(t, filepath.Join(dir, "covers", "NES", "real.png"), []byte("nes-content"))

	// All of these are URL shapes that can survive http.ServeMux path
	// cleaning by encoding '/' as %2F. The handler receives a `rest`
	// containing literal "../" segments.
	cases := []string{
		"/covers/NES%2F..%2F..%2Foutside-secret.txt",
		"/covers/NES%2F..%2F..%2F..%2Foutside-secret.txt",
		"/covers/..%2F..%2Foutside-secret.txt",
		"/covers/foo%2F..%2F..%2Foutside-secret.txt",
		// Encoded mixed with literal slashes
		"/covers/NES/..%2F..%2Foutside-secret.txt",
	}

	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			w := doRequest(t, srv, http.MethodGet, p, nil)
			// The negative invariant: the body must never contain the
			// outside-secret content. We don't pin a specific status
			// because either http.ServeFile (400) or PathInside (404)
			// could fire first depending on Go runtime version.
			if strings.Contains(w.Body.String(), "OUTSIDE-SECRET-CONTENT") {
				t.Errorf(
					"path %q served outside content (status=%d body=%q)",
					p, w.Code, w.Body.String(),
				)
			}
		})
	}
}

// TestServeSecureFile_LexicallyInsideButCrossesNamespace pins a subtle
// behavior of PathInside + filepath.Clean: a request like
// "a/b/../../etc/passwd" cleans to "etc/passwd" and stays *inside*
// baseDir — it does NOT escape to "/etc/passwd". The handler will look
// for a file literally at "<baseDir>/etc/passwd" and serve it if and
// only if such a file exists. The relevant correctness property: an
// attacker cannot exfiltrate files outside baseDir via this shape, but
// a request with this shape DOES land at a different in-baseDir file
// than the URL appears to ask for. Pin both halves.
func TestServeSecureFile_LexicallyInsideButCrossesNamespace(t *testing.T) {
	srv, dir := testServer(t)

	covers := filepath.Join(dir, "covers")
	if err := os.MkdirAll(covers, 0o755); err != nil {
		t.Fatal(err)
	}

	// Plant a file at <covers>/etc/passwd to demonstrate the resolution.
	weirdPath := filepath.Join(covers, "etc", "passwd")
	writeTestFile(t, weirdPath, []byte("inside-but-weird"))

	// "a/b/../../etc/passwd" → filepath.Clean → "etc/passwd"
	// → filepath.Join(covers, "etc/passwd") → <covers>/etc/passwd
	req := httptest.NewRequest("GET", "/covers/", nil)
	w := httptest.NewRecorder()
	srv.serveSecureFile(w, req, covers, "a/b/../../etc/passwd", longCacheImmutable)

	// In-baseDir resolution is fine; PathInside correctly accepts.
	if w.Code != http.StatusOK {
		t.Errorf(
			"expected 200 (file exists at <covers>/etc/passwd), got %d body=%q",
			w.Code, w.Body.String(),
		)
	}
	if w.Body.String() != "inside-but-weird" {
		t.Errorf("body = %q, want %q", w.Body.String(), "inside-but-weird")
	}

	// Negative invariant: confirm no file system traversal escaped baseDir.
	// Even though the request looked traversal-shaped, the served bytes
	// MUST be the in-baseDir file (or 404), never /etc/passwd.
	outsideEtcPasswd, err := os.ReadFile("/etc/passwd")
	if err == nil && bytes.Equal(w.Body.Bytes(), outsideEtcPasswd) {
		t.Errorf("served real /etc/passwd content — escape!")
	}
}

// TestServeSecureFile_SymlinkInsideBaseDirFollowsTarget pins current
// behavior: a symlink within baseDir whose target is OUTSIDE baseDir
// is followed by os.Stat and http.ServeFile, and the outside content
// is served. PathInside checks the lexical request path, not the
// resolved-symlink path.
//
// This is intentionally pinned (not flagged as a bug) because:
//   - dataDir is operator-controlled; planting a symlink there is
//     equivalent to filesystem write access, which already grants the
//     attacker direct read.
//   - Operators legitimately use symlinks to consolidate covers and
//     manuals across consoles (e.g., one cover for a multi-platform
//     game symlinked from each console subdirectory).
//
// A future hardening pass that adds O_NOFOLLOW or filepath.EvalSymlinks
// gating would break those workflows. This test catches that regression
// in either direction — a fresh maintainer who tightens the check
// without thinking through legit operator setups will see this fail.
//
// Documented as out-of-scope-but-flagged in the bug-hunt synthesis:
// it's a defense-in-depth gap, not an exploitable bug given threat
// model.
func TestServeSecureFile_SymlinkInsideBaseDirFollowsTarget(t *testing.T) {
	srv, dir := testServer(t)

	// Plant a target outside baseDir.
	outside := filepath.Join(dir, "outside-target.png")
	writeTestFile(t, outside, []byte("symlink-target-content"))

	// Create the covers tree.
	covers := filepath.Join(dir, "covers", "NES")
	if err := os.MkdirAll(covers, 0o755); err != nil {
		t.Fatal(err)
	}

	// Symlink inside covers pointing to the outside target.
	link := filepath.Join(covers, "linked.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	w := doRequest(t, srv, http.MethodGet, "/covers/NES/linked.png", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200 (symlink should be followed)", w.Code)
	}
	if w.Body.String() != "symlink-target-content" {
		t.Errorf(
			"body = %q, want %q (symlink target)",
			w.Body.String(), "symlink-target-content",
		)
	}
}

// TestHandleROM_FilePathValueIsSingleSegment pins that the {file} mux
// placeholder rejects multi-segment paths. handleROM trusts that {file}
// is a single segment (no '/'), so a regression that switched the
// pattern to {file...} would expand the attack surface to allow
// per-ROM-dir traversal — at which point SafePathSegment would belong
// on this handler too.
func TestHandleROM_FilePathValueIsSingleSegment(t *testing.T) {
	srv, dir := testServer(t)

	// Plant a file outside the NES ROM dir but inside the same parent.
	otherROM := filepath.Join(dir, "roms", "SNES", "OtherGame.sfc")
	writeTestFile(t, otherROM, []byte("snes-rom"))

	// /roms/NES/<file> — a {file} of "../SNES/OtherGame.sfc" must NOT
	// match the route (mux requires single segment) or, if it did, must
	// not serve OtherGame.sfc.
	w := doRequest(t, srv, http.MethodGet, "/roms/NES/..%2FSNES%2FOtherGame.sfc", nil)
	if w.Body.String() == "snes-rom" {
		t.Errorf(
			"served SNES ROM through NES route: status=%d body=%q",
			w.Code, w.Body.String(),
		)
	}
	// The {file} placeholder is single-segment so encoded slash gets
	// passed through as part of the literal filename. The on-disk file
	// "../SNES/OtherGame.sfc" doesn't exist under <romDir>, so 404.
	// Non-200 is the contract; specific code is implementation detail.
	if w.Code == http.StatusOK {
		t.Errorf("traversal-shaped {file} returned 200")
	}
}

// TestHandleROM_UnknownConsoleNeverReachesServeSecureFile pins the
// defense-by-membership behavior of handleROM: if the console isn't in
// s.cfg.ROMs, the handler short-circuits to 404 without consulting the
// filesystem. Belt-and-suspenders for a regression that swapped the
// order of the map lookup and serveSecureFile call.
//
// Distinct from TestROMServingUnknownConsole, which pins only the
// status code. This pins the side effect: nothing in <dataDir> or the
// ROM dirs is touched.
func TestHandleROM_UnknownConsoleNeverReachesServeSecureFile(t *testing.T) {
	srv, dir := testServer(t)

	// Plant a sentinel file with a known-attractive name in dataDir
	// itself. If a future regression let an unknown-console request
	// fall through to serveSecureFile with some default baseDir, we
	// want the test to fail loudly.
	sentinel := filepath.Join(dir, "shouldnt-be-served.txt")
	writeTestFile(t, sentinel, []byte("sentinel"))

	// Unknown console. Map lookup returns ok=false, handler 404s.
	w := doRequest(t, srv, http.MethodGet, "/roms/UNKNOWN/foo.bin", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "sentinel") {
		t.Errorf(
			"unknown-console request leaked sentinel content: body=%q",
			w.Body.String(),
		)
	}
}

// TestHandleCovers_NoConsoleMembershipGate pins the asymmetry the
// hotspot brief flagged: handleCovers does NOT validate {rest} via
// SafePathSegment (handleROM uses defense-by-membership in s.cfg.ROMs).
// Instead it trusts PathInside as the sole boundary. A request for a
// console that isn't configured (e.g. /covers/SegaCD/foo.png with no
// SegaCD in config) is served if and only if the file happens to exist
// on disk.
//
// This is current contract — covers/manuals/cache live outside config
// so an unknown-but-on-disk console subdirectory is legitimate. Pin
// the contract so a future commit that adds a membership gate doesn't
// silently break operator workflows where covers/ contains directories
// for consoles not in the active config.
func TestHandleCovers_NoConsoleMembershipGate(t *testing.T) {
	srv, dir := testServer(t)

	// Plant a cover for a console that is NOT in s.cfg.ROMs (which
	// only has NES per testServer).
	writeTestFile(
		t,
		filepath.Join(dir, "covers", "SegaCD", "Sonic.png"),
		[]byte("segacd-cover"),
	)

	w := doRequest(t, srv, http.MethodGet, "/covers/SegaCD/Sonic.png", nil)
	if w.Code != http.StatusOK {
		t.Errorf(
			"got status %d, want 200 — covers handler must not gate "+
				"on s.cfg.ROMs membership",
			w.Code,
		)
	}
	if w.Body.String() != "segacd-cover" {
		t.Errorf("body = %q, want %q", w.Body.String(), "segacd-cover")
	}
}

// TestServeSecureFile_NestedSubdirAllowed confirms multi-level
// subdirectories under baseDir are reachable. The {rest...} wildcard
// supports arbitrary depth; a regression that started rejecting
// multi-segment paths would silently break the IGDB cache layout
// (cache/igdb/<console>/<cleanName>/<file>).
func TestServeSecureFile_NestedSubdirAllowed(t *testing.T) {
	srv, dir := testServer(t)

	// 4 levels of subdirectory under cache/igdb to mimic real layout.
	deep := filepath.Join(dir, "cache", "igdb", "NES", "Mega Man", "extra", "deep.json")
	writeTestFile(t, deep, []byte("deep-content"))

	w := doRequest(
		t, srv, http.MethodGet,
		"/cache/igdb/NES/Mega%20Man/extra/deep.json", nil,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", w.Code)
	}
	if w.Body.String() != "deep-content" {
		t.Errorf("body = %q, want %q", w.Body.String(), "deep-content")
	}
}

// TestHandleCovers_BackslashNotTraversalOnLinux pins that a backslash
// in {rest} on Linux is treated as a literal filename character (Linux
// filesystems allow it). filepath.Clean does not interpret '\' as a
// separator on Linux, so PathInside resolves the request inside
// baseDir and the file is served if it exists. The current asymmetry
// versus parseSaveParams (which rejects '\' via SafePathSegment) is
// load-bearing: backslashes in legitimate cover/manual filenames must
// not be rejected. A regression that added SafePathSegment-style
// rejection here would refuse to serve files with '\' in their names.
//
// On Windows this test would behave differently (filepath.Separator =
// '\'), but Freeplay targets Linux servers; the GOOS=linux assumption
// is documented in CLAUDE.md / Makefile.
func TestHandleCovers_BackslashNotTraversalOnLinux(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skipf("Linux-only test (separator = %q)", filepath.Separator)
	}
	srv, dir := testServer(t)

	// "Game\Special.png" — a literal backslash in the filename. On
	// Linux this is a single filename, not a traversal sequence.
	const fname = "Game\\Special.png"
	writeTestFile(
		t, filepath.Join(dir, "covers", "NES", fname),
		[]byte("backslash-png"),
	)

	w := doRequest(t, srv, http.MethodGet, "/covers/NES/Game%5CSpecial.png", nil)
	if w.Code != http.StatusOK {
		t.Errorf(
			"got status %d, want 200 — backslash in filename must "+
				"not be treated as traversal on Linux (body=%q)",
			w.Code, w.Body.String(),
		)
	}
	if w.Body.String() != "backslash-png" {
		t.Errorf("body = %q, want %q", w.Body.String(), "backslash-png")
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
	srv.serveSecureFile(w, req, coverDir, ".gitkeep", longCacheImmutable)

	// Hidden files should be servable (they pass all security checks).
	if w.Code != http.StatusOK {
		t.Errorf("hidden file: got status %d, want 200", w.Code)
	}
}
