//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// game mirrors the subset of scanner.Game the integration tests need
// from /api/games. Decoupled from the production type so a new field
// added there doesn't ripple into every test.
type game struct {
	Filename  string `json:"filename"`
	Console   string `json:"console"`
	HasCover  bool   `json:"hasCover"`
	HasManual bool   `json:"hasManual"`
}

type catalog struct {
	Consoles []string `json:"consoles"`
	Games    []game   `json:"games"`
}

// fetchCatalog GETs /api/games and decodes the catalog. Fails the test
// on any error.
func fetchCatalog(t *testing.T, baseURL string) catalog {
	t.Helper()
	res, err := http.Get(baseURL + "/api/games") //nolint:gosec,noctx // test code, baseURL is httptest.NewServer
	if err != nil {
		t.Fatalf("GET /api/games: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/games: status %d", res.StatusCode)
	}
	var cat catalog
	if err := json.NewDecoder(res.Body).Decode(&cat); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	return cat
}

// stripExt mirrors the frontend's stripExt: strip the final dot
// extension if the dot is at index > 0. Lets the integration tests
// build the URLs the frontend would build without depending on a JS
// runtime.
func stripExt(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot > 0 {
		return filename[:dot]
	}
	return filename
}

// TestGameDetailsURLAcceptsCatalogFilenames pins that every filename
// the scanner emits in /api/games is accepted (or 404'd, with cache
// empty) by the /api/game-details endpoint. A 400 response means
// SafePathSegment rejected a filename — i.e., the scanner produced
// a filename the details handler refuses to look up. That class of
// regression silently breaks the details page for affected ROMs.
func TestGameDetailsURLAcceptsCatalogFilenames(t *testing.T) {
	dataDir := freshDataDir(t)
	ts, _ := bootServer(t, dataDir)

	cat := fetchCatalog(t, ts.URL)
	if len(cat.Games) == 0 {
		t.Fatal("catalog must not be empty for this test to mean anything")
	}

	for _, g := range cat.Games {
		u := fmt.Sprintf(
			"%s/api/game-details?console=%s&rom=%s",
			ts.URL,
			url.QueryEscape(g.Console),
			url.QueryEscape(g.Filename),
		)
		res, err := http.Get(u) //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("GET %s: %v", u, err)
		}
		_ = res.Body.Close()
		switch res.StatusCode {
		case http.StatusOK, http.StatusNotFound:
			// 200 = cached entry, 404 = no cache (acceptable; IGDB
			// disabled in test). Both mean the URL was accepted.
		default:
			t.Errorf(
				"game-details for %s/%s: status %d, want 200 or 404",
				g.Console, g.Filename, res.StatusCode,
			)
		}
	}
}

// TestCoverManualURLsForRealCatalog asserts that the URLs the frontend
// builds for covers and manuals (slug shape — strip the extension off
// the filename, append .png/.pdf) resolve to 200 for every game whose
// catalog entry advertises hasCover or hasManual. Catches: scanner
// flagging a cover/manual as present at a path the static-file route
// can't find — the cover/manual analog of the slug-vs-filename bug.
func TestCoverManualURLsForRealCatalog(t *testing.T) {
	dataDir := freshDataDir(t)
	ts, _ := bootServer(t, dataDir)
	cat := fetchCatalog(t, ts.URL)

	covered := 0
	manualed := 0
	for _, g := range cat.Games {
		slug := stripExt(g.Filename)
		if g.HasCover {
			covered++
			u := fmt.Sprintf(
				"%s/covers/%s/%s.png",
				ts.URL, url.PathEscape(g.Console), url.PathEscape(slug),
			)
			res, err := http.Get(u) //nolint:gosec,noctx
			if err != nil {
				t.Fatalf("GET %s: %v", u, err)
			}
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Errorf(
					"cover for %s/%s: status %d, want 200",
					g.Console, g.Filename, res.StatusCode,
				)
			}
		}
		if g.HasManual {
			manualed++
			u := fmt.Sprintf(
				"%s/manuals/%s/%s.pdf",
				ts.URL, url.PathEscape(g.Console), url.PathEscape(slug),
			)
			res, err := http.Get(u) //nolint:gosec,noctx
			if err != nil {
				t.Fatalf("GET %s: %v", u, err)
			}
			_ = res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Errorf(
					"manual for %s/%s: status %d, want 200",
					g.Console, g.Filename, res.StatusCode,
				)
			}
		}
	}

	if covered == 0 && manualed == 0 {
		t.Fatal("testdata had no covers or manuals; assertion vacuously passed")
	}
}

// TestEmulatorJSLoaderResolvesEmbeddedDeps verifies the embedded
// EmulatorJS tree contains the assets the loader will request. Unit
// tests stub the FS with three files; this test boots against the
// real freeplay.EmulatorjsFS so a `make fetch-emulatorjs` regression
// or a //go:embed pattern drift surfaces as a 404 here.
func TestEmulatorJSLoaderResolvesEmbeddedDeps(t *testing.T) {
	dataDir := freshDataDir(t)
	ts, _ := bootServer(t, dataDir)

	// Sample paths the loader and its descendants reach for. Not
	// exhaustive — we just need a 404-trigger if any of the major
	// subtrees disappear from the embed.
	paths := []string{
		"/emulatorjs/data/loader.js",
		"/emulatorjs/data/version.json",
		"/emulatorjs/data/emulator.min.js",
		"/emulatorjs/data/src/emulator.js",
		"/emulatorjs/data/src/GameManager.js",
	}
	for _, p := range paths {
		res, err := http.Get(ts.URL + p) //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", p, res.StatusCode)
		}
	}
}

// TestEmulatorJSMinifiedBundleHasLightgunPatches asserts that the
// embedded emulator.min.js contains the controller-port-device API
// that lightgun support depends on (merged upstream as EmulatorJS
// PR #1182). A future EMULATORJS_TAG bump to a release that loses or
// reverts the patches would fail this test instead of breaking the
// lightgun UX silently.
func TestEmulatorJSMinifiedBundleHasLightgunPatches(t *testing.T) {
	dataDir := freshDataDir(t)
	ts, _ := bootServer(t, dataDir)

	res, err := http.Get(ts.URL + "/emulatorjs/data/emulator.min.js") //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("GET emulator.min.js: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET emulator.min.js: status %d, want 200", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// setControllerPortDevice is the RetroArch C API exposed to the JS
	// binding by the patches in EmulatorJS PR #1182 / RetroArch PR #38.
	// Minification preserves cross-language symbol names, so this is the
	// stable marker for the lightgun patches in the minified bundle.
	if !bytes.Contains(body, []byte("setControllerPortDevice")) {
		t.Errorf("emulator.min.js missing setControllerPortDevice; " +
			"lightgun patches may have been dropped from the pinned " +
			"EmulatorJS release")
	}
}

// TestRescanReflectsNewROM exercises the full scan→catalog→slug-gate
// pipeline through the real library.Library and scanner.Scanner.
// Adds a ROM file mid-test, triggers a rescan, then verifies the
// new game appears in the catalog *and* its slug is POST-acceptable
// for saves. The slug-gate check is the load-bearing assertion: a
// rescan that updates the catalog but fails to rebuild the slugSet
// would leave the new game unsaveable.
func TestRescanReflectsNewROM(t *testing.T) {
	dataDir := freshDataDir(t)
	ts, _ := bootServer(t, dataDir)

	before := fetchCatalog(t, ts.URL)
	beforeCount := len(before.Games)

	// Drop a new ROM into testdata's NES dir.
	const newROM = "Integration Test ROM.zip"
	romPath := filepath.Join(dataDir, "roms", "nes", newROM)
	if err := os.WriteFile(romPath, []byte("not actually a rom"), 0o644); err != nil {
		t.Fatalf("write new ROM: %v", err)
	}

	// Trigger rescan via the API. CSRF middleware requires
	// X-Requested-With on POST endpoints.
	rescanReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/rescan", nil)
	if err != nil {
		t.Fatalf("build rescan request: %v", err)
	}
	rescanReq.Header.Set("X-Requested-With", "freeplay")
	res, err := http.DefaultClient.Do(rescanReq)
	if err != nil {
		t.Fatalf("POST /api/rescan: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/rescan: status %d, want 200", res.StatusCode)
	}

	// Poll until the pipeline finishes. With IGDB disabled, this
	// settles in milliseconds — the timeout is a backstop.
	deadline := time.Now().Add(5 * time.Second)
	for {
		statusRes, err := http.Get(ts.URL + "/api/status") //nolint:gosec,noctx
		if err != nil {
			t.Fatalf("GET /api/status: %v", err)
		}
		var s struct {
			FetchingDetails bool `json:"fetchingDetails"`
		}
		_ = json.NewDecoder(statusRes.Body).Decode(&s)
		_ = statusRes.Body.Close()
		if !s.FetchingDetails {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rescan did not settle within 5s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	after := fetchCatalog(t, ts.URL)
	if len(after.Games) != beforeCount+1 {
		t.Fatalf(
			"catalog count after rescan: %d, want %d",
			len(after.Games), beforeCount+1,
		)
	}
	found := false
	for _, g := range after.Games {
		if g.Console == "NES" && g.Filename == newROM {
			found = true
		}
	}
	if !found {
		t.Errorf("new ROM %q not present in post-rescan catalog", newROM)
	}

	// The new game's slug must be saveable. This is the post-rescan
	// slugSet rebuild check.
	slug := stripExt(newROM)
	saveURL := fmt.Sprintf(
		"%s/api/saves/NES/%s/sram",
		ts.URL, url.PathEscape(slug),
	)
	req, err := http.NewRequest(http.MethodPost, saveURL, bytes.NewReader([]byte("data")))
	if err != nil {
		t.Fatalf("build save request: %v", err)
	}
	req.Header.Set("X-Requested-With", "freeplay")
	postRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	_ = postRes.Body.Close()
	if postRes.StatusCode != http.StatusOK {
		t.Errorf(
			"POST save for new ROM (slug=%q): status %d, want 200",
			slug, postRes.StatusCode,
		)
	}
}

// TestSaveSurvivesRestart confirms saves on disk are readable by a
// fresh server instance pointed at the same dataDir. Catches: any
// drift between writer-side and reader-side path construction in
// datadir.SavePath, atomicfile rename failures that leave a temp
// file but no real file, or fsync regressions that mask data on
// process restart.
func TestSaveSurvivesRestart(t *testing.T) {
	dataDir := freshDataDir(t)

	// Server 1: write a save.
	ts1, _ := bootServer(t, dataDir)
	saveData := []byte{0x01, 0x02, 0x03, 0x04, 0xff, 0x00, 0xab, 0xcd}
	saveURL := ts1.URL + "/api/saves/NES/Mega%20Man/sram"

	req, err := http.NewRequest(http.MethodPost, saveURL, bytes.NewReader(saveData))
	if err != nil {
		t.Fatalf("build save request: %v", err)
	}
	req.Header.Set("X-Requested-With", "freeplay")
	postRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST save: %v", err)
	}
	_ = postRes.Body.Close()
	if postRes.StatusCode != http.StatusOK {
		t.Fatalf("POST save: status %d, want 200", postRes.StatusCode)
	}
	ts1.Close()

	// Server 2: same dataDir, fresh process state.
	ts2, _ := bootServer(t, dataDir)
	getRes, err := http.Get(ts2.URL + "/api/saves/NES/Mega%20Man/sram") //nolint:gosec,noctx
	if err != nil {
		t.Fatalf("GET save after restart: %v", err)
	}
	defer getRes.Body.Close()
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("GET save after restart: status %d, want 200", getRes.StatusCode)
	}
	body, err := io.ReadAll(getRes.Body)
	if err != nil {
		t.Fatalf("read save body: %v", err)
	}
	if !bytes.Equal(body, saveData) {
		t.Errorf(
			"GET save body after restart: got %x, want %x",
			body, saveData,
		)
	}
}
