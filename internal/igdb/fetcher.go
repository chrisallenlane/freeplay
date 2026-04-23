// Package igdb manages IGDB API access and name-cleaning utilities.
package igdb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fetcher fetches game metadata from the IGDB API.
type Fetcher struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	client      *http.Client
}

// NewFetcher creates an IGDB game metadata fetcher.
// apiKey should be in "client_id:client_secret" format.
func NewFetcher(apiKey string) *Fetcher {
	parts := strings.SplitN(apiKey, ":", 2)
	var clientID, clientSecret string
	if len(parts) == 2 {
		clientID = parts[0]
		clientSecret = parts[1]
	}
	return &Fetcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *Fetcher) getToken() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.token != "" && time.Now().Before(f.tokenExpiry) {
		return f.token, nil
	}

	vals := url.Values{
		"client_id":     {f.clientID},
		"client_secret": {f.clientSecret},
		"grant_type":    {"client_credentials"},
	}

	resp, err := f.client.PostForm("https://id.twitch.tv/oauth2/token", vals)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	f.token = result.AccessToken
	f.tokenExpiry = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	return f.token, nil
}

func (f *Fetcher) apiRequest(endpoint, body string) ([]byte, error) {
	resp, err := f.doRequest(endpoint, body)
	if err != nil {
		return nil, err
	}

	// On 401, clear the cached token and retry once.
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		f.mu.Lock()
		f.token = ""
		f.mu.Unlock()

		resp, err = f.doRequest(endpoint, body)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("IGDB returned %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	return data, err
}

// doRequest performs a single authenticated POST to the IGDB API.
func (f *Fetcher) doRequest(endpoint, body string) (*http.Response, error) {
	token, err := f.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(
		"POST",
		"https://api.igdb.com/v4/"+endpoint,
		strings.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", f.clientID)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("IGDB request failed: %w", err)
	}
	return resp, nil
}

// SearchGame searches IGDB for a game by name and returns the IGDB game ID
// of the best case-insensitive exact name match. Returns 0 if no match is
// found. When platformIDs is non-empty, results are filtered to those IDs.
func (f *Fetcher) SearchGame(gameName string, platformIDs []int) (int, error) {
	escaped := strings.ReplaceAll(gameName, `"`, `\"`)
	var query string
	if len(platformIDs) > 0 {
		ids := intsToStrings(platformIDs)
		query = fmt.Sprintf(
			`search "%s"; fields name; where platforms = (%s); limit 5;`,
			escaped,
			strings.Join(ids, ","),
		)
	} else {
		query = fmt.Sprintf(`search "%s"; fields name; limit 5;`, escaped)
	}

	data, err := f.apiRequest("games", query)
	if err != nil {
		return 0, err
	}

	var games []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &games); err != nil {
		return 0, fmt.Errorf("parsing game search: %w", err)
	}

	// Prefer exact case-insensitive match.
	for _, g := range games {
		if strings.EqualFold(g.Name, gameName) {
			return g.ID, nil
		}
	}

	// Try diacritics-insensitive match (e.g. "Déjà Vu" == "Deja Vu").
	normalizedSearch := stripDiacritics(gameName)
	for _, g := range games {
		if strings.EqualFold(stripDiacritics(g.Name), normalizedSearch) {
			return g.ID, nil
		}
	}

	// When platform-constrained, fall back to the first result. IGDB's
	// relevance ranking combined with the platform filter is usually
	// correct, and this covers cases where the IGDB title differs more
	// substantially from the ROM filename.
	if len(platformIDs) > 0 && len(games) > 0 {
		return games[0].ID, nil
	}

	return 0, nil
}

// FetchDetailsByID retrieves full game metadata from IGDB by game ID.
// Returns nil if the game is not found.
func (f *Fetcher) FetchDetailsByID(gameID int) (*GameDetails, error) {
	fields := `name, url, summary, storyline, first_release_date, cover.url, ` +
		`involved_companies.company.name, involved_companies.developer, ` +
		`involved_companies.publisher, platforms.name, screenshots.url, ` +
		`artworks.url, collection.name`
	query := fmt.Sprintf(`fields %s; where id = %d;`, fields, gameID)

	data, err := f.apiRequest("games", query)
	if err != nil {
		return nil, err
	}

	var games []igdbGame
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("parsing game details: %w", err)
	}
	if len(games) == 0 {
		return nil, nil
	}

	return gameDetailsFromIGDB(games[0]), nil
}

// stripDiacritics removes diacritical marks from a string using Unicode
// NFD decomposition. For example, "Déjà Vu" becomes "Deja Vu".
func stripDiacritics(s string) string {
	result, _, _ := transform.String(
		transform.Chain(
			norm.NFD,
			runes.Remove(runes.In(unicode.Mn)),
			norm.NFC,
		),
		s,
	)
	return result
}

func intsToStrings(ids []int) []string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return s
}
