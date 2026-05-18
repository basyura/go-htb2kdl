package hatena

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRSS(t *testing.T) {
	rss := `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/">
  <item>
    <title>Example</title>
    <link>https://example.com/article</link>
    <dc:date>2026-05-17T10:20:30+09:00</dc:date>
  </item>
</rdf:RDF>`

	got, err := ParseRSS(strings.NewReader(rss))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Title != "Example" {
		t.Fatalf("title = %q", got[0].Title)
	}
	if got[0].URL != "https://example.com/article" {
		t.Fatalf("url = %q", got[0].URL)
	}
	if got[0].BookmarkedAt.IsZero() {
		t.Fatal("BookmarkedAt is zero")
	}
}

func TestFetchBookmarksFiltersFromDate(t *testing.T) {
	from := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alice/rss" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/">
  <item>
    <title>Old</title>
    <link>https://old.example.com</link>
    <dc:date>2026-05-16T23:59:59Z</dc:date>
  </item>
  <item>
    <title>New</title>
    <link>https://new.example.com</link>
    <dc:date>2026-05-17T00:00:00Z</dc:date>
  </item>
</rdf:RDF>`))
	}))
	defer server.Close()

	client := NewClient(server.Client())
	client.baseURL = server.URL
	got, err := client.FetchBookmarks(context.Background(), "alice", from)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].URL != "https://new.example.com" {
		t.Fatalf("url = %q", got[0].URL)
	}
}
