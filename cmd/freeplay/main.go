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
	"github.com/chrisallenlane/freeplay/internal/details"
	"github.com/chrisallenlane/freeplay/internal/scanner"
	"github.com/chrisallenlane/freeplay/internal/server"
)

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	os.Exit(1)
}

func main() {
	dataDir := flag.String("data", "/data", "path to data directory")
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

	// Set up IGDB fetcher and details cache if configured
	var igdbFetcher *covers.IGDBFetcher
	if cfg.CoverArtAPI == "igdb" {
		igdbFetcher = covers.NewIGDBFetcher(cfg.CoverArtKey)
	}

	detailsCache := details.New(*dataDir, igdbFetcher)

	var detailsFetcher server.DetailsFetcher
	if igdbFetcher != nil {
		detailsFetcher = igdbFetcher
	}

	srv, err := server.New(
		cfg, *dataDir,
		freeplay.FrontendFS, freeplay.EmulatorjsFS,
		detailsCache, detailsFetcher,
	)
	if err != nil {
		fatal(err)
	}

	// Wire details cache population to run after each scan
	srv.Scanner().SetOnScanComplete(func(games []scanner.Game) {
		entries := make([]covers.GameEntry, len(games))
		for i, g := range games {
			entries[i] = covers.GameEntry{
				Console:         g.Console,
				Filename:        g.Filename,
				IGDBPlatformIDs: g.IGDBPlatformIDs,
			}
		}
		go func() {
			if detailsCache.FetchAll(entries) > 0 {
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
