package bookmarks

import (
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
	file := File{}
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

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "version:") {
		t.Fatalf("bookmarks.yml should not contain version: %s", data)
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
