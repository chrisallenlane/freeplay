// Package main implements the freeplay retro-gaming server.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	freeplay "github.com/chrisallenlane/freeplay"
	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/covers"
	"github.com/chrisallenlane/freeplay/internal/scanner"
	"github.com/chrisallenlane/freeplay/internal/server"
)

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	os.Exit(1)
}

func main() {
	dataDir := flag.String("data", "/data", "path to data directory")
	flag.Parse()

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fatal(err)
	}

	// Set up cover art fetcher if configured
	var fetcher covers.Fetcher
	if cfg.CoverArtAPI == "igdb" {
		fetcher = covers.NewIGDBFetcher(cfg.CoverArtKey)
	}
	coverMgr := covers.New(*dataDir, fetcher)

	srv, err := server.New(cfg, *dataDir, freeplay.FrontendFS, freeplay.EmulatorjsFS, coverMgr)
	if err != nil {
		fatal(err)
	}

	// Wire cover art fetching to run after each scan
	srv.Scanner().SetOnScanComplete(func(games []scanner.Game) {
		entries := make([]covers.GameEntry, len(games))
		for i, g := range games {
			entries[i] = covers.GameEntry{Console: g.Console, Filename: g.Filename, IGDBPlatformIDs: g.IGDBPlatformIDs}
		}
		go func() {
			if coverMgr.FetchMissing(entries) > 0 {
				// Rescan so the catalog picks up newly fetched covers
				srv.Scanner().ScanBlocking()
			}
		}()
	})

	// Trigger initial ROM scan asynchronously
	go srv.Scanner().ScanBlocking()

	slog.Info("starting freeplay", "port", cfg.Port, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		fatal(err)
	}
}
