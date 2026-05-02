package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// loggingResponseWriter wraps http.ResponseWriter to capture the status
// code and number of bytes written for request logging.
type loggingResponseWriter struct {
	http.ResponseWriter
	// status is the HTTP status code sent to the client. Defaults to 200
	// if WriteHeader is never called explicitly.
	status int
	// bytes is the total number of bytes written to the response body.
	bytes int
	// wroteHeader is true once WriteHeader has been called (implicitly
	// via Write or explicitly). Prevents double-WriteHeader on panic
	// recovery.
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	// Increment by the bytes actually written this call. Treat error as
	// 0-bytes-this-call; net/http surfaces the error to the caller.
	w.bytes += n
	return n, err
}

// logRequests is middleware that emits one structured slog.Info line per
// request after the inner handler returns. Fields: method, path, status,
// bytes written, and duration in milliseconds.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}
		next.ServeHTTP(lw, r)
		slog.Info(
			"http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", lw.status,
			"bytes", lw.bytes,
			"ms", time.Since(start).Milliseconds(),
		)
	})
}

// recoverPanic is middleware that recovers from panics in the inner
// handler, logs a structured slog.Error, and returns HTTP 500 to the
// client (unless headers have already been flushed, in which case only
// the log is emitted).
//
// recoverPanic must sit inside logRequests so that logRequests observes
// the 500 status produced by this middleware.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				slog.Error(
					"http panic",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(stack),
				)
				// Only write the 500 if headers haven't been sent yet.
				// If the handler already flushed bytes the connection is
				// unrecoverable; writing again would corrupt or duplicate.
				if lw, ok := w.(*loggingResponseWriter); ok && !lw.wroteHeader {
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}
