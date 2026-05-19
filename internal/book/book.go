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

func DefaultOutputPath(user string, created time.Time) string {
	name := fmt.Sprintf("hateb-%s-%s.epub", safeName(user), created.Format("200601021504"))
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
	if err := writer.CoverPNG(coverImage(opts.Author, opts.Created)); err != nil {
		return fmt.Errorf("EPUB カバーの作成に失敗しました: %w", err)
	}
	writer.AddContent("cover.xhtml", []byte(renderCoverPage(opts.Author, opts.Created)))
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
	chapterFiles := make([]string, len(opts.Chapters))
	for i := range opts.Chapters {
		chapterFiles[i] = fmt.Sprintf("chapter-%03d.xhtml", i+1)
	}
	writer.AddContent("bookmarks.xhtml", []byte(renderBookmarksPage(opts.Chapters, chapterFiles, len(opts.Stylesheet) > 0)))
	for i, chapter := range opts.Chapters {
		filename := chapterFiles[i]
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
			data = removeCoverImageMetadata(data)
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

func removeCoverImageMetadata(data []byte) []byte {
	data = regexp.MustCompile(`(?m)\s*<meta name="cover" content="[^"]+"></meta>\s*`).ReplaceAll(data, []byte("\n"))
	data = regexp.MustCompile(`\s+properties="cover-image"`).ReplaceAll(data, nil)
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

func renderCoverPage(user string, created time.Time) string {
	title := "はてブ"
	if user != "" {
		title += " - " + user
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="ja" xml:lang="ja">
<head>
  <meta charset="utf-8" />
  <title>` + html.EscapeString(title) + `</title>
  <style>
    html, body {
      margin: 0;
      padding: 0;
      width: 100%;
      height: 100%;
      background: #f5f5f5;
    }
    body {
      display: flex;
      align-items: center;
      justify-content: center;
    }
    img {
      display: block;
      max-width: 100%;
      max-height: 100%;
    }
  </style>
</head>
<body>
  <img src="images/cover.png" alt="` + html.EscapeString(title+" "+created.Format("2006/01/02 15:04")) + `" />
</body>
</html>`
}

func renderBookmarksPage(chapters []Chapter, chapterFiles []string, includeStylesheet bool) string {
	stylesheet := ""
	if includeStylesheet {
		stylesheet = `
  <link rel="stylesheet" type="text/css" href="style.css" />`
	}

	var list strings.Builder
	list.WriteString("<ol>\n")
	for i, chapter := range chapters {
		title := chapter.Title
		if title == "" {
			title = chapter.URL
		}
		if title == "" {
			title = fmt.Sprintf("Bookmark %d", i+1)
		}
		list.WriteString(`    <li><a href="`)
		list.WriteString(html.EscapeString(chapterFiles[i]))
		list.WriteString(`">`)
		list.WriteString(html.EscapeString(title))
		list.WriteString(`</a></li>`)
		list.WriteString("\n")
	}
	list.WriteString("  </ol>")

	return `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="ja" xml:lang="ja">
<head>
  <meta charset="utf-8" />
  <title>ブックマーク一覧</title>` + stylesheet + `
</head>
<body>
  <h1>ブックマーク一覧</h1>
  ` + list.String() + `
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

func coverImage(user string, created time.Time) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 800))
	background := color.RGBA{R: 245, G: 245, B: 245, A: 255}
	for y := 0; y < 800; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, background)
		}
	}
	drawCoverLines(img, []coverLine{
		{text: "はてブ", preferredScale: 12},
		{text: user, preferredScale: 9},
		{text: created.Format("2006/01/02 15:04"), preferredScale: 5},
	}, color.RGBA{R: 30, G: 30, B: 30, A: 255})
	return img
}

type coverLine struct {
	text           string
	preferredScale int
}

func drawCoverLines(img *image.RGBA, lines []coverLine, c color.Color) {
	scales := make([]int, 0, len(lines))
	totalHeight := 0
	const lineGap = 36

	for _, line := range lines {
		scale := coverLineScale(line.text, line.preferredScale, img.Bounds().Dx()-80)
		scales = append(scales, scale)
		totalHeight += coverGlyphHeight * scale
	}
	if len(lines) > 1 {
		totalHeight += lineGap * (len(lines) - 1)
	}

	y := (img.Bounds().Dy() - totalHeight) / 2
	for i, line := range lines {
		scale := scales[i]
		width := coverTextWidth(line.text, scale)
		x := (img.Bounds().Dx() - width) / 2
		drawCoverTextAt(img, line.text, x, y, scale, c)
		y += coverGlyphHeight*scale + lineGap
	}
}

func coverLineScale(text string, preferredScale, maxWidth int) int {
	if preferredScale < 1 {
		preferredScale = 1
	}
	scale := preferredScale
	for scale > 1 && coverTextWidth(text, scale) > maxWidth {
		scale--
	}
	return scale
}

func coverTextWidth(text string, scale int) int {
	width := 0
	for range text {
		width += (coverGlyphWidth + coverGlyphSpacing) * scale
	}
	if width > 0 {
		width -= coverGlyphSpacing * scale
	}
	return width
}

func drawCoverTextAt(img *image.RGBA, text string, x, y, scale int, c color.Color) {
	for _, r := range text {
		rows, ok := coverGlyph(r)
		if !ok {
			x += (coverGlyphWidth + coverGlyphSpacing) * scale
			continue
		}
		for row, bits := range rows {
			for col, bit := range bits {
				if bit != '1' {
					continue
				}
				fillRect(img, x+col*scale, y+row*scale, scale, scale, c)
			}
		}
		x += (coverGlyphWidth + coverGlyphSpacing) * scale
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.Color) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			img.Set(xx, yy, c)
		}
	}
}

const (
	coverGlyphWidth   = 5
	coverGlyphHeight  = 7
	coverGlyphSpacing = 2
)

func coverGlyph(r rune) ([]string, bool) {
	rows, ok := coverGlyphs[r]
	if ok {
		return rows, true
	}
	if 'a' <= r && r <= 'z' {
		rows, ok = coverGlyphs[r-'a'+'A']
	}
	return rows, ok
}

var coverGlyphs = map[rune][]string{
	'は': {
		"10010",
		"10010",
		"10111",
		"11110",
		"10010",
		"10010",
		"10111",
	},
	'て': {
		"11111",
		"00010",
		"00100",
		"01000",
		"01000",
		"00100",
		"00011",
	},
	'ブ': {
		"11101",
		"00001",
		"00000",
		"00010",
		"00100",
		"01000",
		"10000",
	},
	'-': {
		"00000",
		"00000",
		"00000",
		"11111",
		"00000",
		"00000",
		"00000",
	},
	'/': {
		"00001",
		"00010",
		"00010",
		"00100",
		"01000",
		"01000",
		"10000",
	},
	':': {
		"00000",
		"00100",
		"00100",
		"00000",
		"00100",
		"00100",
		"00000",
	},
	'.': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"01100",
		"01100",
	},
	'_': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"11111",
	},
	'A': {
		"01110",
		"10001",
		"10001",
		"11111",
		"10001",
		"10001",
		"10001",
	},
	'B': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10001",
		"10001",
		"11110",
	},
	'C': {
		"01111",
		"10000",
		"10000",
		"10000",
		"10000",
		"10000",
		"01111",
	},
	'D': {
		"11110",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"11110",
	},
	'E': {
		"11111",
		"10000",
		"10000",
		"11110",
		"10000",
		"10000",
		"11111",
	},
	'F': {
		"11111",
		"10000",
		"10000",
		"11110",
		"10000",
		"10000",
		"10000",
	},
	'G': {
		"01111",
		"10000",
		"10000",
		"10111",
		"10001",
		"10001",
		"01111",
	},
	'H': {
		"10001",
		"10001",
		"10001",
		"11111",
		"10001",
		"10001",
		"10001",
	},
	'I': {
		"01110",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'J': {
		"00111",
		"00010",
		"00010",
		"00010",
		"10010",
		"10010",
		"01100",
	},
	'K': {
		"10001",
		"10010",
		"10100",
		"11000",
		"10100",
		"10010",
		"10001",
	},
	'L': {
		"10000",
		"10000",
		"10000",
		"10000",
		"10000",
		"10000",
		"11111",
	},
	'M': {
		"10001",
		"11011",
		"10101",
		"10101",
		"10001",
		"10001",
		"10001",
	},
	'N': {
		"10001",
		"11001",
		"10101",
		"10011",
		"10001",
		"10001",
		"10001",
	},
	'O': {
		"01110",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"01110",
	},
	'P': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10000",
		"10000",
		"10000",
	},
	'Q': {
		"01110",
		"10001",
		"10001",
		"10001",
		"10101",
		"10010",
		"01101",
	},
	'R': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10100",
		"10010",
		"10001",
	},
	'S': {
		"01111",
		"10000",
		"10000",
		"01110",
		"00001",
		"00001",
		"11110",
	},
	'T': {
		"11111",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
	},
	'U': {
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"01110",
	},
	'V': {
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"01010",
		"00100",
	},
	'W': {
		"10001",
		"10001",
		"10001",
		"10101",
		"10101",
		"10101",
		"01010",
	},
	'X': {
		"10001",
		"10001",
		"01010",
		"00100",
		"01010",
		"10001",
		"10001",
	},
	'Y': {
		"10001",
		"10001",
		"01010",
		"00100",
		"00100",
		"00100",
		"00100",
	},
	'Z': {
		"11111",
		"00001",
		"00010",
		"00100",
		"01000",
		"10000",
		"11111",
	},
	'a': {
		"00000",
		"01110",
		"00001",
		"01111",
		"10001",
		"10011",
		"01101",
	},
	'b': {
		"10000",
		"10000",
		"10110",
		"11001",
		"10001",
		"10001",
		"11110",
	},
	'c': {
		"00000",
		"00000",
		"01111",
		"10000",
		"10000",
		"10000",
		"01111",
	},
	'd': {
		"00001",
		"00001",
		"01101",
		"10011",
		"10001",
		"10001",
		"01111",
	},
	'e': {
		"00000",
		"00000",
		"01110",
		"10001",
		"11111",
		"10000",
		"01111",
	},
	'f': {
		"00110",
		"01001",
		"01000",
		"11100",
		"01000",
		"01000",
		"01000",
	},
	'g': {
		"00000",
		"01111",
		"10001",
		"10001",
		"01111",
		"00001",
		"01110",
	},
	'h': {
		"10000",
		"10000",
		"10110",
		"11001",
		"10001",
		"10001",
		"10001",
	},
	'i': {
		"00100",
		"00000",
		"01100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'j': {
		"00010",
		"00000",
		"00110",
		"00010",
		"00010",
		"10010",
		"01100",
	},
	'k': {
		"10000",
		"10000",
		"10010",
		"10100",
		"11000",
		"10100",
		"10010",
	},
	'l': {
		"01100",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'm': {
		"00000",
		"00000",
		"11010",
		"10101",
		"10101",
		"10101",
		"10101",
	},
	'n': {
		"00000",
		"00000",
		"10110",
		"11001",
		"10001",
		"10001",
		"10001",
	},
	'o': {
		"00000",
		"00000",
		"01110",
		"10001",
		"10001",
		"10001",
		"01110",
	},
	'p': {
		"00000",
		"11110",
		"10001",
		"10001",
		"11110",
		"10000",
		"10000",
	},
	'q': {
		"00000",
		"01111",
		"10001",
		"10001",
		"01111",
		"00001",
		"00001",
	},
	'r': {
		"00000",
		"00000",
		"10110",
		"11001",
		"10000",
		"10000",
		"10000",
	},
	's': {
		"00000",
		"00000",
		"01111",
		"10000",
		"01110",
		"00001",
		"11110",
	},
	't': {
		"01000",
		"01000",
		"11100",
		"01000",
		"01000",
		"01001",
		"00110",
	},
	'u': {
		"00000",
		"00000",
		"10001",
		"10001",
		"10001",
		"10011",
		"01101",
	},
	'v': {
		"00000",
		"00000",
		"10001",
		"10001",
		"10001",
		"01010",
		"00100",
	},
	'w': {
		"00000",
		"00000",
		"10001",
		"10001",
		"10101",
		"10101",
		"01010",
	},
	'x': {
		"00000",
		"00000",
		"10001",
		"01010",
		"00100",
		"01010",
		"10001",
	},
	'y': {
		"00000",
		"10001",
		"10001",
		"10001",
		"01111",
		"00001",
		"01110",
	},
	'z': {
		"00000",
		"00000",
		"11111",
		"00010",
		"00100",
		"01000",
		"11111",
	},
	'0': {
		"01110",
		"10001",
		"10011",
		"10101",
		"11001",
		"10001",
		"01110",
	},
	'1': {
		"00100",
		"01100",
		"00100",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'2': {
		"01110",
		"10001",
		"00001",
		"00010",
		"00100",
		"01000",
		"11111",
	},
	'3': {
		"11110",
		"00001",
		"00001",
		"01110",
		"00001",
		"00001",
		"11110",
	},
	'4': {
		"00010",
		"00110",
		"01010",
		"10010",
		"11111",
		"00010",
		"00010",
	},
	'5': {
		"11111",
		"10000",
		"10000",
		"11110",
		"00001",
		"00001",
		"11110",
	},
	'6': {
		"00110",
		"01000",
		"10000",
		"11110",
		"10001",
		"10001",
		"01110",
	},
	'7': {
		"11111",
		"00001",
		"00010",
		"00100",
		"01000",
		"01000",
		"01000",
	},
	'8': {
		"01110",
		"10001",
		"10001",
		"01110",
		"10001",
		"10001",
		"01110",
	},
	'9': {
		"01110",
		"10001",
		"10001",
		"01111",
		"00001",
		"00010",
		"01100",
	},
}
