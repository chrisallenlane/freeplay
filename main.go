package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/chrisallenlane/freeplay/internal/config"
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

	srv := server.New(cfg, *dataDir, frontendFS, emulatorjsFS)

	slog.Info("starting freeplay", "port", cfg.Port, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}
