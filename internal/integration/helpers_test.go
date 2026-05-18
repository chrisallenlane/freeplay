//go:build integration

// Package integration runs cross-package integration tests against the
// real freeplay server: actual scanner, real saves manager, real
// embedded FrontendFS and EmulatorjsFS, full middleware chain. Build
// tag keeps it out of `go test ./...` so unit-test feedback stays
// fast — run via `make integration`.
package integration

import (
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	freeplay "github.com/chrisallenlane/freeplay"
	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/details"
	"github.com/chrisallenlane/freeplay/internal/library"
	"github.com/chrisallenlane/freeplay/internal/scanner"
	"github.com/chrisallenlane/freeplay/internal/server"
)

// repoRoot resolves the project root from the package directory so
// helpers can copy testdata regardless of where `go test` is invoked.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// We're at <root>/internal/integration when the test runs.
	return filepath.Join(wd, "..", "..")
}

// copyTree copies srcDir into dstDir, preserving file modes. The
// "saves" subtree is skipped — it contains root-owned fixtures from
// prior tests, and integration tests write their own saves anyway.
// Symlinks are not expected in testdata.
func copyTree(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		if rel == "saves" || strings.HasPrefix(rel, "saves"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(dst, info.Mode())
		}
		data, err := os.ReadFile(p) //nolint:gosec // test fixture I/O
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("copy testdata: %v", err)
	}
}

// freshDataDir returns a per-test copy of ./testdata under t.TempDir().
// Tests can mutate the returned directory freely; cleanup happens
// automatically via t.TempDir.
func freshDataDir(t *testing.T) string {
	t.Helper()
	src := filepath.Join(repoRoot(t), "testdata")
	dst := t.TempDir()
	copyTree(t, src, dst)
	return dst
}

// bootServer wires up the production stack — scanner, library, details
// cache (with IGDB disabled to keep tests offline), real embedded FSes
// — and wraps the resulting Server in httptest.NewServer. Returns the
// test server and the dataDir so tests can inspect on-disk side
// effects. The IGDB pipeline is never started (no lib.Start), so no
// network calls fire.
func bootServer(t *testing.T, dataDir string) (*httptest.Server, *server.Server) {
	t.Helper()

	cfg, err := config.Load(dataDir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	// Disable IGDB so the details cache has no fetcher; handleGameDetails
	// reads only what's already on disk under <dataDir>/cache/igdb.
	cfg.CoverArtAPI = ""

	detailsCache := details.New(dataDir, nil)
	scn := scanner.New(cfg, dataDir)
	lib := library.New(scn, detailsCache)

	srv, err := server.New(
		cfg, dataDir,
		freeplay.FrontendFS, freeplay.EmulatorjsFS,
		detailsCache, scn, lib,
		"test",
	)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	// Populate the catalog synchronously — no enrichment, no IGDB.
	scn.ScanBlocking()

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, srv
}
