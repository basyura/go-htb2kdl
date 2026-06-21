package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	logFileName   = "htb2kdl.log"
	maxLogSize    = 5 * 1024 * 1024
	maxLogBackups = 3
)

// runtimeLogger writes timestamped runtime messages to the configured writer.
type runtimeLogger struct {
	writer io.Writer
}

// newRuntimeLogger creates a logger that writes to stdout and a rotating log
// file.
func newRuntimeLogger(stdout io.Writer, logPath string) (*runtimeLogger, io.Closer, error) {
	file, err := newRotatingLogFile(logPath, maxLogSize, maxLogBackups)
	if err != nil {
		return nil, nil, err
	}
	return &runtimeLogger{writer: io.MultiWriter(stdout, file)}, file, nil
}

// Printf writes a timestamped formatted log line.
func (l *runtimeLogger) Printf(format string, args ...any) {
	if l == nil || l.writer == nil {
		return
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(l.writer, "%s "+format+"\n", append([]any{timestamp}, args...)...)
}

// PrintStartBanner writes a visual separator for the start of a CLI run.
func (l *runtimeLogger) PrintStartBanner(startedAt time.Time) {
	if l == nil || l.writer == nil {
		return
	}
	fmt.Fprintf(l.writer, "###############################\n")
	fmt.Fprintf(l.writer, "#     %s     #\n", startedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(l.writer, "###############################\n")
}

// rotatingLogFile appends to a log file and rotates backup files when it grows
// beyond the configured size.
type rotatingLogFile struct {
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
	size       int64
}

// newRotatingLogFile opens a rotating log file, creating its directory if
// needed.
func newRotatingLogFile(path string, maxSize int64, maxBackups int) (*rotatingLogFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("ログディレクトリの作成に失敗しました: %w", err)
	}
	writer := &rotatingLogFile{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
	}
	if err := writer.open(); err != nil {
		return nil, err
	}
	return writer, nil
}

// Write appends data to the current log file, rotating it before the write when
// the size limit would be exceeded.
func (w *rotatingLogFile) Write(p []byte) (int, error) {
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("ログファイルの書き込みに失敗しました: %w", err)
	}
	return n, nil
}

// Close closes the current log file if it is open.
func (w *rotatingLogFile) Close() error {
	if w.file == nil {
		return nil
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("ログファイルのクローズに失敗しました: %w", err)
	}
	w.file = nil
	return nil
}

// open opens the current log file and rotates an oversized existing file before
// appending.
func (w *rotatingLogFile) open() error {
	info, err := os.Stat(w.path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ログファイルの確認に失敗しました: %w", err)
	}
	if err == nil && info.Size() > w.maxSize {
		if err := rotateLogFiles(w.path, w.maxBackups); err != nil {
			return err
		}
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ログファイルのオープンに失敗しました: %w", err)
	}
	info, err = file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("ログファイルの確認に失敗しました: %w", err)
	}
	w.file = file
	w.size = info.Size()
	return nil
}

// rotate closes the active file, rotates backups, and reopens the base log file.
func (w *rotatingLogFile) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return fmt.Errorf("ログファイルのクローズに失敗しました: %w", err)
		}
		w.file = nil
	}
	if err := rotateLogFiles(w.path, w.maxBackups); err != nil {
		return err
	}
	return w.open()
}

// rotateLogFiles shifts numbered backup files and moves the base log to the
// first backup slot.
func rotateLogFiles(path string, maxBackups int) error {
	oldest := fmt.Sprintf("%s.%d", path, maxBackups)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("古いログファイルの削除に失敗しました: %w", err)
	}

	for i := maxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", path, i)
		dst := fmt.Sprintf("%s.%d", path, i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("ログファイルのローテーションに失敗しました: %w", err)
		}
	}

	if err := os.Rename(path, path+".1"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ログファイルのローテーションに失敗しました: %w", err)
	}
	return nil
}

// defaultLogPath returns the default log path next to the executable.
func defaultLogPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("実行ファイルの場所を取得できませんでした: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), logFileName), nil
}
