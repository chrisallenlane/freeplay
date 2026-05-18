package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestEmulatorJSETagPresent verifies that 2xx responses under
// /emulatorjs/ carry a version-stamped ETag and a Cache-Control header
// without the `immutable` directive. Closes the cache-immutability trap
// that broke v1.1.1 — see ticket #57 and the parallel 404-side test in
// cachecontrol_404_test.go.
func TestEmulatorJSETagPresent(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	etag := w.Header().Get("ETag")
	if etag != `"freeplay-test"` {
		t.Errorf("ETag = %q, want %q (derived from server version)",
			etag, `"freeplay-test"`)
	}

	cc := w.Header().Get("Cache-Control")
	if cc == "" {
		t.Errorf("Cache-Control header missing")
	}
	if strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q must not contain `immutable` — the "+
			"directive blocks revalidation even on hard refresh and traps "+
			"users on stale assets after a Freeplay release", cc)
	}
}

// TestEmulatorJSETagMatchReturns304 verifies the revalidation path:
// a request whose If-None-Match matches the server's ETag gets a 304
// with no body. This is what lets browsers cache aggressively while
// still picking up a Freeplay release without manual intervention.
func TestEmulatorJSETagMatchReturns304(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil)
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("first GET missing ETag — fixture broken")
	}

	w2 := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil,
		http.Header{"If-None-Match": []string{etag}})
	if w2.Code != http.StatusNotModified {
		t.Errorf("matching If-None-Match: status = %d, want 304", w2.Code)
	}
	if w2.Body.Len() != 0 {
		t.Errorf("304 response carried body of %d bytes; should be empty",
			w2.Body.Len())
	}
}

// TestEmulatorJSETagMismatchReturns200 verifies the release-invalidation
// path: a request whose If-None-Match does NOT match the server's ETag
// (because the operator cut a new release with a different version)
// gets a fresh 200 with the current bytes. This is the self-healing
// behavior that v1.1.1 needed.
func TestEmulatorJSETagMismatchReturns200(t *testing.T) {
	srv, _ := testServer(t)

	w := doRequest(t, srv, http.MethodGet, "/emulatorjs/data/loader.js", nil,
		http.Header{"If-None-Match": []string{`"freeplay-some-old-version"`}})
	if w.Code != http.StatusOK {
		t.Errorf("mismatched If-None-Match: status = %d, want 200", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Errorf("200 response carried no body; expected fresh asset bytes")
	}
}
