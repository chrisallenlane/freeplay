// Package main implements the freeplay retro-gaming server.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	freeplay "github.com/chrisallenlane/freeplay"
	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/details"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/library"
	"github.com/chrisallenlane/freeplay/internal/scanner"
	"github.com/chrisallenlane/freeplay/internal/server"
)

// parseLogLevel converts a LOG_LEVEL string to a slog.Level. Case-insensitive.
// Unrecognised values fall back to slog.LevelInfo. The caller is responsible
// for warning about the fallback.
func parseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	os.Exit(1)
}

func main() {
	// Configure slog before any other calls so all startup messages use
	// the correct handler. Level is controlled by LOG_LEVEL env var;
	// unrecognised values fall back to info with a one-time warning.
	logLevelStr := os.Getenv("LOG_LEVEL")
	logLevel, ok := parseLogLevel(logLevelStr)
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
	if !ok {
		slog.Warn(
			"unrecognised LOG_LEVEL value; defaulting to info",
			"value", logLevelStr,
		)
	}

	dataDir := flag.String("data", "/data", "path to data directory")
	port := flag.Int("port", 0, "override port from config (0 = use config value)")
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *version {
		fmt.Println(freeplay.Version)
		return
	}

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fatal(err)
	}
	if *port != 0 {
		cfg.Port = *port
	}

	// Set up IGDB fetcher and details cache if configured. The if/else
	// is load-bearing: passing a typed-nil *igdb.Fetcher to details.New
	// would yield a typed-nil interface inside the Cache, and the
	// (c.fetcher == nil) guard in FetchAll wouldn't catch it — the
	// background pipeline would then dereference nil and crash.
	var detailsCache *details.Cache
	if cfg.CoverArtAPI == "igdb" {
		detailsCache = details.New(*dataDir, igdb.NewFetcher(cfg.CoverArtKey))
	} else {
		detailsCache = details.New(*dataDir, nil)
	}

	scn := scanner.New(cfg, *dataDir)
	lib := library.New(scn, detailsCache)

	srv, err := server.New(
		cfg, *dataDir,
		freeplay.FrontendFS, freeplay.EmulatorjsFS,
		detailsCache, scn, lib,
	)
	if err != nil {
		fatal(err)
	}

	// Kick off the initial scan/enrich/fetch pipeline in the background.
	lib.Start()

	slog.Info("starting freeplay", "port", cfg.Port, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		fatal(err)
	}
}
