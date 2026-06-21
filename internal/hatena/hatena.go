package hatena

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the base URL for Hatena Bookmark.
const DefaultBaseURL = "https://b.hatena.ne.jp"

// Bookmark represents one item from a Hatena Bookmark RSS feed.
type Bookmark struct {
	Title        string
	URL          string
	BookmarkedAt time.Time
}

// Client fetches and filters Hatena Bookmark RSS feeds.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a Hatena Bookmark client using http.DefaultClient when no
// client is provided.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    DefaultBaseURL,
	}
}

// FetchBookmarks fetches a user's RSS feed and returns bookmarks on or after
// from.
func (c *Client) FetchBookmarks(ctx context.Context, user string, from time.Time) ([]Bookmark, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.rssURL(user), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "htb2kdl/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("はてなブックマーク RSS の取得に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("はてなブックマーク RSS の取得に失敗しました: status %s", resp.Status)
	}

	bookmarks, err := ParseRSS(resp.Body)
	if err != nil {
		return nil, err
	}

	filtered := bookmarks[:0]
	for _, bm := range bookmarks {
		if bm.BookmarkedAt.IsZero() || !bm.BookmarkedAt.Before(from) {
			filtered = append(filtered, bm)
		}
	}
	return filtered, nil
}

// rssURL builds the RSS feed URL for a Hatena user.
func (c *Client) rssURL(user string) string {
	base := strings.TrimRight(c.baseURL, "/")
	return base + "/" + url.PathEscape(user) + "/rss"
}

// ParseRSS parses Hatena Bookmark RSS data into bookmarks.
func ParseRSS(r io.Reader) ([]Bookmark, error) {
	decoder := xml.NewDecoder(r)
	var bookmarks []Bookmark

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("RSS の解析に失敗しました: %w", err)
		}

		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "item" {
			continue
		}

		item, err := parseItem(decoder)
		if err != nil {
			return nil, err
		}
		if item.URL == "" {
			continue
		}
		bookmarks = append(bookmarks, item)
	}

	return bookmarks, nil
}

// parseItem parses a single RSS item element.
func parseItem(decoder *xml.Decoder) (Bookmark, error) {
	var bm Bookmark
	for {
		tok, err := decoder.Token()
		if err != nil {
			return bm, fmt.Errorf("RSS item の解析に失敗しました: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			var value string
			if err := decoder.DecodeElement(&value, &t); err != nil {
				return bm, fmt.Errorf("RSS item の解析に失敗しました: %w", err)
			}
			value = strings.TrimSpace(value)
			switch t.Name.Local {
			case "title":
				bm.Title = value
			case "link":
				bm.URL = value
			case "date", "pubDate":
				if parsed, err := parseDate(value); err == nil {
					bm.BookmarkedAt = parsed
				}
			}
		case xml.EndElement:
			if t.Name.Local == "item" {
				return bm, nil
			}
		}
	}
}

// parseDate parses the date formats commonly used in Hatena Bookmark RSS.
func parseDate(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		time.RFC1123Z,
		time.RFC1123,
	}
	var lastErr error
	for _, layout := range layouts {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}
