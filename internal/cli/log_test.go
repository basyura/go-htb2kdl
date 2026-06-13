package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRuntimeLoggerWritesStdoutAndFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, logFileName)
	var stdout strings.Builder

	logger, closer, err := newRuntimeLogger(&stdout, logPath)
	if err != nil {
		t.Fatal(err)
	}
	logger.Printf("hello %s", "world")
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(stdout.String(), "hello world") {
		t.Fatalf("stdout = %q, want log message", stdout.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Fatalf("log file = %q, want log message", string(data))
	}
}

func TestRuntimeLoggerWritesStartBanner(t *testing.T) {
	var stdout strings.Builder
	logger := &runtimeLogger{writer: &stdout}
	startedAt := time.Date(2026, 6, 13, 22, 48, 31, 0, time.Local)

	logger.PrintStartBanner(startedAt)

	want := "###############################\n" +
		"#     2026-06-13 22:48:31     #\n" +
		"###############################\n"
	if stdout.String() != want {
		t.Fatalf("banner = %q, want %q", stdout.String(), want)
	}
}

func TestRotatingLogFileRotatesAndKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, logFileName)
	if err := os.WriteFile(logPath, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		path := filepath.Join(dir, logFileName+"."+strconv.Itoa(i))
		if err := os.WriteFile(path, []byte("backup"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writer, err := newRotatingLogFile(logPath, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("rotated")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, logPath, "rotated")
	assertFileContains(t, logPath+".1", "1234567890")
	assertFileContains(t, logPath+".2", "current")
	assertFileContains(t, logPath+".3", "backup")
	if _, err := os.Stat(logPath + ".4"); !os.IsNotExist(err) {
		t.Fatalf("logPath.4 exists or stat failed unexpectedly: %v", err)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s = %q, want %q", path, string(data), want)
	}
}
