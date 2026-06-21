package bookmarks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const maxCompletedURLs = 100

// File represents the contents of bookmarks.yml.
type File struct {
	Settings SettingsConfig  `yaml:"settings,omitempty"`
	Mail     MailConfig      `yaml:"mail,omitempty"`
	Users    map[string]User `yaml:"users"`
}

// SettingsConfig holds global queue settings from bookmarks.yml.
type SettingsConfig struct {
	Limit *int `yaml:"limit,omitempty"`
}

// MailConfig holds Gmail SMTP settings from bookmarks.yml.
type MailConfig struct {
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	AppPassword string `yaml:"app_password"`
}

// User stores queued and completed bookmark URLs for one Hatena user.
type User struct {
	Bookmarks []string `yaml:"bookmarks"`
	Completed []string `yaml:"completed"`
}

// Load reads bookmarks.yml and returns an empty file structure when it does not
// exist.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{Users: make(map[string]User)}, nil
		}
		return File{}, fmt.Errorf("bookmarks.yml の読み込みに失敗しました: %w", err)
	}

	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return File{}, fmt.Errorf("bookmarks.yml の解析に失敗しました: %w", err)
	}
	if file.Settings.Limit != nil && *file.Settings.Limit < 0 {
		return File{}, errors.New("bookmarks.yml の settings.limit は 0 以上で指定してください")
	}
	if file.Users == nil {
		file.Users = make(map[string]User)
	}
	return file, nil
}

// Add appends new URLs to a user's queue while skipping empty, queued, and
// completed URLs.
func (f *File) Add(user string, urls []string) int {
	if f.Users == nil {
		f.Users = make(map[string]User)
	}

	entry := f.Users[user]
	seen := make(map[string]struct{}, len(entry.Bookmarks)+len(entry.Completed))
	for _, url := range entry.Bookmarks {
		seen[url] = struct{}{}
	}
	for _, url := range entry.Completed {
		seen[url] = struct{}{}
	}

	added := 0
	for _, url := range urls {
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		entry.Bookmarks = append(entry.Bookmarks, url)
		seen[url] = struct{}{}
		added++
	}
	f.Users[user] = entry
	return added
}

// Queued returns a copy of the queued URLs for the user.
func (f File) Queued(user string) []string {
	entry := f.Users[user]
	return append([]string(nil), entry.Bookmarks...)
}

// CompleteFirst moves the first count queued URLs to the completed history.
func (f *File) CompleteFirst(user string, count int) {
	if count <= 0 {
		return
	}
	entry := f.Users[user]
	if count > len(entry.Bookmarks) {
		count = len(entry.Bookmarks)
	}

	completed := entry.Bookmarks[:count]
	seen := make(map[string]struct{}, len(entry.Completed)+len(completed))
	for _, url := range entry.Completed {
		seen[url] = struct{}{}
	}
	for _, url := range completed {
		if _, ok := seen[url]; ok {
			continue
		}
		entry.Completed = append(entry.Completed, url)
		seen[url] = struct{}{}
	}
	if len(entry.Completed) > maxCompletedURLs {
		entry.Completed = append([]string(nil), entry.Completed[len(entry.Completed)-maxCompletedURLs:]...)
	}

	if count == len(entry.Bookmarks) {
		entry.Bookmarks = nil
	} else {
		entry.Bookmarks = append([]string(nil), entry.Bookmarks[count:]...)
	}
	f.Users[user] = entry
}

// SaveAtomic writes bookmarks.yml through a temporary file and atomic rename.
func SaveAtomic(path string, file File) error {
	if file.Users == nil {
		file.Users = make(map[string]User)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return fmt.Errorf("bookmarks.yml の生成に失敗しました: %w", err)
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("bookmarks.yml の出力ディレクトリ作成に失敗しました: %w", err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("bookmarks.yml 一時ファイルの作成に失敗しました: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("bookmarks.yml 一時ファイルの書き込みに失敗しました: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("bookmarks.yml 一時ファイルのクローズに失敗しました: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("bookmarks.yml の更新に失敗しました: %w", err)
	}
	removeTmp = false
	return nil
}
