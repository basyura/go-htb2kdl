package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bookmarkfile "htb2kdl/internal/bookmarks"
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

func TestParseArgsLimitAndFile(t *testing.T) {
	opts, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--limit", "5", "--file", "queue.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.limit != 5 {
		t.Fatalf("limit = %d, want 5", opts.limit)
	}
	if opts.file != "queue.yml" {
		t.Fatalf("file = %q, want queue.yml", opts.file)
	}
	if !opts.limitSpecified {
		t.Fatal("limitSpecified = false, want true")
	}
}

func TestParseArgsDetectsExplicitZeroLimit(t *testing.T) {
	opts, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--limit", "0"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.limit != 0 {
		t.Fatalf("limit = %d, want 0", opts.limit)
	}
	if !opts.limitSpecified {
		t.Fatal("limitSpecified = false, want true")
	}
}

func TestParseArgsSend(t *testing.T) {
	opts, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--send"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.send {
		t.Fatal("send = false, want true")
	}
}

func TestApplyBookmarksLimitUsesYAMLWhenLimitIsNotSpecified(t *testing.T) {
	limit := 10
	opts := applyBookmarksLimit(options{}, bookmarkfile.File{
		Settings: bookmarkfile.SettingsConfig{Limit: &limit},
	})

	if opts.limit != 10 {
		t.Fatalf("limit = %d, want 10", opts.limit)
	}
}

func TestApplyBookmarksLimitKeepsExplicitLimit(t *testing.T) {
	limit := 10
	opts := applyBookmarksLimit(options{limit: 5, limitSpecified: true}, bookmarkfile.File{
		Settings: bookmarkfile.SettingsConfig{Limit: &limit},
	})

	if opts.limit != 5 {
		t.Fatalf("limit = %d, want 5", opts.limit)
	}
}

func TestApplyBookmarksLimitKeepsExplicitZeroLimit(t *testing.T) {
	limit := 10
	opts := applyBookmarksLimit(options{limitSpecified: true}, bookmarkfile.File{
		Settings: bookmarkfile.SettingsConfig{Limit: &limit},
	})

	if opts.limit != 0 {
		t.Fatalf("limit = %d, want 0", opts.limit)
	}
}

func TestApplyBookmarksLimitIgnoresMissingYAMLLimit(t *testing.T) {
	opts := applyBookmarksLimit(options{}, bookmarkfile.File{})

	if opts.limit != 0 {
		t.Fatalf("limit = %d, want 0", opts.limit)
	}
}

func TestParseFromDateAcceptsRelativeNegativeDays(t *testing.T) {
	today := time.Date(2026, 5, 22, 15, 30, 0, 0, time.Local)

	got, err := parseFromDate("-2", today)
	if err != nil {
		t.Fatal(err)
	}

	want := time.Date(2026, 5, 20, 0, 0, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("from = %v, want %v", got, want)
	}
}

func TestParseFromDateRejectsInvalidRelativeDays(t *testing.T) {
	_, err := parseFromDate("-0", time.Date(2026, 5, 22, 0, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--from は yyyyMMdd 形式または -n 形式で指定してください") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseArgsRejectsNegativeLimit(t *testing.T) {
	_, err := parseArgs([]string{"--user", "alice", "--from", "20260520", "--limit", "-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--limit は 0 以上で指定してください") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveBookmarksPathPrefersFileOption(t *testing.T) {
	got, err := resolveBookmarksPath(options{file: "custom.yml"}, runConfig{bookmarksPath: "config.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom.yml" {
		t.Fatalf("path = %q, want custom.yml", got)
	}
}

func TestFileExistsReportsMissingFileAsFalse(t *testing.T) {
	got, err := fileExists(filepath.Join(t.TempDir(), "missing.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("fileExists = true, want false")
	}
}

func TestFileExistsReportsExistingFileAsTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.yml")
	if err := os.WriteFile(path, []byte("users: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fileExists(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("fileExists = false, want true")
	}
}

func TestRemoveSentBook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(path, []byte("epub"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeSentBook(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat error = %v, want os.ErrNotExist", err)
	}
}

func TestRemoveSentBookReportsError(t *testing.T) {
	err := removeSentBook(filepath.Join(t.TempDir(), "missing.epub"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "送信済み EPUB ファイルの削除に失敗しました") {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildChaptersIncludesFailureChapter(t *testing.T) {
	bookmarks := []hatena.Bookmark{{URL: "://bad-url"}}
	var stderr strings.Builder

	chapters, err := buildChapters(context.Background(), nil, bookmarks, true, &stderr, nil)
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

	_, err := buildChapters(context.Background(), nil, bookmarks, false, &stderr, nil)
	if !errors.Is(err, errNoChapters) {
		t.Fatalf("err = %v, want errNoChapters", err)
	}
}
