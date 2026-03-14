package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/covers"
	"github.com/chrisallenlane/freeplay/internal/scanner"
	"github.com/chrisallenlane/freeplay/internal/server"
)

//go:embed frontend
var frontendFS embed.FS

//go:embed emulatorjs
var emulatorjsFS embed.FS

func main() {
	dataDir := flag.String("data", "/data", "path to data directory")
	flag.Parse()

	cfg, err := config.Load(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}

	// Set up cover art fetcher if configured
	var fetcher covers.CoverFetcher
	if cfg.CoverArtAPI == "igdb" {
		fetcher = covers.NewIGDBFetcher(cfg.CoverArtKey)
	}
	coverMgr := covers.New(*dataDir, fetcher)

	srv := server.New(cfg, *dataDir, frontendFS, emulatorjsFS)

	// Wire cover art fetching to run after each scan
	srv.Scanner().SetOnScanComplete(func(games []scanner.Game) {
		entries := make([]covers.GameEntry, len(games))
		for i, g := range games {
			entries[i] = covers.GameEntry{Console: g.Console, Filename: g.Filename}
		}
		go coverMgr.FetchMissing(entries)
	})

	// Trigger initial ROM scan asynchronously
	go srv.Scanner().ScanBlocking()

	slog.Info("starting freeplay", "port", cfg.Port, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
