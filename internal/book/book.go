package book

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/raitucarp/epub"
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

	toc := epub.TOC{
		Title: opts.Title,
		Items: make([]epub.TOC, 0, len(opts.Chapters)),
	}
	for i, chapter := range opts.Chapters {
		filename := fmt.Sprintf("chapter-%03d.xhtml", i+1)
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
			if len(stylesheet) > 0 {
				data = ensureStylesheetManifest(data)
			}
		case "epub/toc.xhtml":
			data = ensureEPUBNamespace(data)
		}

		entries = append(entries, zipEntry{
			name:   file.Name,
			method: file.Method,
			data:   data,
		})
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

func ensureStylesheetManifest(data []byte) []byte {
	const item = `<item id="style-css" href="style.css" media-type="text/css"></item>`
	if bytes.Contains(data, []byte(`href="style.css"`)) {
		return data
	}
	return bytes.Replace(data, []byte(`</manifest>`), []byte(item+"\n  </manifest>"), 1)
}

func ensureEPUBNamespace(data []byte) []byte {
	const epubNS = `xmlns:epub="http://www.idpf.org/2007/ops"`
	if bytes.Contains(data, []byte(epubNS)) {
		return data
	}
	return bytes.Replace(data, []byte(`<html `), []byte(`<html `+epubNS+` `), 1)
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
