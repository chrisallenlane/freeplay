package covers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// rewriteTransport redirects all HTTP requests to a test server URL.
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	// Extract just the host:port from target
	req.URL.Host = strings.TrimPrefix(t.target, "http://")
	return t.base.RoundTrip(req)
}

// writeTokenResponse writes the standard test OAuth2 token response.
func writeTokenResponse(w http.ResponseWriter) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "test-token",
		"expires_in":   3600,
	})
}

func FuzzNewIGDBFetcher(f *testing.F) {
	f.Add("client_id:client_secret")
	f.Add("a:b")
	f.Add(":")
	f.Add(":secret")
	f.Add("id:")
	f.Add("")
	f.Add("nocolon")
	f.Add("a:b:c")

	f.Fuzz(func(t *testing.T, apiKey string) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewIGDBFetcher panicked on input %q: %v", apiKey, r)
			}
		}()
		NewIGDBFetcher(apiKey)
	})
}

func TestIntsToStrings(t *testing.T) {
	tests := []struct {
		input []int
		want  []string
	}{
		{[]int{18}, []string{"18"}},
		{[]int{18, 99}, []string{"18", "99"}},
		{[]int{}, []string{}},
	}
	for _, tt := range tests {
		got := intsToStrings(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf(
				"intsToStrings(%v) = %v, want %v",
				tt.input, got, tt.want,
			)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf(
					"intsToStrings(%v)[%d] = %q, want %q",
					tt.input, i, got[i], tt.want[i],
				)
			}
		}
	}
}

func TestSearchGameExactMatch(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 42, "name": "Mega Man X"},
		{"id": 17, "name": "Mega Man"},
		{"id": 99, "name": "Mega Man 2"},
	})
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				writeTokenResponse(w)
			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				_, _ = w.Write(searchResp)
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}

	id, err := f.SearchGame("Mega Man", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 17 {
		t.Errorf("SearchGame() = %d, want 17 (exact match)", id)
	}
}

func TestSearchGameNoMatch(t *testing.T) {
	searchResp, _ := json.Marshal([]map[string]any{
		{"id": 42, "name": "Mega Man X"},
	})
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				writeTokenResponse(w)
			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				_, _ = w.Write(searchResp)
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}

	id, err := f.SearchGame("Mega Man", nil)
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if id != 0 {
		t.Errorf("SearchGame() = %d, want 0 (no exact match)", id)
	}
}

func TestSearchGameWithPlatformFilter(t *testing.T) {
	var capturedQuery string
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				writeTokenResponse(w)
			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				body := make([]byte, r.ContentLength)
				_, _ = r.Body.Read(body)
				capturedQuery = string(body)
				resp, _ := json.Marshal([]map[string]any{
					{"id": 18, "name": "Metroid"},
				})
				_, _ = w.Write(resp)
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}

	_, err := f.SearchGame("Metroid", []int{18, 99})
	if err != nil {
		t.Fatalf("SearchGame returned error: %v", err)
	}
	if !strings.Contains(capturedQuery, "where platforms = (18,99)") {
		t.Errorf("query missing platform filter: %s", capturedQuery)
	}
}

func TestFetchDetailsByID(t *testing.T) {
	detailsResp, _ := json.Marshal([]map[string]any{
		{
			"id":                 17,
			"name":               "Mega Man",
			"url":                "https://www.igdb.com/games/mega-man",
			"summary":            "A platformer.",
			"first_release_date": int64(565_920_000),
			"cover":              map[string]any{"url": "//images.igdb.com/t_thumb/abc.jpg"},
			"platforms":          []map[string]any{{"name": "NES"}},
			"involved_companies": []map[string]any{
				{
					"company":   map[string]any{"name": "Capcom"},
					"developer": true,
					"publisher": true,
				},
			},
		},
	})

	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				writeTokenResponse(w)
			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				_, _ = w.Write(detailsResp)
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}

	details, err := f.FetchDetailsByID(17)
	if err != nil {
		t.Fatalf("FetchDetailsByID returned error: %v", err)
	}
	if details == nil {
		t.Fatal("expected non-nil details")
	}
	if details.Name != "Mega Man" {
		t.Errorf("Name = %q, want %q", details.Name, "Mega Man")
	}
	if details.Summary != "A platformer." {
		t.Errorf("Summary = %q, want %q", details.Summary, "A platformer.")
	}
	if len(details.Platforms) != 1 || details.Platforms[0] != "NES" {
		t.Errorf("Platforms = %v, want [NES]", details.Platforms)
	}
	if len(details.Developers) != 1 || details.Developers[0] != "Capcom" {
		t.Errorf("Developers = %v, want [Capcom]", details.Developers)
	}
	if !strings.Contains(details.CoverURL, "t_original") {
		t.Errorf("CoverURL should use t_original, got %q", details.CoverURL)
	}
}

func TestFetchDetailsByIDNotFound(t *testing.T) {
	ts := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/oauth2/token"):
				writeTokenResponse(w)
			case strings.HasSuffix(r.URL.Path, "/v4/games"):
				_, _ = w.Write([]byte("[]"))
			}
		}),
	)
	t.Cleanup(ts.Close)

	f := NewIGDBFetcher("test-id:test-secret")
	f.client = &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: ts.URL,
		},
	}

	details, err := f.FetchDetailsByID(999)
	if err != nil {
		t.Fatalf("FetchDetailsByID returned error: %v", err)
	}
	if details != nil {
		t.Errorf("expected nil details for empty response, got %+v", details)
	}
}
