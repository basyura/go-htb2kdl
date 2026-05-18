package content

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	readability "github.com/mackee/go-readability"
)

type Article struct {
	Title    string
	Markdown string
	HTML     string
}

type Extractor struct {
	httpClient *http.Client
}

func NewExtractor(httpClient *http.Client) *Extractor {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Extractor{httpClient: httpClient}
}

func (e *Extractor) Extract(ctx context.Context, targetURL string) (Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return Article{}, err
	}
	req.Header.Set("User-Agent", "htb2kdl/0.1")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return Article{}, fmt.Errorf("ページの取得に失敗しました: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Article{}, fmt.Errorf("ページの取得に失敗しました: status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Article{}, fmt.Errorf("ページ本文の読み込みに失敗しました: %w", err)
	}

	options := readability.DefaultOptions()
	options.CharThreshold = 1
	doc, err := readability.ParseHTML(string(body), targetURL)
	if err != nil {
		return Article{}, fmt.Errorf("HTML の解析に失敗しました: %w", err)
	}
	readability.PreprocessDocument(doc)
	article := readability.ExtractContent(doc, options)
	if article.Root == nil {
		return Article{}, errors.New("メインコンテンツを抽出できませんでした")
	}

	markdown := strings.TrimSpace(readability.ToMarkdown(article.Root))
	if markdown == "" {
		return Article{}, errors.New("メインコンテンツが空です")
	}

	return Article{
		Title:    strings.TrimSpace(article.Title),
		Markdown: markdown,
		HTML:     strings.TrimSpace(readability.ToHTML(article.Root)),
	}, nil
}
