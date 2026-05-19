package book

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"image"
	"image/color"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/raitucarp/epub"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type Chapter struct {
	Title        string
	URL          string
	BookmarkedAt time.Time
	HTMLBody     string
}

type Options struct {
	Title      string
	Author     string
	Output     string
	Chapters   []Chapter
	Created    time.Time
	Stylesheet []byte
	Context    context.Context
	HTTPClient *http.Client
}

func DefaultOutputPath(user string, from time.Time) string {
	name := fmt.Sprintf("%s-%s.epub", safeName(user), from.Format("20060102"))
	return filepath.Clean(name)
}

func Write(opts Options) error {
	if len(opts.Chapters) == 0 {
		return fmt.Errorf("章がありません")
	}
	if opts.Output == "" {
		return fmt.Errorf("出力先が指定されていません")
	}
	if opts.Created.IsZero() {
		opts.Created = time.Now()
	}

	if dir := filepath.Dir(opts.Output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("出力ディレクトリの作成に失敗しました: %w", err)
		}
	}

	writer := epub.New("htb2kdl:" + opts.Author + ":" + opts.Created.Format("20060102150405"))
	writer.Title(opts.Title)
	writer.Author(opts.Author)
	writer.Languages("ja")
	writer.Date(opts.Created)
	if err := writer.CoverPNG(blankCover()); err != nil {
		return fmt.Errorf("EPUB カバーの作成に失敗しました: %w", err)
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	imageCache := make(map[string]string)

	toc := epub.TOC{
		Title: opts.Title,
		Items: make([]epub.TOC, 0, len(opts.Chapters)),
	}
	for i, chapter := range opts.Chapters {
		filename := fmt.Sprintf("chapter-%03d.xhtml", i+1)
		images, body := embedChapterImages(ctx, httpClient, chapter, i+1, imageCache)
		for _, image := range images {
			writer.AddImage(image.Name, image.Data)
		}
		chapter.HTMLBody = rewriteChapterLinks(body, chapter.URL)
		writer.AddContent(filename, []byte(renderChapter(chapter, len(opts.Stylesheet) > 0)))
		toc.Items = append(toc.Items, epub.TOC{
			Title: chapter.Title,
			Href:  filename,
		})
	}

	if err := writer.TableOfContents("toc", toc); err != nil {
		return fmt.Errorf("EPUB 目次の作成に失敗しました: %w", err)
	}
	if err := writer.Write(opts.Output); err != nil {
		return fmt.Errorf("EPUB の書き込みに失敗しました: %w", err)
	}
	if err := normalizeEPUB(opts.Output, opts.Stylesheet); err != nil {
		return fmt.Errorf("EPUB の正規化に失敗しました: %w", err)
	}
	return nil
}

type zipEntry struct {
	name   string
	method uint16
	data   []byte
}

type chapterImage struct {
	Name string
	Data []byte
}

func embedChapterImages(ctx context.Context, client *http.Client, chapter Chapter, chapterNo int, cache map[string]string) ([]chapterImage, string) {
	if !strings.Contains(chapter.HTMLBody, "<img") {
		return nil, chapter.HTMLBody
	}

	root := &xhtml.Node{
		Type:     xhtml.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(chapter.HTMLBody), root)
	if err != nil {
		return nil, chapter.HTMLBody
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}

	base, _ := url.Parse(chapter.URL)
	var images []chapterImage
	seen := make(map[string]struct{})
	rewriteImages(root, func(src string) string {
		imageURL := resolveImageURL(base, src)
		if imageURL == "" {
			return src
		}
		if name, ok := cache[imageURL]; ok {
			return path.Join("images", name)
		}

		data, contentType, err := downloadImage(ctx, client, imageURL)
		if err != nil {
			return src
		}
		name := imageFileName(chapterNo, imageURL, contentType, data)
		cache[imageURL] = name
		if _, ok := seen[name]; !ok {
			images = append(images, chapterImage{Name: name, Data: data})
			seen[name] = struct{}{}
		}
		return path.Join("images", name)
	})

	var buf bytes.Buffer
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := xhtml.Render(&buf, node); err != nil {
			return images, chapter.HTMLBody
		}
	}
	return images, strings.TrimSpace(buf.String())
}

func rewriteImages(node *xhtml.Node, rewrite func(string) string) {
	if node.Type == xhtml.ElementNode && node.Data == "img" {
		for i := range node.Attr {
			if strings.EqualFold(node.Attr[i].Key, "src") {
				node.Attr[i].Val = rewrite(strings.TrimSpace(node.Attr[i].Val))
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		rewriteImages(child, rewrite)
	}
}

func rewriteChapterLinks(body, baseURL string) string {
	root := &xhtml.Node{
		Type:     xhtml.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(body), root)
	if err != nil {
		return body
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}

	base, _ := url.Parse(baseURL)
	rewriteLinks(root, func(href string) string {
		linkURL := resolveLinkURL(base, href)
		if linkURL == "" {
			return href
		}
		return linkURL
	})

	var buf bytes.Buffer
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		if err := xhtml.Render(&buf, node); err != nil {
			return body
		}
	}
	return strings.TrimSpace(buf.String())
}

func rewriteLinks(node *xhtml.Node, rewrite func(string) string) {
	if node.Type == xhtml.ElementNode && node.Data == "a" {
		for i := range node.Attr {
			if strings.EqualFold(node.Attr[i].Key, "href") {
				node.Attr[i].Val = rewrite(strings.TrimSpace(node.Attr[i].Val))
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		rewriteLinks(child, rewrite)
	}
}

func resolveLinkURL(base *url.URL, href string) string {
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base == nil || base.Scheme == "" || base.Host == "" {
		return ""
	}
	if ref.IsAbs() {
		return ""
	}
	return base.ResolveReference(ref).String()
}

func resolveImageURL(base *url.URL, src string) string {
	if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "cid:") {
		return ""
	}
	ref, err := url.Parse(src)
	if err != nil {
		return ""
	}
	if !ref.IsAbs() {
		if base == nil || base.Scheme == "" || base.Host == "" {
			return ""
		}
		ref = base.ResolveReference(ref)
	}
	if ref.Scheme != "http" && ref.Scheme != "https" {
		return ""
	}
	return ref.String()
}

func downloadImage(ctx context.Context, client *http.Client, imageURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "htb2kdl/0.1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("status %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty image")
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func imageFileName(chapterNo int, imageURL, contentType string, data []byte) string {
	sum := sha1.Sum([]byte(imageURL))
	ext := imageExtension(imageURL, contentType, data)
	return fmt.Sprintf("chapter-%03d-%s%s", chapterNo, hex.EncodeToString(sum[:])[:12], ext)
}

func imageExtension(imageURL, contentType string, data []byte) string {
	if parsed, err := url.Parse(imageURL); err == nil {
		ext := strings.ToLower(path.Ext(parsed.Path))
		if isImageExt(ext) {
			return ext
		}
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		switch mediaType {
		case "image/jpeg":
			return ".jpg"
		case "image/png":
			return ".png"
		case "image/gif":
			return ".gif"
		case "image/svg+xml":
			return ".svg"
		case "image/webp":
			return ".webp"
		}
	}
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".img"
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp":
		return true
	default:
		return false
	}
}

func normalizeEPUB(path string, stylesheet []byte) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	entries := make([]zipEntry, 0, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}

		switch file.Name {
		case "epub/content.opf":
			data = ensurePackageNamespaces(data)
			data = ensureModifiedMetadata(data)
			data = ensureImageMediaTypes(data)
			if len(stylesheet) > 0 {
				data = ensureStylesheetManifest(data)
			}
		case "epub/toc.ncx":
			data = ensureNCXMetadata(data)
		case "epub/toc.xhtml":
			data = ensureEPUBNamespace(data)
			data = removeEmptyLandmarks(data)
		}

		entries = append(entries, zipEntry{
			name:   file.Name,
			method: file.Method,
			data:   data,
		})
	}
	if uid := packageIdentifier(entries); uid != "" {
		for i := range entries {
			if entries[i].name == "epub/toc.ncx" {
				entries[i].data = ensureNCXUID(entries[i].data, uid)
			}
		}
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	if err := writeEntry(writer, zipEntry{
		name:   "mimetype",
		method: zip.Store,
		data:   []byte("application/epub+zip"),
	}); err != nil {
		_ = writer.Close()
		return err
	}

	for _, entry := range entries {
		if entry.name == "mimetype" {
			continue
		}
		if err := writeEntry(writer, entry); err != nil {
			_ = writer.Close()
			return err
		}
	}
	if len(stylesheet) > 0 {
		if err := writeEntry(writer, zipEntry{
			name:   "epub/style.css",
			method: zip.Deflate,
			data:   stylesheet,
		}); err != nil {
			_ = writer.Close()
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func writeEntry(writer *zip.Writer, entry zipEntry) error {
	header := &zip.FileHeader{
		Name:   entry.name,
		Method: entry.method,
	}
	w, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = w.Write(entry.data)
	return err
}

func ensurePackageNamespaces(data []byte) []byte {
	const dcNS = `xmlns:dc="http://purl.org/dc/elements/1.1/"`
	if !bytes.Contains(data, []byte(dcNS)) {
		data = bytes.Replace(data, []byte(`<package `), []byte(`<package `+dcNS+` `), 1)
	}
	return bytes.Replace(data, []byte(`toc="ncx"`), []byte(`toc="toc"`), 1)
}

func ensureModifiedMetadata(data []byte) []byte {
	if bytes.Contains(data, []byte(`property="dcterms:modified"`)) {
		return data
	}
	modified := "1970-01-01T00:00:00Z"
	dateRe := regexp.MustCompile(`<dc:date[^>]*>([^<]+)</dc:date>`)
	if matches := dateRe.FindSubmatch(data); len(matches) == 2 {
		if t, err := time.Parse(time.RFC3339, string(matches[1])); err == nil {
			modified = t.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	meta := `<meta property="dcterms:modified">` + modified + `</meta>`
	return bytes.Replace(data, []byte(`</metadata>`), []byte("    "+meta+"\n  </metadata>"), 1)
}

func ensureNCXMetadata(data []byte) []byte {
	data = regexp.MustCompile(`version="[^"]*"`).ReplaceAll(data, []byte(`version="2005-1"`))
	if bytes.Contains(data, []byte(`<meta name="dtb:uid"`)) {
		return data
	}
	head := `<head><meta name="dtb:uid" content="htb2kdl"></meta><meta name="dtb:depth" content="1"></meta><meta name="dtb:totalPageCount" content="0"></meta><meta name="dtb:maxPageNumber" content="0"></meta></head>`
	return regexp.MustCompile(`<head>\s*</head>`).ReplaceAll(data, []byte(head))
}

func packageIdentifier(entries []zipEntry) string {
	re := regexp.MustCompile(`<dc:identifier[^>]*\bid="pub-id"[^>]*>([^<]+)</dc:identifier>`)
	for _, entry := range entries {
		if entry.name != "epub/content.opf" {
			continue
		}
		if matches := re.FindSubmatch(entry.data); len(matches) == 2 {
			return string(matches[1])
		}
	}
	return ""
}

func ensureNCXUID(data []byte, uid string) []byte {
	re := regexp.MustCompile(`(<meta name="dtb:uid" content=")[^"]*("></meta>)`)
	return re.ReplaceAll(data, []byte(`${1}`+html.EscapeString(uid)+`${2}`))
}

func ensureStylesheetManifest(data []byte) []byte {
	const item = `<item id="style-css" href="style.css" media-type="text/css"></item>`
	if bytes.Contains(data, []byte(`href="style.css"`)) {
		return data
	}
	return bytes.Replace(data, []byte(`</manifest>`), []byte(item+"\n  </manifest>"), 1)
}

func ensureImageMediaTypes(data []byte) []byte {
	replacements := map[string]string{
		".svg":  "image/svg+xml",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
	}
	for ext, mediaType := range replacements {
		re := regexp.MustCompile(`(<item\b[^>]*href="[^"]+` + regexp.QuoteMeta(ext) + `"[^>]*media-type=")[^"]+("[^>]*>)`)
		data = re.ReplaceAll(data, []byte(`${1}`+mediaType+`${2}`))
	}
	return data
}

func ensureEPUBNamespace(data []byte) []byte {
	const epubNS = `xmlns:epub="http://www.idpf.org/2007/ops"`
	if bytes.Contains(data, []byte(epubNS)) {
		return data
	}
	return bytes.Replace(data, []byte(`<html `), []byte(`<html `+epubNS+` `), 1)
}

func removeEmptyLandmarks(data []byte) []byte {
	re := regexp.MustCompile(`<nav\b[^>]*\bepub:type="landmarks"[^>]*>.*?<ol>\s*</ol>\s*</nav>`)
	return re.ReplaceAll(data, nil)
}

func renderChapter(ch Chapter, includeStylesheet bool) string {
	date := ""
	if !ch.BookmarkedAt.IsZero() {
		date = ch.BookmarkedAt.Format(time.RFC3339)
	}

	var meta strings.Builder
	meta.WriteString(`<p class="source">`)
	if ch.URL != "" {
		meta.WriteString(`URL: <a href="`)
		meta.WriteString(html.EscapeString(ch.URL))
		meta.WriteString(`">`)
		meta.WriteString(html.EscapeString(ch.URL))
		meta.WriteString(`</a>`)
	}
	if date != "" {
		if ch.URL != "" {
			meta.WriteString(`<br />`)
		}
		meta.WriteString(`Bookmarked: `)
		meta.WriteString(html.EscapeString(date))
	}
	meta.WriteString(`</p>`)

	stylesheet := ""
	if includeStylesheet {
		stylesheet = `
  <link rel="stylesheet" type="text/css" href="style.css" />`
	}

	return `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="ja" xml:lang="ja">
<head>
  <meta charset="utf-8" />
  <title>` + html.EscapeString(ch.Title) + `</title>` + stylesheet + `
</head>
<body>
  <h1>` + html.EscapeString(ch.Title) + `</h1>
  ` + meta.String() + `
  ` + ch.HTMLBody + `
</body>
</html>`
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "bookmarks"
	}
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	return strings.Trim(re.ReplaceAllString(value, "-"), "-")
}

func blankCover() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{R: 245, G: 245, B: 245, A: 255})
		}
	}
	return img
}
