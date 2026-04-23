//go:build debug

package server

import (
	"log/slog"
	"net/http"
	"time"

	// Registers pprof handlers on http.DefaultServeMux.
	_ "net/http/pprof"
)

// The debug build-tag wires up net/http/pprof on a separate listener
// bound to 127.0.0.1:6060. Non-debug builds omit this file entirely so
// the standard binary has no pprof exposure. Use `make build-debug`
// and capture profiles via:
//
//	go tool pprof -http :8081 http://localhost:6060/debug/pprof/profile?seconds=30
//	go tool pprof -http :8081 http://localhost:6060/debug/pprof/allocs
func init() {
	go func() {
		slog.Info("pprof listener on 127.0.0.1:6060 (debug build)")
		srv := &http.Server{
			Addr:              "127.0.0.1:6060",
			ReadHeaderTimeout: 10 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			slog.Warn("pprof listener stopped", "error", err)
		}
	}()
}
