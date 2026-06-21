package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/smtp"
	"os"
	"path/filepath"
)

const (
	gmailSMTPHost = "smtp.gmail.com"
	gmailSMTPAddr = gmailSMTPHost + ":587"
)

// Config holds Gmail SMTP sender, recipient, and app password settings.
type Config struct {
	From        string
	To          string
	AppPassword string
}

// EPUBMessage describes the email subject, body, and EPUB attachment path.
type EPUBMessage struct {
	Subject string
	Body    string
	Path    string
}

// SendEPUB builds and sends an EPUB email through Gmail SMTP.
func SendEPUB(ctx context.Context, cfg Config, msg EPUBMessage) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := BuildEPUBMessage(cfg, msg)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	auth := smtp.PlainAuth("", cfg.From, cfg.AppPassword, gmailSMTPHost)
	if err := smtp.SendMail(gmailSMTPAddr, auth, cfg.From, []string{cfg.To}, data); err != nil {
		return fmt.Errorf("EPUB のメール送信に失敗しました: %w", err)
	}
	return nil
}

// Validate checks that all required mail configuration fields are present.
func (cfg Config) Validate() error {
	if cfg.From == "" {
		return errors.New("mail.from を bookmarks.yml に設定してください")
	}
	if cfg.To == "" {
		return errors.New("mail.to を bookmarks.yml に設定してください")
	}
	if cfg.AppPassword == "" {
		return errors.New("mail.app_password を bookmarks.yml に設定してください")
	}
	return nil
}

// BuildEPUBMessage builds a MIME message with a text body and EPUB attachment.
func BuildEPUBMessage(cfg Config, msg EPUBMessage) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if msg.Path == "" {
		return nil, errors.New("添付する EPUB ファイルが指定されていません")
	}

	epub, err := os.ReadFile(msg.Path)
	if err != nil {
		return nil, fmt.Errorf("EPUB ファイルの読み込みに失敗しました: %w", err)
	}

	subject := msg.Subject
	if subject == "" {
		subject = "EPUB"
	}
	body := msg.Body
	if body == "" {
		body = "EPUB を送信します。"
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	headers := []string{
		"From: " + cfg.From,
		"To: " + cfg.To,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=" + writer.Boundary(),
		"",
	}
	for _, header := range headers {
		if _, err := fmt.Fprintf(&buf, "%s\r\n", header); err != nil {
			return nil, err
		}
	}

	textHeader := make(map[string][]string)
	textHeader["Content-Type"] = []string{`text/plain; charset="utf-8"`}
	textHeader["Content-Transfer-Encoding"] = []string{"base64"}
	textPart, err := writer.CreatePart(textHeader)
	if err != nil {
		return nil, fmt.Errorf("メール本文の生成に失敗しました: %w", err)
	}
	if err := writeBase64(textPart, []byte(body)); err != nil {
		return nil, fmt.Errorf("メール本文の生成に失敗しました: %w", err)
	}

	filename := filepath.Base(msg.Path)
	attachmentHeader := make(map[string][]string)
	attachmentHeader["Content-Type"] = []string{mime.FormatMediaType("application/epub+zip", map[string]string{"name": filename})}
	attachmentHeader["Content-Disposition"] = []string{mime.FormatMediaType("attachment", map[string]string{"filename": filename})}
	attachmentHeader["Content-Transfer-Encoding"] = []string{"base64"}
	attachment, err := writer.CreatePart(attachmentHeader)
	if err != nil {
		return nil, fmt.Errorf("添付ファイルの生成に失敗しました: %w", err)
	}
	if err := writeBase64(attachment, epub); err != nil {
		return nil, fmt.Errorf("添付ファイルの生成に失敗しました: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("メールの生成に失敗しました: %w", err)
	}
	return buf.Bytes(), nil
}

// writeBase64 writes base64-encoded data using CRLF line wrapping.
func writeBase64(w interface{ Write([]byte) (int, error) }, data []byte) error {
	encoder := base64.NewEncoder(base64.StdEncoding, &newLineWriter{w: w})
	if _, err := encoder.Write(data); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}

// newLineWriter wraps base64 output at 76 columns for MIME compatibility.
type newLineWriter struct {
	w      interface{ Write([]byte) (int, error) }
	column int
}

// Write writes bytes to the underlying writer while inserting CRLF line breaks.
func (w *newLineWriter) Write(data []byte) (int, error) {
	written := 0
	for _, b := range data {
		if w.column == 76 {
			if _, err := w.w.Write([]byte("\r\n")); err != nil {
				return written, err
			}
			w.column = 0
		}
		if _, err := w.w.Write([]byte{b}); err != nil {
			return written, err
		}
		w.column++
		written++
	}
	return written, nil
}
