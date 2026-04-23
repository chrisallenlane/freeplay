package details

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chrisallenlane/freeplay/internal/datadir"
	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// seedCachedDetails writes a details.json file into the cache at the
// canonical layout for the given (console, cleanName). Creates parent
// directories. Fails the test on any error.
func seedCachedDetails(
	t *testing.T,
	dataDir, console, cleanName string,
	d *igdb.GameDetails,
) {
	t.Helper()
	dir := filepath.Join(datadir.IGDBCache(dataDir), console, cleanName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "details.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// seedNotFound writes the .notfound marker for (console, cleanName).
// Creates parent directories. Fails the test on any error.
func seedNotFound(t *testing.T, dataDir, console, cleanName string) {
	t.Helper()
	dir := filepath.Join(datadir.IGDBCache(dataDir), console, cleanName)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".notfound"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// startImageServerWith starts an httptest.Server using handler and registers
// t.Cleanup(srv.Close). Use this when the test needs a custom handler.
func startImageServerWith(
	t *testing.T,
	handler http.HandlerFunc,
) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// startFakeImageServer starts an httptest.Server that always returns a minimal
// image/jpeg body. It registers t.Cleanup(srv.Close).
func startFakeImageServer(t *testing.T) *httptest.Server {
	t.Helper()
	return startImageServerWith(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fakeimage"))
	})
}

// downloadTestImage calls c.downloadImage with the canonical test arguments
// (cacheDir = <tempDir>/cache, urlBase = /cache/igdb/NES/Game,
// filename = cover.jpg) and returns the results unchanged. Use it when the
// test does not care about the specific subpath or filename and only needs to
// exercise the download behaviour itself.
func downloadTestImage(
	t *testing.T,
	c *Cache,
	rawURL string,
) (localPath, localURL string, err error) {
	t.Helper()
	return c.downloadImage(
		rawURL,
		filepath.Join(c.dataDir, "cache"),
		"/cache/igdb/NES/Game",
		"cover.jpg",
	)
}
