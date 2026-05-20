package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildEPUBMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := os.WriteFile(path, []byte("epub data"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg, err := BuildEPUBMessage(Config{
		From:        "sender@gmail.com",
		To:          "kindle@example.com",
		AppPassword: "app password",
	}, EPUBMessage{
		Subject: "alice のはてなブックマーク",
		Body:    "本文",
		Path:    path,
	})
	if err != nil {
		t.Fatal(err)
	}

	got := string(msg)
	for _, want := range []string{
		"From: sender@gmail.com",
		"To: kindle@example.com",
		"Subject: =?utf-8?",
		"Content-Type: multipart/mixed;",
		"Content-Type: application/epub+zip; name=book.epub",
		"Content-Disposition: attachment; filename=book.epub",
		"ZXB1YiBkYXRh",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("message does not contain %q:\n%s", want, got)
		}
	}
}

func TestValidateReportsMissingConfig(t *testing.T) {
	err := (Config{}).Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mail.from") {
		t.Fatalf("error = %v", err)
	}
}
