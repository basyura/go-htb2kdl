package book

import (
	"archive/zip"
	"io"
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

	toc := readZipFile(t, &zipReader.Reader, "epub/toc.xhtml")
	if !strings.Contains(toc, `xmlns:epub="http://www.idpf.org/2007/ops"`) {
		t.Fatalf("toc.xhtml is missing epub namespace: %s", toc)
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
