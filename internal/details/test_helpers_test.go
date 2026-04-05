package details

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func startFakeImageServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fakeimage"))
		}),
	)
	t.Cleanup(srv.Close)
	return srv
}
