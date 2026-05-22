package bookmarks

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "bookmarks.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Users == nil {
		t.Fatal("Users is nil")
	}
}

func TestLoadAndSaveAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.yml")
	file := File{
		Mail: MailConfig{
			From:        "sender@gmail.com",
			To:          "kindle@example.com",
			AppPassword: "app password",
		},
	}
	file.Add("alice", []string{"https://example.com/1", "https://example.com/2"})
	file.CompleteFirst("alice", 1)

	if err := SaveAtomic(path, file); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://example.com/2"}
	if !reflect.DeepEqual(got.Queued("alice"), want) {
		t.Fatalf("Queued = %v, want %v", got.Queued("alice"), want)
	}
	completed := got.Users["alice"].Completed
	if !reflect.DeepEqual(completed, []string{"https://example.com/1"}) {
		t.Fatalf("Completed = %v", completed)
	}
	if got.Mail.From != "sender@gmail.com" ||
		got.Mail.To != "kindle@example.com" ||
		got.Mail.AppPassword != "app password" {
		t.Fatalf("Mail = %+v", got.Mail)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version:") {
		t.Fatalf("bookmarks.yml should not contain version: %s", data)
	}
}

func TestLoadMailConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.yml")
	data := []byte(`mail:
  from: sender@gmail.com
  to: kindle@example.com
  app_password: app password
users:
  alice:
    bookmarks:
      - https://example.com/1
    completed: []
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mail.From != "sender@gmail.com" {
		t.Fatalf("From = %q", got.Mail.From)
	}
	if got.Mail.To != "kindle@example.com" {
		t.Fatalf("To = %q", got.Mail.To)
	}
	if got.Mail.AppPassword != "app password" {
		t.Fatalf("AppPassword = %q", got.Mail.AppPassword)
	}
}

func TestAddDeduplicatesAndPreservesOrder(t *testing.T) {
	file := File{}
	file.Add("alice", []string{"https://example.com/1", "https://example.com/2"})
	file.CompleteFirst("alice", 1)
	added := file.Add("alice", []string{"https://example.com/1", "https://example.com/2", "https://example.com/3"})

	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	want := []string{"https://example.com/2", "https://example.com/3"}
	if !reflect.DeepEqual(file.Queued("alice"), want) {
		t.Fatalf("Queued = %v, want %v", file.Queued("alice"), want)
	}
}

func TestCompleteFirst(t *testing.T) {
	file := File{}
	file.Add("alice", []string{"https://example.com/1", "https://example.com/2", "https://example.com/3"})

	file.CompleteFirst("alice", 2)

	want := []string{"https://example.com/3"}
	if !reflect.DeepEqual(file.Queued("alice"), want) {
		t.Fatalf("Queued = %v, want %v", file.Queued("alice"), want)
	}
	completed := []string{"https://example.com/1", "https://example.com/2"}
	if !reflect.DeepEqual(file.Users["alice"].Completed, completed) {
		t.Fatalf("Completed = %v, want %v", file.Users["alice"].Completed, completed)
	}
}

func TestCompleteFirstKeepsLatestCompletedURLs(t *testing.T) {
	file := File{
		Users: map[string]User{
			"alice": {
				Completed: make([]string, 99),
			},
		},
	}
	for i := range file.Users["alice"].Completed {
		file.Users["alice"].Completed[i] = fmt.Sprintf("https://example.com/completed-%03d", i+1)
	}
	file.Add("alice", []string{
		"https://example.com/new-1",
		"https://example.com/new-2",
	})

	file.CompleteFirst("alice", 2)

	completed := file.Users["alice"].Completed
	if len(completed) != maxCompletedURLs {
		t.Fatalf("len(Completed) = %d, want %d", len(completed), maxCompletedURLs)
	}
	if completed[0] != "https://example.com/completed-002" {
		t.Fatalf("first completed = %q", completed[0])
	}
	if completed[98] != "https://example.com/new-1" ||
		completed[99] != "https://example.com/new-2" {
		t.Fatalf("latest completed = %v", completed[98:])
	}
}

func TestLoadReportsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.yml")
	if err := os.WriteFile(path, []byte("users:\n  alice:\n    bookmarks: ["), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "bookmarks.yml の解析に失敗しました") {
		t.Fatalf("error = %v", err)
	}
}
