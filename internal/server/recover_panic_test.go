package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRecoverPanicStripsLongCacheHeaders pins finding #1 from the
// post-#57 sweep: when an inner handler under a cacheControl /
// cacheWithETag wrapper panics, the recovery middleware emits a 500
// via http.Error. Go's http.Error does NOT clear Cache-Control / ETag
// / Last-Modified — only the unexported http.serveError (fs.go:188)
// does. Without the explicit strip in recoverPanic, the 500 response
// inherits the long-cache directive set by the upstream middleware
// and clients cache the failure for the full max-age window.
//
// Mirrors the noDirListing strip-on-404 pattern (middleware.go) for
// the 5xx case. Builds the middleware chain manually because there
// is no production code path that panics on demand.
func TestRecoverPanicStripsLongCacheHeaders(t *testing.T) {
	panicker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic — should not crash the server")
	})

	// Match production composition: cacheWithETag sets the long-cache
	// headers, recoverPanic catches the panic, logRequests is outermost
	// (recoverPanic requires *loggingResponseWriter to actually emit
	// the 500 — see the type assertion in recoverPanic).
	chain := logRequests(recoverPanic(cacheWithETag(
		`"test-etag"`, longCache, panicker,
	)))

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("Cache-Control = %q on 500 response; must be cleared so "+
			"the failure is not cached for the full max-age window", cc)
	}
	if et := w.Header().Get("ETag"); et != "" {
		t.Errorf("ETag = %q on 500 response; must be cleared", et)
	}
	if lm := w.Header().Get("Last-Modified"); lm != "" {
		t.Errorf("Last-Modified = %q on 500 response; must be cleared", lm)
	}
}
