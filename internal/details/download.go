package details

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/chrisallenlane/freeplay/internal/atomicfile"
	"github.com/chrisallenlane/freeplay/internal/igdb"
)

// downloadCoverPair downloads the full-res cover image and its thumbnail
// variant, then rewrites details.CoverURL to the local serving path.
// A failed main-cover download is logged as a warning and clears CoverURL;
// a failed thumbnail download is silently ignored.
func (c *Cache) downloadCoverPair(
	details *igdb.GameDetails,
	cacheDir, urlBase, cleanName string,
) {
	if details.CoverURL == "" {
		return
	}

	_, localURL, err := c.downloadImage(
		details.CoverURL, cacheDir, urlBase, "cover.jpg",
	)
	if err != nil {
		slog.Warn("downloading cover failed", "game", cleanName, "error", err)
		details.CoverURL = ""
		return
	}

	// Derive thumbnail from the original IGDB URL before rewriting.
	// Also download t_cover_big for library grid thumbnails.
	thumbURL := strings.Replace(
		details.CoverURL, "t_original", "t_cover_big", 1,
	)
	_, _, _ = c.downloadImage(thumbURL, cacheDir, urlBase, "cover_thumb.jpg")
	details.CoverURL = localURL
}

// downloadImageSet downloads a batch of images (screenshots or artworks),
// logging warnings for individual failures and returning the local URLs.
func (c *Cache) downloadImageSet(
	urls []string, cacheDir, urlBase, cleanName, prefix string,
) []string {
	var out []string
	for i, u := range urls {
		filename := fmt.Sprintf("%s_%d.jpg", prefix, i)
		_, localURL, err := c.downloadImage(u, cacheDir, urlBase, filename)
		if err != nil {
			slog.Warn(
				"downloading "+prefix+" failed",
				"game", cleanName, "index", i, "error", err,
			)
			continue
		}
		out = append(out, localURL)
	}
	return out
}

// downloadImage fetches a remote URL and saves it to cacheDir/filename.
// Returns the local filesystem path and the URL path for serving.
func (c *Cache) downloadImage(
	rawURL, cacheDir, urlBase, filename string,
) (string, string, error) {
	resp, err := c.client.Get(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("downloading %s: %w", filename, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf(
			"downloading %s: status %d", filename, resp.StatusCode,
		)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", "", fmt.Errorf(
			"downloading %s: unexpected content-type %q", filename, ct,
		)
	}

	localPath := filepath.Join(cacheDir, filename)
	if err := atomicfile.Write(localPath, func(w io.Writer) error {
		_, err := io.Copy(w, io.LimitReader(resp.Body, 20<<20))
		return err
	}); err != nil {
		return "", "", err
	}

	return localPath, urlBase + "/" + url.PathEscape(filename), nil
}

// ensureCoverThumbnail copies the cached cover image to the standard cover
// path (used by the covers handler) if it doesn't already exist.
func (c *Cache) ensureCoverThumbnail(console, nameNoExt, cleanName string) {
	dst := coverPath(c.dataDir, console, nameNoExt)
	if _, err := os.Stat(dst); err == nil {
		return // already exists
	}

	srcPath := filepath.Join(c.cacheDir(console, cleanName), "cover_thumb.jpg")
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return // no cached cover yet
	}

	_ = atomicfile.Write(dst, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
