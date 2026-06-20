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
	"strconv"
	"strings"
	"time"

	"htb2kdl/internal/book"
	bookmarkfile "htb2kdl/internal/bookmarks"
	"htb2kdl/internal/content"
	"htb2kdl/internal/convert"
	"htb2kdl/internal/hatena"
	"htb2kdl/internal/mail"
)

const dateLayout = "20060102"

var errNoChapters = errors.New("EPUB に含められる記事がありません")

type options struct {
	user           string
	from           time.Time
	out            string
	css            string
	file           string
	limit          int
	limitSpecified bool
	send           bool
}

type runConfig struct {
	defaultStylesheet []byte
	bookmarksPath     string
	logPath           string
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

	logPath := cfg.logPath
	if logPath == "" {
		logPath, err = defaultLogPath()
		if err != nil {
			return err
		}
	}
	logger, closer, err := newRuntimeLogger(stdout, logPath)
	if err != nil {
		return err
	}
	defer closer.Close()
	logger.PrintStartBanner(time.Now())
	logger.Printf("実行を開始しました user=%s from=%s", opts.user, opts.from.Format(dateLayout))

	if opts.limitSpecified {
		if opts.limit > 0 {
			return runQueued(ctx, opts, cfg, stdout, stderr, logger)
		}
		return runImmediate(ctx, opts, cfg, stdout, stderr, logger)
	}

	bookmarksPath, err := resolveBookmarksPath(opts, cfg)
	if err != nil {
		return err
	}
	hasBookmarks, err := fileExists(bookmarksPath)
	if err != nil {
		return err
	}
	if !hasBookmarks {
		return runImmediate(ctx, opts, cfg, stdout, stderr, logger)
	}
	queue, err := bookmarkfile.Load(bookmarksPath)
	if err != nil {
		return err
	}
	opts = applyBookmarksLimit(opts, queue)
	if opts.limit > 0 {
		return runQueuedWithQueue(ctx, opts, cfg, stdout, stderr, logger, bookmarksPath, queue)
	}

	return runImmediate(ctx, opts, cfg, stdout, stderr, logger)
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("bookmarks.yml の確認に失敗しました: %w", err)
}

func applyBookmarksLimit(opts options, queue bookmarkfile.File) options {
	if opts.limitSpecified {
		return opts
	}
	if queue.Settings.Limit != nil && *queue.Settings.Limit > 0 {
		opts.limit = *queue.Settings.Limit
	}
	return opts
}

func runImmediate(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer, logger *runtimeLogger) error {
	client := &http.Client{Timeout: 30 * time.Second}
	hatenaClient := hatena.NewClient(client)
	logger.Printf("はてなブックマーク RSS を取得します")
	bookmarks, err := hatenaClient.FetchBookmarks(ctx, opts.user, opts.from)
	if err != nil {
		return err
	}
	logger.Printf("はてなブックマーク RSS を取得しました count=%d", len(bookmarks))
	if len(bookmarks) == 0 {
		return errors.New("対象のブックマークがありません")
	}
	sortBookmarksOldestFirst(bookmarks)

	chapters, err := buildChapters(ctx, client, bookmarks, false, stderr, logger)
	if err != nil {
		return err
	}
	out, err := writeBook(ctx, client, opts, cfg, chapters, stdout, logger)
	if err != nil {
		return err
	}
	if opts.send {
		mailConfig, err := loadMailConfig(opts, cfg)
		if err != nil {
			return err
		}
		if err := sendBook(ctx, opts, mailConfig, out, stdout, logger); err != nil {
			return err
		}
	}
	return nil
}

func runQueued(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer, logger *runtimeLogger) error {
	bookmarksPath, err := resolveBookmarksPath(opts, cfg)
	if err != nil {
		return err
	}
	logger.Printf("キューファイルを読み込みます path=%s", bookmarksPath)

	queue, err := bookmarkfile.Load(bookmarksPath)
	if err != nil {
		return err
	}
	return runQueuedWithQueue(ctx, opts, cfg, stdout, stderr, logger, bookmarksPath, queue)
}

func runQueuedWithQueue(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer, logger *runtimeLogger, bookmarksPath string, queue bookmarkfile.File) error {
	client := &http.Client{Timeout: 30 * time.Second}
	hatenaClient := hatena.NewClient(client)
	logger.Printf("はてなブックマーク RSS を取得します")
	fetched, err := hatenaClient.FetchBookmarks(ctx, opts.user, opts.from)
	if err != nil {
		return err
	}
	logger.Printf("はてなブックマーク RSS を取得しました count=%d", len(fetched))
	sortBookmarksOldestFirst(fetched)

	urls := make([]string, 0, len(fetched))
	for _, bm := range fetched {
		urls = append(urls, bm.URL)
	}
	queue.Add(opts.user, urls)

	queued := queue.Queued(opts.user)
	logger.Printf("キューに URL を追加しました added=%d queued=%d limit=%d", len(urls), len(queued), opts.limit)
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

	chapters, err := buildChapters(ctx, client, targets, true, stderr, logger)
	if err != nil {
		return err
	}
	out, err := writeBook(ctx, client, opts, cfg, chapters, stdout, logger)
	if err != nil {
		return err
	}
	if opts.send {
		if err := sendBook(ctx, opts, queue.Mail, out, stdout, logger); err != nil {
			return err
		}
	}

	queue.CompleteFirst(opts.user, opts.limit)
	logger.Printf("キューを更新します completed=%d", opts.limit)
	return bookmarkfile.SaveAtomic(bookmarksPath, queue)
}

func buildChapters(ctx context.Context, client *http.Client, bookmarks []hatena.Bookmark, includeFailureChapters bool, stderr io.Writer, logger *runtimeLogger) ([]book.Chapter, error) {
	extractor := content.NewExtractor(client)
	converter := convert.NewMarkdownConverter()
	chapters := make([]book.Chapter, 0, len(bookmarks))
	var warnings []error

	for i, bm := range bookmarks {
		logger.Printf("記事本文を取得します index=%d/%d url=%s", i+1, len(bookmarks), bm.URL)
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
	logger.Printf("記事本文の処理が完了しました chapters=%d warnings=%d", len(chapters), len(warnings))
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

func writeBook(ctx context.Context, client *http.Client, opts options, cfg runConfig, chapters []book.Chapter, stdout io.Writer, logger *runtimeLogger) (string, error) {
	created := time.Now()
	out := opts.out
	if out == "" {
		out = book.DefaultOutputPath(opts.user, created)
	}

	stylesheet, err := loadStylesheet(opts.css, cfg.defaultStylesheet)
	if err != nil {
		return "", err
	}

	logger.Printf("EPUB を生成します output=%s chapters=%d", out, len(chapters))
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
		return "", err
	}

	fmt.Fprintf(stdout, "generated: %s\n", out)
	return out, nil
}

func loadMailConfig(opts options, cfg runConfig) (bookmarkfile.MailConfig, error) {
	bookmarksPath, err := resolveBookmarksPath(opts, cfg)
	if err != nil {
		return bookmarkfile.MailConfig{}, err
	}
	file, err := bookmarkfile.Load(bookmarksPath)
	if err != nil {
		return bookmarkfile.MailConfig{}, err
	}
	return file.Mail, nil
}

func sendBook(ctx context.Context, opts options, cfg bookmarkfile.MailConfig, out string, stdout io.Writer, logger *runtimeLogger) error {
	mailConfig := mail.Config{
		From:        cfg.From,
		To:          cfg.To,
		AppPassword: cfg.AppPassword,
	}
	logger.Printf("EPUB をメール送信します to=%s path=%s", cfg.To, out)
	if err := mail.SendEPUB(ctx, mailConfig, mail.EPUBMessage{
		Subject: fmt.Sprintf("%s のはてなブックマーク", opts.user),
		Body:    "生成した EPUB を送信します。",
		Path:    out,
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "sent: %s\n", cfg.To)
	if err := removeSentBook(out); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed: %s\n", out)
	return nil
}

func removeSentBook(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("送信済み EPUB ファイルの削除に失敗しました: %w", err)
	}
	return nil
}

func resolveBookmarksPath(opts options, cfg runConfig) (string, error) {
	if opts.file != "" {
		return opts.file, nil
	}
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
	fs.StringVar(&opts.file, "file", "", "bookmarks.yml path for queued mode")
	fs.IntVar(&opts.limit, "limit", 0, "number of queued bookmarks required to generate EPUB")
	fs.BoolVar(&opts.send, "send", false, "send generated EPUB by Gmail SMTP")
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
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			opts.limitSpecified = true
		}
	})
	if *from == "" {
		return opts, errors.New("--from は必須です")
	}

	parsed, err := parseFromDate(*from, time.Now())
	if err != nil {
		return opts, err
	}
	opts.from = parsed
	return opts, nil
}

func parseFromDate(value string, today time.Time) (time.Time, error) {
	if strings.HasPrefix(value, "-") {
		days, err := strconv.Atoi(value)
		if err != nil || days >= 0 {
			return time.Time{}, errors.New("--from は yyyyMMdd 形式または -n 形式で指定してください")
		}
		localToday := today.In(time.Local)
		year, month, day := localToday.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, time.Local).AddDate(0, 0, days), nil
	}

	parsed, err := time.ParseInLocation(dateLayout, value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("--from は yyyyMMdd 形式または -n 形式で指定してください: %w", err)
	}
	return parsed, nil
}
