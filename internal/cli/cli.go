package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
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

// options holds validated command-line options used by a single CLI run.
type options struct {
	user           string
	from           time.Time
	out            string
	css            string
	file           string
	debugURL       string
	limit          int
	limitSpecified bool
	send           bool
}

// runConfig holds injectable runtime settings that are not parsed from flags.
type runConfig struct {
	defaultStylesheet []byte
	bookmarksPath     string
	logPath           string
}

// RunOption customizes runConfig before the CLI starts processing.
type RunOption func(*runConfig)

// WithDefaultStylesheet sets the stylesheet bytes used when no CSS path is
// provided by command-line options.
func WithDefaultStylesheet(stylesheet []byte) RunOption {
	return func(cfg *runConfig) {
		cfg.defaultStylesheet = stylesheet
	}
}

// Run parses CLI arguments, selects immediate or queued execution, and writes
// progress and result messages to the provided writers.
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
	if opts.debugURL != "" {
		logger.Printf("実行を開始しました url=%s", opts.debugURL)
		return runDebugURL(ctx, opts, cfg, stdout, stderr, logger)
	}
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

// runDebugURL generates an EPUB from a single URL without fetching Hatena
// Bookmark RSS or touching the queued bookmarks file.
func runDebugURL(ctx context.Context, opts options, cfg runConfig, stdout, stderr io.Writer, logger *runtimeLogger) error {
	client := &http.Client{Timeout: 30 * time.Second}
	targets := []hatena.Bookmark{{URL: opts.debugURL}}
	chapters, err := buildChapters(ctx, client, targets, true, stderr, logger)
	if err != nil {
		return err
	}
	_, err = writeBook(ctx, client, opts, cfg, chapters, stdout, logger)
	return err
}

// fileExists reports whether path exists while treating unexpected stat errors
// as user-facing failures.
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

// applyBookmarksLimit applies the queue file's default limit when the user did
// not specify --limit explicitly.
func applyBookmarksLimit(opts options, queue bookmarkfile.File) options {
	if opts.limitSpecified {
		return opts
	}
	if queue.Settings.Limit != nil && *queue.Settings.Limit > 0 {
		opts.limit = *queue.Settings.Limit
	}
	return opts
}

// runImmediate fetches the current bookmark range and generates an EPUB without
// using the queued bookmarks file.
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

// runQueued loads the queued bookmarks file and delegates queued EPUB
// generation to runQueuedWithQueue.
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

// runQueuedWithQueue adds fetched bookmarks to the queue and generates an EPUB
// once enough queued URLs are available.
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

// buildChapters extracts article content for each bookmark and converts it into
// EPUB chapters, optionally preserving failures as chapters.
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

// failedChapter builds a placeholder chapter that records the URL and extraction
// error for an article that could not be fetched.
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

// writeBook writes the EPUB file and returns the generated output path.
func writeBook(ctx context.Context, client *http.Client, opts options, cfg runConfig, chapters []book.Chapter, stdout io.Writer, logger *runtimeLogger) (string, error) {
	created := time.Now()
	out := opts.out
	if out == "" {
		out = book.DefaultOutputPath(bookAuthor(opts), created)
	}

	stylesheet, err := loadStylesheet(opts.css, cfg.defaultStylesheet)
	if err != nil {
		return "", err
	}

	logger.Printf("EPUB を生成します output=%s chapters=%d", out, len(chapters))
	if err := book.Write(book.Options{
		Title:      bookTitle(opts),
		Author:     bookAuthor(opts),
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

// bookTitle returns the EPUB title for the current run mode.
func bookTitle(opts options) string {
	if opts.debugURL != "" {
		return "デバッグ用 EPUB"
	}
	return fmt.Sprintf("%s のはてなブックマーク", opts.user)
}

// bookAuthor returns the EPUB author for the current run mode.
func bookAuthor(opts options) string {
	if opts.debugURL != "" {
		return "htb2kdl debug"
	}
	return opts.user
}

// loadMailConfig reads mail settings from the resolved bookmarks file.
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

// sendBook sends the generated EPUB by mail and removes the local file after a
// successful send.
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

// removeSentBook deletes an EPUB file that has already been sent.
func removeSentBook(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("送信済み EPUB ファイルの削除に失敗しました: %w", err)
	}
	return nil
}

// resolveBookmarksPath chooses the bookmarks.yml path from CLI options,
// injected configuration, or the executable directory.
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

// sortBookmarksOldestFirst orders bookmarks in-place from oldest to newest,
// keeping entries without timestamps at the end.
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

// loadStylesheet returns the embedded stylesheet unless a CSS path is provided,
// in which case it reads and returns that file.
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

// parseArgs parses command-line flags and validates required options.
func parseArgs(args []string) (options, error) {
	var opts options
	flagArgs, positionals, err := splitFlagArgs(args)
	if err != nil {
		return opts, err
	}
	if len(positionals) > 1 {
		return opts, errors.New("URL は 1 件だけ指定してください")
	}
	if len(positionals) == 1 {
		if err := validateDebugURL(positionals[0]); err != nil {
			return opts, err
		}
		opts.debugURL = positionals[0]
	}

	fs := flag.NewFlagSet("htb2kdl", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.user, "user", "", "Hatena user ID")
	fs.StringVar(&opts.out, "out", "", "output EPUB path")
	fs.StringVar(&opts.css, "css", "", "CSS file path for EPUB styling")
	fs.StringVar(&opts.file, "file", "", "bookmarks.yml path for queued mode")
	fs.IntVar(&opts.limit, "limit", 0, "number of queued bookmarks required to generate EPUB")
	fs.BoolVar(&opts.send, "send", false, "send generated EPUB by Gmail SMTP")
	from := fs.String("from", "", "bookmark date lower bound in yyyyMMdd")

	if err := fs.Parse(flagArgs); err != nil {
		return opts, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			opts.limitSpecified = true
		}
	})
	if opts.debugURL != "" {
		if opts.limitSpecified && opts.limit > 0 {
			return opts, errors.New("URL 指定時は --limit を指定できません")
		}
		if opts.file != "" {
			return opts, errors.New("URL 指定時は --file を指定できません")
		}
		if opts.send {
			return opts, errors.New("URL 指定時は --send を指定できません")
		}
		return opts, nil
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

	parsed, err := parseFromDate(*from, time.Now())
	if err != nil {
		return opts, err
	}
	opts.from = parsed
	return opts, nil
}

// splitFlagArgs separates one positional URL from flags while allowing the URL
// to appear before or after supported flags.
func splitFlagArgs(args []string) ([]string, []string, error) {
	valueFlags := map[string]struct{}{
		"user":  {},
		"out":   {},
		"css":   {},
		"file":  {},
		"limit": {},
		"from":  {},
	}
	boolFlags := map[string]struct{}{
		"send": {},
	}

	var flagArgs []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name, hasValue := flagName(arg)
		if _, ok := boolFlags[name]; ok {
			flagArgs = append(flagArgs, arg)
			continue
		}
		if _, ok := valueFlags[name]; ok {
			flagArgs = append(flagArgs, arg)
			if !hasValue {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("flag needs an argument: -%s", name)
				}
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		flagArgs = append(flagArgs, arg)
	}
	return flagArgs, positionals, nil
}

// flagName returns a normalized flag name and whether the value is included in
// the same argument.
func flagName(arg string) (string, bool) {
	name := strings.TrimLeft(arg, "-")
	if idx := strings.Index(name, "="); idx >= 0 {
		return name[:idx], true
	}
	return name, false
}

// validateDebugURL checks whether targetURL is an absolute HTTP(S) URL.
func validateDebugURL(targetURL string) error {
	parsed, err := url.ParseRequestURI(targetURL)
	if err != nil {
		return fmt.Errorf("URL が不正です: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL は http または https で指定してください")
	}
	if parsed.Host == "" {
		return errors.New("URL はホスト名を含めて指定してください")
	}
	return nil
}

// parseFromDate parses --from as either yyyyMMdd or a negative relative day
// offset from today in the local timezone.
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
