// Package server implements the freeplay HTTP server.
package server

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/chrisallenlane/freeplay/internal/config"
	"github.com/chrisallenlane/freeplay/internal/igdb"
	"github.com/chrisallenlane/freeplay/internal/saves"
	"github.com/chrisallenlane/freeplay/internal/scanner"
)

// longCacheImmutable is for content-addressed assets (URL changes when
// bytes change) — browsers skip revalidation entirely.
const longCacheImmutable = "public, max-age=31536000, immutable"

// longCacheMutable is for static-ish files whose bytes may change
// behind a stable URL (covers re-downloaded after an IGDB rescan,
// operator-updated manuals). Browsers still cache aggressively but
// revalidate via If-Modified-Since once the max-age expires.
const longCacheMutable = "public, max-age=31536000"

// DetailsCache serves locally-cached game metadata.
type DetailsCache interface {
	Get(console, romFilename string) *igdb.GameDetails
	Fetching() bool
}

// Rescanner triggers the scan/enrich/fetch pipeline in response to an
// HTTP /api/rescan request. TriggerRescan returns false if a pipeline is
// already running; the server maps that to HTTP 409.
type Rescanner interface {
	TriggerRescan() bool
}

// Server is the Freeplay HTTP server.
type Server struct {
	cfg           *config.Config
	dataDir       string
	scanner       *scanner.Scanner
	rescanner     Rescanner
	saves         *saves.Manager
	detailsCache  DetailsCache
	frontendSub   fs.FS
	emulatorjsSub fs.FS
	mux           *http.ServeMux
	handler       http.Handler
}

// New creates a configured Server ready to listen. detailsCache may be
// nil if IGDB is not configured. rescanner may be nil, in which case
// POST /api/rescan returns 503.
func New(
	cfg *config.Config,
	dataDir string,
	frontendFS, emulatorjsFS fs.FS,
	detailsCache DetailsCache,
	scn *scanner.Scanner,
	rescanner Rescanner,
) (*Server, error) {
	frontendSub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		return nil, fmt.Errorf("frontend fs: %w", err)
	}
	emulatorjsSub, err := fs.Sub(emulatorjsFS, "emulatorjs")
	if err != nil {
		return nil, fmt.Errorf("emulatorjs fs: %w", err)
	}

	s := &Server{
		cfg:           cfg,
		dataDir:       dataDir,
		scanner:       scn,
		rescanner:     rescanner,
		saves:         saves.New(dataDir),
		detailsCache:  detailsCache,
		frontendSub:   frontendSub,
		emulatorjsSub: emulatorjsSub,
		mux:           http.NewServeMux(),
	}
	s.routes()
	// gzip is outermost so it sees all responses (including those
	// emitted by securityHeaders short-circuits); securityHeaders
	// stays inside so its headers apply to both paths.
	s.handler = gzipMiddleware(securityHeaders(s.mux))
	return s, nil
}

// ListenAndServe starts the HTTP server with production timeouts.
func (s *Server) ListenAndServe() error {
	return s.newHTTPServer().ListenAndServe()
}

// newHTTPServer constructs the http.Server with explicit timeouts.
// Timeouts defeat slow-body drip attacks (see SEC-1) that otherwise
// pin heap memory per idle connection. WriteTimeout of 60s is generous
// enough for a 64 MiB save upload over LAN.
func (s *Server) newHTTPServer() *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.Port),
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 14,
	}
}
