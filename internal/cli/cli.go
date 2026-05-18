package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"htb2kdl/internal/book"
	"htb2kdl/internal/content"
	"htb2kdl/internal/convert"
	"htb2kdl/internal/hatena"
)

const dateLayout = "20060102"

type options struct {
	user string
	from time.Time
	out  string
	css  string
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	opts, err := parseArgs(args)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	hatenaClient := hatena.NewClient(client)
	bookmarks, err := hatenaClient.FetchBookmarks(ctx, opts.user, opts.from)
	if err != nil {
		return err
	}
	if len(bookmarks) == 0 {
		return errors.New("対象のブックマークがありません")
	}

	extractor := content.NewExtractor(client)
	converter := convert.NewMarkdownConverter()
	chapters := make([]book.Chapter, 0, len(bookmarks))
	var warnings []error

	for _, bm := range bookmarks {
		article, err := extractor.Extract(ctx, bm.URL)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: %w", bm.URL, err))
			continue
		}

		title := article.Title
		if title == "" {
			title = bm.Title
		}
		if title == "" {
			title = bm.URL
		}

		body := article.HTML
		if body == "" {
			var err error
			body, err = converter.Convert(article.Markdown)
			if err != nil {
				warnings = append(warnings, fmt.Errorf("%s: %w", bm.URL, err))
				continue
			}
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
		return errors.New("EPUB に含められる記事がありません")
	}

	out := opts.out
	if out == "" {
		out = book.DefaultOutputPath(opts.user, opts.from)
	}

	var stylesheet []byte
	if opts.css != "" {
		var err error
		stylesheet, err = os.ReadFile(opts.css)
		if err != nil {
			return fmt.Errorf("CSS ファイルの読み込みに失敗しました: %w", err)
		}
	}

	if err := book.Write(book.Options{
		Title:      fmt.Sprintf("%s のはてなブックマーク", opts.user),
		Author:     opts.user,
		Output:     out,
		Chapters:   chapters,
		Created:    time.Now(),
		Stylesheet: stylesheet,
		Context:    ctx,
		HTTPClient: client,
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "generated: %s\n", out)
	return nil
}

func parseArgs(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("htb2kdl", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.user, "user", "", "Hatena user ID")
	fs.StringVar(&opts.out, "out", "", "output EPUB path")
	fs.StringVar(&opts.css, "css", "", "CSS file path for EPUB styling")
	from := fs.String("from", "", "bookmark date lower bound in yyyyMMdd")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if opts.user == "" {
		return opts, errors.New("--user は必須です")
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
