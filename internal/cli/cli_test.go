package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"htb2kdl/internal/hatena"
)

func TestSortBookmarksOldestFirst(t *testing.T) {
	oldest := time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	middle := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	bookmarks := []hatena.Bookmark{
		{Title: "newest", BookmarkedAt: newest},
		{Title: "unknown 1"},
		{Title: "oldest", BookmarkedAt: oldest},
		{Title: "unknown 2"},
		{Title: "middle", BookmarkedAt: middle},
	}

	sortBookmarksOldestFirst(bookmarks)

	got := make([]string, 0, len(bookmarks))
	for _, bm := range bookmarks {
		got = append(got, bm.Title)
	}
	want := []string{"oldest", "middle", "newest", "unknown 1", "unknown 2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bookmark order = %v, want %v", got, want)
		}
	}
}

func TestLoadStylesheetUsesDefaultWhenCSSPathIsEmpty(t *testing.T) {
	stylesheet, err := loadStylesheet("", []byte("default css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stylesheet) != "default css" {
		t.Fatalf("stylesheet = %q", stylesheet)
	}
}

func TestLoadStylesheetPrefersCSSPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "style.css")
	if err := os.WriteFile(path, []byte("custom css"), 0o644); err != nil {
		t.Fatal(err)
	}

	stylesheet, err := loadStylesheet(path, []byte("default css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stylesheet) != "custom css" {
		t.Fatalf("stylesheet = %q", stylesheet)
	}
}

func TestLoadStylesheetReportsCSSReadError(t *testing.T) {
	_, err := loadStylesheet(filepath.Join(t.TempDir(), "missing.css"), []byte("default css"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "CSS ファイルの読み込みに失敗しました") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseArgsThreshold(t *testing.T) {
	opts, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--limit", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.limit != 5 {
		t.Fatalf("limit = %d, want 5", opts.limit)
	}
}

func TestParseArgsRejectsNegativeThreshold(t *testing.T) {
	_, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--limit", "-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--limit は 0 以上で指定してください") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildChaptersIncludesFailureChapter(t *testing.T) {
	bookmarks := []hatena.Bookmark{{URL: "://bad-url"}}
	var stderr strings.Builder

	chapters, err := buildChapters(context.Background(), nil, bookmarks, true, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if len(chapters) != 1 {
		t.Fatalf("len(chapters) = %d, want 1", len(chapters))
	}
	if chapters[0].Title != "記事を取得できませんでした" {
		t.Fatalf("Title = %q", chapters[0].Title)
	}
	if !strings.Contains(chapters[0].HTMLBody, "記事本文を取得できませんでした。") ||
		!strings.Contains(chapters[0].HTMLBody, "://bad-url") {
		t.Fatalf("HTMLBody = %s", chapters[0].HTMLBody)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Fatalf("stderr = %q, want warning", stderr.String())
	}
}

func TestBuildChaptersWithoutFailureChapterReportsEmpty(t *testing.T) {
	bookmarks := []hatena.Bookmark{{URL: "://bad-url"}}
	var stderr strings.Builder

	_, err := buildChapters(context.Background(), nil, bookmarks, false, &stderr)
	if !errors.Is(err, errNoChapters) {
		t.Fatalf("err = %v, want errNoChapters", err)
	}
}
