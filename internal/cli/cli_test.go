package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStylesheetUsesDefaultWhenCSSPathIsEmpty(t *testing.T) {
	stylesheet, err := loadStylesheet("", []byte("default css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stylesheet) != "default css" {
		t.Fatalf("stylesheet = %q", stylesheet)
	}
}

func TestLoadStylesheetPrefersCSSPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "style.css")
	if err := os.WriteFile(path, []byte("custom css"), 0o644); err != nil {
		t.Fatal(err)
	}

	stylesheet, err := loadStylesheet(path, []byte("default css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stylesheet) != "custom css" {
		t.Fatalf("stylesheet = %q", stylesheet)
	}
}

func TestLoadStylesheetReportsCSSReadError(t *testing.T) {
	_, err := loadStylesheet(filepath.Join(t.TempDir(), "missing.css"), []byte("default css"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "CSS ファイルの読み込みに失敗しました") {
		t.Fatalf("error = %v", err)
	}
}
