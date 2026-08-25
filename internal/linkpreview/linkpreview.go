// Package linkpreview fetches a web page and extracts a title and preview image
// from its OpenGraph / Twitter-card metadata (FR13). It is intentionally small:
// a bounded HTTP GET plus regex extraction, no external HTML dependency.
package linkpreview

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Preview holds the metadata pulled from a page.
type Preview struct {
	Title    string
	ImageURL string
}

// Empty reports whether nothing useful was found.
func (p Preview) Empty() bool { return p.Title == "" && p.ImageURL == "" }

const maxBytes = 1 << 20 // 1 MiB is plenty for a document <head>.

var client = &http.Client{Timeout: 10 * time.Second}

// ErrBadURL is returned for input that is not an http(s) URL.
var ErrBadURL = errors.New("URL must start with http:// or https://")

// Fetch retrieves rawURL and extracts its preview metadata. A successful fetch
// with no metadata returns an empty Preview and nil error; network/HTTP failures
// return an error so the caller can offer manual fallback entry (FR13.2).
func Fetch(ctx context.Context, rawURL string) (Preview, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return Preview{}, ErrBadURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Preview{}, err
	}
	req.Header.Set("User-Agent", "BackbonePlate/1.0 (link preview)")
	req.Header.Set("Accept", "text/html")
	resp, err := client.Do(req)
	if err != nil {
		return Preview{}, fmt.Errorf("could not fetch page: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Preview{}, fmt.Errorf("page returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return Preview{}, err
	}
	return Extract(string(body), rawURL), nil
}

var (
	metaRe    = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	attrRe    = regexp.MustCompile(`(?is)([a-z:-]+)\s*=\s*("([^"]*)"|'([^']*)')`)
	titleTag  = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
)

// Extract parses preview metadata out of an HTML document. pageURL is used to
// resolve a relative image URL to an absolute one. Exported for testing.
func Extract(htmlDoc, pageURL string) Preview {
	var p Preview
	for _, tag := range metaRe.FindAllString(htmlDoc, -1) {
		key, content := "", ""
		for _, a := range attrRe.FindAllStringSubmatch(tag, -1) {
			name := strings.ToLower(a[1])
			val := a[3]
			if val == "" {
				val = a[4]
			}
			switch name {
			case "property", "name":
				key = strings.ToLower(val)
			case "content":
				content = val
			}
		}
		if content == "" {
			continue
		}
		content = strings.TrimSpace(html.UnescapeString(content))
		switch key {
		case "og:title", "twitter:title":
			if p.Title == "" {
				p.Title = content
			}
		case "og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src":
			if p.ImageURL == "" {
				p.ImageURL = content
			}
		}
	}
	if p.Title == "" {
		if m := titleTag.FindStringSubmatch(htmlDoc); m != nil {
			p.Title = strings.TrimSpace(html.UnescapeString(m[1]))
		}
	}
	p.ImageURL = absoluteURL(pageURL, p.ImageURL)
	return p
}

// absoluteURL resolves a possibly-relative image URL against the page URL.
func absoluteURL(pageURL, ref string) string {
	if ref == "" {
		return ""
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ref
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}
