package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"htb2kdl/internal/book"
	bookmarkfile "htb2kdl/internal/bookmarks"
	"htb2kdl/internal/content"
	"htb2kdl/internal/convert"
	"htb2kdl/internal/hatena"
)

const dateLayout = "20060102"

var errNoChapters = errors.New("EPUB に含められる記事がありません")

type options struct {
	user  string
	from  time.Time
	out   string
	css   string
	limit int
}

type runConfig struct {
	defaultStylesheet []byte
	bookmarksPath     string
}

type RunOption func(*runConfig)

func WithDefaultStylesheet(stylesheet []byte) RunOption {
	return func(cfg *runConfig) {
		cfg.defaultStylesheet = stylesheet
	}
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, runOptions ...RunOption) error {
	var cfg runConfig
	for _, option := range runOptions {
		option(&cfg)
	}

	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	if opts.limit > 0 {
		return runQueued(ctx, opts, cfg, stdout, stderr)
	}
	return runImmediate(ctx, opts, cfg, stdout, stderr)
}

func runImmediate(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer) error {
	client := &http.Client{Timeout: 30 * time.Second}
	hatenaClient := hatena.NewClient(client)
	bookmarks, err := hatenaClient.FetchBookmarks(ctx, opts.user, opts.from)
	if err != nil {
		return err
	}
	if len(bookmarks) == 0 {
		return errors.New("対象のブックマークがありません")
	}
	sortBookmarksOldestFirst(bookmarks)

	chapters, err := buildChapters(ctx, client, bookmarks, false, stderr)
	if err != nil {
		return err
	}
	return writeBook(ctx, client, opts, cfg, chapters, stdout)
}

func runQueued(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer) error {
	bookmarksPath, err := resolveBookmarksPath(cfg)
	if err != nil {
		return err
	}

	queue, err := bookmarkfile.Load(bookmarksPath)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	hatenaClient := hatena.NewClient(client)
	fetched, err := hatenaClient.FetchBookmarks(ctx, opts.user, opts.from)
	if err != nil {
		return err
	}
	sortBookmarksOldestFirst(fetched)

	urls := make([]string, 0, len(fetched))
	for _, bm := range fetched {
		urls = append(urls, bm.URL)
	}
	queue.Add(opts.user, urls)

	queued := queue.Queued(opts.user)
	if len(queued) < opts.limit {
		if err := bookmarkfile.SaveAtomic(bookmarksPath, queue); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "queued: %d/%d\n", len(queued), opts.limit)
		return nil
	}

	targetURLs := queued[:opts.limit]
	targets := make([]hatena.Bookmark, 0, len(targetURLs))
	for _, url := range targetURLs {
		targets = append(targets, hatena.Bookmark{URL: url})
	}

	chapters, err := buildChapters(ctx, client, targets, true, stderr)
	if err != nil {
		return err
	}
	if err := writeBook(ctx, client, opts, cfg, chapters, stdout); err != nil {
		return err
	}

	queue.CompleteFirst(opts.user, opts.limit)
	return bookmarkfile.SaveAtomic(bookmarksPath, queue)
}

func buildChapters(ctx context.Context, client *http.Client, bookmarks []hatena.Bookmark, includeFailureChapters bool, stderr io.Writer) ([]book.Chapter, error) {
	extractor := content.NewExtractor(client)
	converter := convert.NewMarkdownConverter()
	chapters := make([]book.Chapter, 0, len(bookmarks))
	var warnings []error

	for _, bm := range bookmarks {
		article, err := extractor.Extract(ctx, bm.URL)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", bm.URL, err))
			if includeFailureChapters {
				chapters = append(chapters, failedChapter(bm.URL, err))
			}
			continue
		}

		title := article.Title
		if title == "" {
			title = bm.Title
		}
		if title == "" {
			title = bm.URL
		}

		body, err := converter.Convert(article.Markdown)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", bm.URL, err))
			if includeFailureChapters {
				chapters = append(chapters, failedChapter(bm.URL, err))
			}
			continue
		}

		chapters = append(chapters, book.Chapter{
			Title:        title,
			URL:          bm.URL,
			BookmarkedAt: bm.BookmarkedAt,
			HTMLBody:     body,
		})
	}

	for _, warning := range warnings {
		fmt.Fprintf(stderr, "warning: %v\n", warning)
	}
	if len(chapters) == 0 {
		return nil, errNoChapters
	}
	return chapters, nil
}

func failedChapter(targetURL string, err error) book.Chapter {
	return book.Chapter{
		Title: "記事を取得できませんでした",
		URL:   targetURL,
		HTMLBody: fmt.Sprintf(
			"<p>記事本文を取得できませんでした。</p><h2>URL</h2><p><a href=\"%s\">%s</a></p><h2>エラー</h2><p>%s</p>",
			html.EscapeString(targetURL),
			html.EscapeString(targetURL),
			html.EscapeString(err.Error()),
		),
	}
}

func writeBook(ctx context.Context, client *http.Client, opts options, cfg runConfig, chapters []book.Chapter, stdout io.Writer) error {
	created := time.Now()
	out := opts.out
	if out == "" {
		out = book.DefaultOutputPath(opts.user, created)
	}

	stylesheet, err := loadStylesheet(opts.css, cfg.defaultStylesheet)
	if err != nil {
		return err
	}

	if err := book.Write(book.Options{
		Title:      fmt.Sprintf("%s のはてなブックマーク", opts.user),
		Author:     opts.user,
		Output:     out,
		Chapters:   chapters,
		Created:    created,
		Stylesheet: stylesheet,
		Context:    ctx,
		HTTPClient: client,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "generated: %s\n", out)
	return nil
}

func resolveBookmarksPath(cfg runConfig) (string, error) {
	if cfg.bookmarksPath != "" {
		return cfg.bookmarksPath, nil
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("実行ファイルの場所を取得できませんでした: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), "bookmarks.yml"), nil
}

func sortBookmarksOldestFirst(bookmarks []hatena.Bookmark) {
	sort.SliceStable(bookmarks, func(i, j int) bool {
		left := bookmarks[i].BookmarkedAt
		right := bookmarks[j].BookmarkedAt
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})
}

func loadStylesheet(cssPath string, defaultStylesheet []byte) ([]byte, error) {
	if cssPath == "" {
		return defaultStylesheet, nil
	}
	stylesheet, err := os.ReadFile(cssPath)
	if err != nil {
		return nil, fmt.Errorf("CSS ファイルの読み込みに失敗しました: %w", err)
	}
	return stylesheet, nil
}

func parseArgs(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("htb2kdl", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.user, "user", "", "Hatena user ID")
	fs.StringVar(&opts.out, "out", "", "output EPUB path")
	fs.StringVar(&opts.css, "css", "", "CSS file path for EPUB styling")
	fs.IntVar(&opts.limit, "limit", 0, "number of queued bookmarks required to generate EPUB")
	from := fs.String("from", "", "bookmark date lower bound in yyyyMMdd")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.user == "" {
		return opts, errors.New("--user は必須です")
	}
	if opts.limit < 0 {
		return opts, errors.New("--limit は 0 以上で指定してください")
	}
	if *from == "" {
		return opts, errors.New("--from は必須です")
	}

	parsed, err := time.ParseInLocation(dateLayout, *from, time.Local)
	if err != nil {
		return opts, fmt.Errorf("--from は yyyyMMdd 形式で指定してください: %w", err)
	}
	opts.from = parsed
	return opts, nil
}
