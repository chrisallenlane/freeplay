package covers

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // register JPEG decoder
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IGDBFetcher fetches cover art from the IGDB API.
type IGDBFetcher struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	client      *http.Client
}

// NewIGDBFetcher creates an IGDB cover art fetcher.
// apiKey should be in "client_id:client_secret" format.
func NewIGDBFetcher(apiKey string) *IGDBFetcher {
	parts := strings.SplitN(apiKey, ":", 2)
	var clientID, clientSecret string
	if len(parts) == 2 {
		clientID = parts[0]
		clientSecret = parts[1]
	}
	return &IGDBFetcher{
		clientID:     clientID,
		clientSecret: clientSecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (f *IGDBFetcher) getToken() (string, error) {
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

func (f *IGDBFetcher) apiRequest(endpoint, body string) ([]byte, error) {
	for attempt := range 2 {
		token, err := f.getToken()
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequest("POST", "https://api.igdb.com/v4/"+endpoint, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Client-ID", f.clientID)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := f.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("IGDB request failed: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			// Token expired, clear and retry once
			f.mu.Lock()
			f.token = ""
			f.mu.Unlock()
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("IGDB returned %d: %s", resp.StatusCode, string(respBody))
		}

		return io.ReadAll(resp.Body)
	}
	// Unreachable: loop always returns or continues
	return nil, fmt.Errorf("IGDB request failed after retry")
}

// Fetch retrieves a cover art image for the given game name.
// When platformIDs is non-empty, results are filtered to those IGDB platform IDs.
func (f *IGDBFetcher) Fetch(gameName string, _ string, platformIDs []int) (image.Image, error) {
	escaped := strings.ReplaceAll(gameName, `"`, `\"`)
	var query string
	if len(platformIDs) > 0 {
		ids := intsToStrings(platformIDs)
		query = fmt.Sprintf(`search "%s"; fields name, cover; where platforms = (%s); limit 5;`, escaped, strings.Join(ids, ","))
	} else {
		query = fmt.Sprintf(`search "%s"; fields name, cover; limit 5;`, escaped)
	}

	data, err := f.apiRequest("games", query)
	if err != nil {
		return nil, err
	}

	var games []struct {
		Name  string `json:"name"`
		Cover int    `json:"cover"`
	}
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("parsing game search: %w", err)
	}
	if len(games) == 0 {
		return nil, nil // no match
	}

	// Pick best match: prefer case-insensitive exact name match, else first result
	best := -1
	for i, g := range games {
		if g.Cover == 0 {
			continue
		}
		if strings.EqualFold(g.Name, gameName) {
			best = i
			break
		}
		if best == -1 {
			best = i
		}
	}
	if best == -1 {
		return nil, nil // no result with cover art
	}

	// Get cover URL
	coverQuery := fmt.Sprintf(`fields url; where id = %d; limit 1;`, games[best].Cover)
	coverData, err := f.apiRequest("covers", coverQuery)
	if err != nil {
		return nil, err
	}

	var coverResults []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(coverData, &coverResults); err != nil {
		return nil, fmt.Errorf("parsing cover result: %w", err)
	}
	if len(coverResults) == 0 || coverResults[0].URL == "" {
		return nil, nil
	}

	// Transform URL: prepend https, upgrade to t_cover_big
	imgURL := coverResults[0].URL
	if strings.HasPrefix(imgURL, "//") {
		imgURL = "https:" + imgURL
	}
	imgURL = strings.Replace(imgURL, "t_thumb", "t_cover_big", 1)

	// Download image
	resp, err := f.client.Get(imgURL)
	if err != nil {
		return nil, fmt.Errorf("downloading cover image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover image returned %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decoding cover image: %w", err)
	}

	return img, nil
}

func intsToStrings(ids []int) []string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return s
}
