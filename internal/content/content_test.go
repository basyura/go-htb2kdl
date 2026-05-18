package content

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractorExtract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head><title>Ignored</title></head>
<body>
<nav>navigation</nav>
<article>
  <h1>Readable Title</h1>
  <p>This is the main content. This paragraph is intentionally long enough to
  satisfy the readability threshold and should be extracted as Markdown.</p>
  <p>Another meaningful paragraph for the generated article content.</p>
</article>
</body>
</html>`))
	}))
	defer server.Close()

	extractor := NewExtractor(server.Client())
	got, err := extractor.Extract(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got.Markdown, "main content") {
		t.Fatalf("markdown = %q", got.Markdown)
	}
}
