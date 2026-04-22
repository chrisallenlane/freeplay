// Package main implements the freeplay retro-gaming server.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	freeplay "github.com/chrisallenlane/freeplay"
	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/details"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/library"
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
	var igdbFetcher *igdb.Fetcher
	if cfg.CoverArtAPI == "igdb" {
		igdbFetcher = igdb.NewFetcher(cfg.CoverArtKey)
	}
	detailsCache := details.New(*dataDir, igdbFetcher)

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
