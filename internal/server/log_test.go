package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// capturingHandler is a minimal slog.Handler that records every log
// record. Mirrors the pattern from internal/config/config_test.go.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

// withAttr returns the string value of the named attribute in r, and
// whether it was found.
func withAttr(r slog.Record, key string) (any, bool) {
	var found any
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value.Any()
			ok = true
			return false // stop iteration
		}
		return true
	})
	return found, ok
}

// installCapturingLogger replaces the default slog logger with one that
// routes all records to h, and restores the original on test cleanup.
func installCapturingLogger(t *testing.T) *capturingHandler {
	t.Helper()
	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func TestLogRequestsEmitsOneLinePerRequest(t *testing.T) {
	h := installCapturingLogger(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})
	mw := logRequests(inner)

	req := httptest.NewRequest(http.MethodGet, "/foo", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	r := recs[0]
	if r.Message != "http" {
		t.Errorf("message = %q, want %q", r.Message, "http")
	}
	if r.Level != slog.LevelInfo {
		t.Errorf("level = %v, want Info", r.Level)
	}

	checkStr := func(key, want string) {
		t.Helper()
		val, ok := withAttr(r, key)
		if !ok {
			t.Errorf("attribute %q missing", key)
			return
		}
		if got, ok2 := val.(string); !ok2 || got != want {
			t.Errorf("attr %q = %v, want %q", key, val, want)
		}
	}
	checkStr("method", http.MethodGet)
	checkStr("path", "/foo")

	checkInt := func(key string, min, max int64) {
		t.Helper()
		val, ok := withAttr(r, key)
		if !ok {
			t.Errorf("attribute %q missing", key)
			return
		}
		var n int64
		switch v := val.(type) {
		case int64:
			n = v
		case int:
			n = int64(v)
		default:
			t.Errorf("attr %q has unexpected type %T = %v", key, val, val)
			return
		}
		if n < min || n > max {
			t.Errorf("attr %q = %d, want [%d, %d]", key, n, min, max)
		}
	}
	checkInt("status", 200, 200)
	checkInt("bytes", 5, 5) // len("hello")
	checkInt("ms", 0, 10000)
}

func TestLogRequestsCapturesNon2xxStatus(t *testing.T) {
	h := installCapturingLogger(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	})
	mw := logRequests(inner)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	val, ok := withAttr(recs[0], "status")
	if !ok {
		t.Fatal("status attribute missing")
	}
	var status int64
	switch v := val.(type) {
	case int64:
		status = v
	case int:
		status = int64(v)
	default:
		t.Fatalf("status has unexpected type %T", val)
	}
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestLogRequestsDefaultStatusIs200WhenHandlerNeverCallsWriteHeader(
	t *testing.T,
) {
	h := installCapturingLogger(t)

	// Handler that only calls Write, never WriteHeader.
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit 200"))
	})
	mw := logRequests(inner)

	req := httptest.NewRequest(http.MethodGet, "/implicit", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	val, ok := withAttr(recs[0], "status")
	if !ok {
		t.Fatal("status attribute missing")
	}
	var status int64
	switch v := val.(type) {
	case int64:
		status = v
	case int:
		status = int64(v)
	default:
		t.Fatalf("status has unexpected type %T", val)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200 (implicit default)", status)
	}
}

func TestRecoverPanicLogsAndReturns500(t *testing.T) {
	h := installCapturingLogger(t)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic value")
	})
	// Wrap in logRequests so the loggingResponseWriter is present (needed
	// by recoverPanic to detect whether headers were already written).
	mw := logRequests(recoverPanic(inner))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	mw.ServeHTTP(w, req)

	// Expect HTTP 500.
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}

	// Expect exactly one slog.Error record for the panic.
	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	var errorRecs []slog.Record
	for _, r := range recs {
		if r.Level == slog.LevelError {
			errorRecs = append(errorRecs, r)
		}
	}
	if len(errorRecs) != 1 {
		t.Fatalf("got %d Error records, want 1", len(errorRecs))
	}
	rec := errorRecs[0]
	if rec.Message != "http panic" {
		t.Errorf("message = %q, want %q", rec.Message, "http panic")
	}
	panicVal, ok := withAttr(rec, "panic")
	if !ok {
		t.Error("panic attribute missing from error record")
	} else if panicVal != "test panic value" {
		t.Errorf("panic attr = %v, want %q", panicVal, "test panic value")
	}
	if _, ok := withAttr(rec, "stack"); !ok {
		t.Error("stack attribute missing from error record")
	}
}

func TestRecoverPanicAfterHeadersWritten(t *testing.T) {
	h := installCapturingLogger(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("panic after write")
	})
	mw := logRequests(recoverPanic(inner))

	req := httptest.NewRequest(http.MethodGet, "/panic-after-write", nil)
	w := httptest.NewRecorder()

	// Must not panic out of ServeHTTP itself.
	mw.ServeHTTP(w, req)

	// The panic was already-headers-written; we must not have issued a
	// second WriteHeader(500). httptest.ResponseRecorder records the
	// first code sent; that's 200 here.
	if w.Code != http.StatusOK {
		t.Errorf("code = %d, want 200 (first WriteHeader wins)", w.Code)
	}

	// The error must still have been logged.
	h.mu.Lock()
	recs := h.records
	h.mu.Unlock()

	var errorRecs []slog.Record
	for _, r := range recs {
		if r.Level == slog.LevelError {
			errorRecs = append(errorRecs, r)
		}
	}
	if len(errorRecs) != 1 {
		t.Fatalf("got %d Error records, want 1 (panic must still be logged)", len(errorRecs))
	}
}
