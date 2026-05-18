package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	readability "github.com/mackee/go-readability"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
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
		HTML:     normalizeCodeBlocks(strings.TrimSpace(readability.ToHTML(article.Root))),
	}, nil
}

func normalizeCodeBlocks(body string) string {
	root := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	}
	nodes, err := html.ParseFragment(strings.NewReader(body), root)
	if err != nil {
		return body
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	linkParents(root)
	var blocks []*html.Node
	collectCodeBlocks(root, &blocks)
	for _, block := range blocks {
		wrapCodeBlock(block)
	}

	var buf bytes.Buffer
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := html.Render(&buf, node); err != nil {
			return body
		}
	}
	return strings.TrimSpace(buf.String())
}

func linkParents(node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		child.Parent = node
		linkParents(child)
	}
}

func collectCodeBlocks(node *html.Node, blocks *[]*html.Node) {
	if node.Type != html.ElementNode || node.Data != "code" {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collectCodeBlocks(child, blocks)
		}
		return
	}
	if !hasAncestor(node, "pre") && isBlockCode(node) {
		*blocks = append(*blocks, node)
	}
}

func wrapCodeBlock(node *html.Node) {
	parent := node.Parent
	if parent == nil {
		return
	}
	pre := &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Pre,
		Data:     "pre",
	}
	parent.InsertBefore(pre, node)
	parent.RemoveChild(node)
	pre.AppendChild(node)
}

func hasAncestor(node *html.Node, tag string) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.Data == tag {
			return true
		}
	}
	return false
}

func isBlockCode(node *html.Node) bool {
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.TextNode && strings.Contains(strings.TrimSpace(n.Data), "\n") {
			return true
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			return true
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(node)
}
