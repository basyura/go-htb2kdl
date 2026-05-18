package convert

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var simpleCodeFenceInfo = regexp.MustCompile(`^[A-Za-z0-9_+.#-]+$`)

type MarkdownConverter struct {
	md goldmark.Markdown
}

func NewMarkdownConverter() *MarkdownConverter {
	return &MarkdownConverter{
		md: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithRendererOptions(gmhtml.WithXHTML()),
		),
	}
}

func (c *MarkdownConverter) Convert(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := c.md.Convert([]byte(normalizeMarkdown(markdown)), &buf); err != nil {
		return "", fmt.Errorf("Markdown から HTML への変換に失敗しました: %w", err)
	}
	return buf.String(), nil
}

func normalizeMarkdown(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence && shouldCloseFenceBefore(trimmed) {
			out = append(out, "```")
			inFence = false
		}
		if strings.HasPrefix(trimmed, "```") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			if rest == "" {
				out = append(out, line)
				inFence = !inFence
				continue
			}
			if inFence {
				out = append(out, "```", rest)
				inFence = false
				continue
			}
			if simpleCodeFenceInfo.MatchString(rest) {
				out = append(out, "```"+rest)
				inFence = true
				continue
			}
			out = append(out, rest)
			continue
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

func shouldCloseFenceBefore(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if regexp.MustCompile(`^#{1,6}\s+`).MatchString(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		return true
	}
	if strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) {
		return true
	}
	return false
}

func looksLikeCode(trimmed string) bool {
	codePrefixes := []string{
		"//", "# ", "/*", "*", "}", "{", ")", "]", "return ", "if ", "for ",
		"while ", "switch ", "case ", "class ", "fun ", "func ", "val ", "var ",
		"let ", "const ", "import ", "package ", "type ", "domain ", "listen ",
		"upstream ", "A = ", "AAAA = ",
	}
	for _, prefix := range codePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return strings.Contains(trimmed, "=") && !strings.Contains(trimmed, "。")
}
