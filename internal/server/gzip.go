package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

// gzipWriterPool reuses gzip.Writer instances across requests. Each
// Writer holds ~96 KB of compression state; pooling avoids per-request
// allocations on a workload where every text response pays the cost.
var gzipWriterPool = sync.Pool{
	New: func() any {
		return gzip.NewWriter(io.Discard)
	},
}

// compressibleContentTypes is the allowlist of response MIMEs worth
// compressing. Anything outside this set (images, fonts, WASM cores on
// some builds, application/octet-stream save blobs) passes through
// uncompressed.
var compressibleContentTypes = map[string]struct{}{
	"application/json":       {},
	"application/javascript": {},
	"application/wasm":       {},
	"application/xml":        {},
	"image/svg+xml":          {},
	"text/css":               {},
	"text/html":              {},
	"text/javascript":        {},
	"text/plain":             {},
}

// gzipMiddleware wraps next so that responses with a compressible
// Content-Type are gzip-encoded when the client sends
// `Accept-Encoding: gzip`. Non-compressible responses (images, fonts,
// binary blobs) pass through untouched. Sets `Vary: Accept-Encoding`
// on compressed responses so caches key correctly.
func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w)
		grw := &gzipResponseWriter{ResponseWriter: w, gz: gz}
		defer func() {
			if grw.compressing {
				_ = gz.Close()
			}
			gzipWriterPool.Put(gz)
		}()
		next.ServeHTTP(grw, r)
	})
}

// gzipResponseWriter is an http.ResponseWriter that compresses the body
// via gz when the upstream handler's declared Content-Type is in the
// compressibleContentTypes allowlist. The compressing decision is made
// exactly once, on the first Write or WriteHeader call; later calls
// honor it consistently.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
	// headerSet is true once WriteHeader has run (either by the handler
	// or by the implicit call inside Write). Decouples the
	// compressing-decision bookkeeping from upstream's code path.
	headerSet bool
	// compressing indicates the response body will be gzip-encoded.
	// Only meaningful once headerSet is true.
	compressing bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	if !g.headerSet {
		g.headerSet = true
		if shouldCompress(g.Header().Get("Content-Type")) {
			g.compressing = true
			g.Header().Set("Content-Encoding", "gzip")
			g.Header().Del("Content-Length")
			g.Header().Add("Vary", "Accept-Encoding")
		}
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.headerSet {
		g.WriteHeader(http.StatusOK)
	}
	if g.compressing {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func shouldCompress(contentType string) bool {
	if i := strings.IndexByte(contentType, ';'); i != -1 {
		contentType = contentType[:i]
	}
	contentType = strings.TrimSpace(contentType)
	_, ok := compressibleContentTypes[contentType]
	return ok
}
