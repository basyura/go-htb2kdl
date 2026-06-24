package book

import (
	"archive/zip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/raitucarp/epub"
)

func TestWrite(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book.epub")
	err := Write(Options{
		Title:   "Test Book",
		Author:  "alice",
		Output:  out,
		Created: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
		Stylesheet: []byte(`body {
  font-family: serif;
}`),
		Chapters: []Chapter{
			{
				Title:    "Chapter 1",
				URL:      "https://example.com",
				HTMLBody: "<p>Hello</p>",
			},
			{
				Title:    "Chapter 2",
				URL:      "https://example.com/2",
				HTMLBody: "<p>World</p>",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("empty epub")
	}

	reader, err := epub.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	if reader.Title() != "Test Book" {
		t.Fatalf("title = %q", reader.Title())
	}

	zipReader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()

	if len(zipReader.File) == 0 || zipReader.File[0].Name != "mimetype" {
		t.Fatalf("first zip entry = %q, want mimetype", zipReader.File[0].Name)
	}
	if zipReader.File[0].Method != zip.Store {
		t.Fatalf("mimetype method = %d, want Store", zipReader.File[0].Method)
	}

	opf := readZipFile(t, &zipReader.Reader, "epub/content.opf")
	if !strings.Contains(opf, `xmlns:dc="http://purl.org/dc/elements/1.1/"`) {
		t.Fatalf("content.opf is missing dc namespace: %s", opf)
	}
	if !strings.Contains(opf, `toc="toc"`) {
		t.Fatalf("content.opf spine toc does not reference NCX item: %s", opf)
	}
	if !strings.Contains(opf, `href="style.css" media-type="text/css"`) {
		t.Fatalf("content.opf is missing stylesheet manifest item: %s", opf)
	}
	if !strings.Contains(opf, `<item href="cover.xhtml" id="cover.xhtml" media-type="application/xhtml+xml"`) {
		t.Fatalf("content.opf is missing cover page manifest item: %s", opf)
	}
	if strings.Contains(opf, `properties="cover-image"`) || strings.Contains(opf, `<meta name="cover"`) {
		t.Fatalf("content.opf should not expose a separate cover image page: %s", opf)
	}
	if !strings.Contains(opf, `<itemref idref="cover.xhtml"></itemref>`) {
		t.Fatalf("content.opf spine is missing cover page: %s", opf)
	}
	if strings.Index(opf, `<itemref idref="cover.xhtml"></itemref>`) > strings.Index(opf, `<itemref idref="chapter-001.xhtml"></itemref>`) {
		t.Fatalf("cover page should be first in spine: %s", opf)
	}
	if !strings.Contains(opf, `<item href="bookmarks.xhtml" id="bookmarks.xhtml" media-type="application/xhtml+xml"`) {
		t.Fatalf("content.opf is missing bookmarks page manifest item: %s", opf)
	}
	if !strings.Contains(opf, `<itemref idref="bookmarks.xhtml"></itemref>`) {
		t.Fatalf("content.opf spine is missing bookmarks page: %s", opf)
	}
	if strings.Index(opf, `<itemref idref="bookmarks.xhtml"></itemref>`) < strings.Index(opf, `<itemref idref="cover.xhtml"></itemref>`) ||
		strings.Index(opf, `<itemref idref="bookmarks.xhtml"></itemref>`) > strings.Index(opf, `<itemref idref="chapter-001.xhtml"></itemref>`) {
		t.Fatalf("bookmarks page should be between cover and first chapter: %s", opf)
	}
	if !strings.Contains(opf, `<item href="end.xhtml" id="end.xhtml" media-type="application/xhtml+xml"`) {
		t.Fatalf("content.opf is missing end page manifest item: %s", opf)
	}
	if !strings.Contains(opf, `<itemref idref="end.xhtml"></itemref>`) {
		t.Fatalf("content.opf spine is missing end page: %s", opf)
	}
	if strings.Index(opf, `<itemref idref="end.xhtml"></itemref>`) < strings.Index(opf, `<itemref idref="chapter-002.xhtml"></itemref>`) {
		t.Fatalf("end page should be after the final chapter: %s", opf)
	}

	toc := readZipFile(t, &zipReader.Reader, "epub/toc.xhtml")
	if !strings.Contains(toc, `xmlns:epub="http://www.idpf.org/2007/ops"`) {
		t.Fatalf("toc.xhtml is missing epub namespace: %s", toc)
	}

	coverPage := readZipFile(t, &zipReader.Reader, "epub/cover.xhtml")
	if !strings.Contains(coverPage, `src="images/cover.png"`) {
		t.Fatalf("cover page does not reference cover image: %s", coverPage)
	}

	bookmarksPage := readZipFile(t, &zipReader.Reader, "epub/bookmarks.xhtml")
	if !strings.Contains(bookmarksPage, `<h1>ブックマーク一覧</h1>`) {
		t.Fatalf("bookmarks page is missing heading: %s", bookmarksPage)
	}
	if !strings.Contains(bookmarksPage, `<a href="chapter-001.xhtml">Chapter 1</a>`) ||
		!strings.Contains(bookmarksPage, `<a href="chapter-002.xhtml">Chapter 2</a>`) {
		t.Fatalf("bookmarks page is missing chapter links: %s", bookmarksPage)
	}

	stylesheet := readZipFile(t, &zipReader.Reader, "epub/style.css")
	if !strings.Contains(stylesheet, "font-family: serif") {
		t.Fatalf("style.css = %q", stylesheet)
	}

	chapter := readZipFile(t, &zipReader.Reader, "epub/chapter-001.xhtml")
	if !strings.Contains(chapter, `<link rel="stylesheet" type="text/css" href="style.css" />`) {
		t.Fatalf("chapter is missing stylesheet link: %s", chapter)
	}
	if strings.Contains(opf, `<itemref idref="style-css"`) {
		t.Fatalf("stylesheet should not be in spine: %s", opf)
	}

	endPage := readZipFile(t, &zipReader.Reader, "epub/end.xhtml")
	if !strings.Contains(endPage, `<p>END</p>`) {
		t.Fatalf("end page is missing END text: %s", endPage)
	}
	if !strings.Contains(endPage, `align-items: center;`) || !strings.Contains(endPage, `justify-content: center;`) {
		t.Fatalf("end page does not center END text: %s", endPage)
	}
}

func TestDefaultOutputPathUsesCreatedTimestamp(t *testing.T) {
	created := time.Date(2026, 5, 18, 18, 1, 30, 0, time.UTC)
	got := DefaultOutputPath("basyura", created)
	want := "hateb-basyura-20260518-1801.epub"
	if got != want {
		t.Fatalf("DefaultOutputPath = %q, want %q", got, want)
	}
}

func TestCoverImageHasTitlePixels(t *testing.T) {
	img := coverImage("basyura", time.Date(2026, 5, 19, 18, 1, 0, 0, time.UTC))
	background := img.At(0, 0)

	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.At(x, y) != background {
				return
			}
		}
	}
	t.Fatal("cover image has no title pixels")
}

func TestWriteWithoutStylesheet(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book.epub")
	err := Write(Options{
		Title:   "Test Book",
		Author:  "alice",
		Output:  out,
		Created: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
		Chapters: []Chapter{
			{
				Title:    "Chapter 1",
				URL:      "https://example.com",
				HTMLBody: "<p>Hello</p>",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	zipReader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()

	opf := readZipFile(t, &zipReader.Reader, "epub/content.opf")
	if strings.Contains(opf, `href="style.css"`) {
		t.Fatalf("content.opf should not include stylesheet: %s", opf)
	}

	chapter := readZipFile(t, &zipReader.Reader, "epub/chapter-001.xhtml")
	if strings.Contains(chapter, `href="style.css"`) {
		t.Fatalf("chapter should not include stylesheet link: %s", chapter)
	}
}

func TestWriteDownloadsImages(t *testing.T) {
	const png = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/assets/picture.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte(png))
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "book.epub")
	err := Write(Options{
		Title:      "Test Book",
		Author:     "alice",
		Output:     out,
		Created:    time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
		HTTPClient: server.Client(),
		Chapters: []Chapter{
			{
				Title:    "Chapter 1",
				URL:      server.URL + "/articles/1",
				HTMLBody: `<p>Hello</p><img alt="picture" src="/assets/picture.png" />`,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	zipReader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()

	opf := readZipFile(t, &zipReader.Reader, "epub/content.opf")
	if !strings.Contains(opf, `href="images/chapter-001-`) || !strings.Contains(opf, `media-type="image/png"`) {
		t.Fatalf("content.opf is missing image manifest item: %s", opf)
	}

	chapter := readZipFile(t, &zipReader.Reader, "epub/chapter-001.xhtml")
	if strings.Contains(chapter, `src="/assets/picture.png"`) || strings.Contains(chapter, `src="`+server.URL) {
		t.Fatalf("chapter still references remote image: %s", chapter)
	}
	if !strings.Contains(chapter, `src="images/chapter-001-`) {
		t.Fatalf("chapter does not reference embedded image: %s", chapter)
	}

	foundImage := false
	for _, file := range zipReader.File {
		if strings.HasPrefix(file.Name, "epub/images/chapter-001-") && strings.HasSuffix(file.Name, ".png") {
			foundImage = true
			image := readZipFile(t, &zipReader.Reader, file.Name)
			if image != png {
				t.Fatalf("embedded image = %q, want %q", image, png)
			}
		}
	}
	if !foundImage {
		t.Fatal("missing embedded image")
	}
}

func TestWriteNormalizesRootRelativeLinks(t *testing.T) {
	out := filepath.Join(t.TempDir(), "book.epub")
	err := Write(Options{
		Title:   "Test Book",
		Author:  "alice",
		Output:  out,
		Created: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
		Chapters: []Chapter{
			{
				Title:    "Chapter 1",
				URL:      "https://example.com/articles/1",
				HTMLBody: `<p><a href="/topics/go">Go</a> <a href="#local">Local</a></p>`,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	zipReader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()

	chapter := readZipFile(t, &zipReader.Reader, "epub/chapter-001.xhtml")
	if !strings.Contains(chapter, `href="https://example.com/topics/go"`) {
		t.Fatalf("root-relative link was not normalized: %s", chapter)
	}
	if !strings.Contains(chapter, `href="https://example.com/articles/1#local"`) {
		t.Fatalf("fragment link was not normalized: %s", chapter)
	}
}

func TestEnsureImageMediaTypes(t *testing.T) {
	got := string(ensureImageMediaTypes([]byte(`<item href="images/icon.svg" id="icon" media-type="text/plain; charset=utf-8"></item>`)))
	if !strings.Contains(got, `media-type="image/svg+xml"`) {
		t.Fatalf("svg media type was not normalized: %s", got)
	}
}

func TestRemoveCoverImageMetadata(t *testing.T) {
	opf := []byte(`<metadata><meta name="cover" content="cover.png"></meta></metadata><manifest><item href="images/cover.png" id="cover.png" media-type="image/png" properties="cover-image"></item></manifest>`)
	got := string(removeCoverImageMetadata(opf))
	if strings.Contains(got, `<meta name="cover"`) || strings.Contains(got, `properties="cover-image"`) {
		t.Fatalf("cover image metadata remains: %s", got)
	}
	if !strings.Contains(got, `href="images/cover.png"`) {
		t.Fatalf("cover image item should remain as a normal image: %s", got)
	}
}

func TestEnsureEPUBCheckMetadata(t *testing.T) {
	opf := []byte(`<package><metadata><dc:date>2026-05-19T14:07:41+09:00</dc:date></metadata></package>`)
	gotOPF := string(ensureModifiedMetadata(opf))
	if !strings.Contains(gotOPF, `<meta property="dcterms:modified">2026-05-19T05:07:41Z</meta>`) {
		t.Fatalf("missing modified metadata: %s", gotOPF)
	}

	ncx := []byte(`<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version=""><head></head></ncx>`)
	gotNCX := string(ensureNCXMetadata(ncx))
	if !strings.Contains(gotNCX, `version="2005-1"`) || !strings.Contains(gotNCX, `name="dtb:uid"`) {
		t.Fatalf("ncx metadata was not normalized: %s", gotNCX)
	}
	gotNCX = string(ensureNCXUID([]byte(gotNCX), "htb2kdl:alice:20260519000000"))
	if !strings.Contains(gotNCX, `content="htb2kdl:alice:20260519000000"`) {
		t.Fatalf("ncx uid was not normalized: %s", gotNCX)
	}

	toc := []byte(`<html><body><nav id="toc" epub:type="toc"><ol><li>x</li></ol></nav><nav id="landmarks" epub:type="landmarks"><h2>Landmarks</h2><ol></ol></nav></body></html>`)
	gotTOC := string(removeEmptyLandmarks(toc))
	if strings.Contains(gotTOC, `epub:type="landmarks"`) {
		t.Fatalf("empty landmarks nav remains: %s", gotTOC)
	}
}

func readZipFile(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatalf("missing zip file: %s", name)
	return ""
}
