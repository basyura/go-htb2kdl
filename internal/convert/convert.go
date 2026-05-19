package convert

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	headingLineRegexp   = regexp.MustCompile(`^#{1,6}\s+`)
	listLineRegexp      = regexp.MustCompile(`^([-+*]|\d+[.)])\s+`)
	preCodeBlockRegexp  = regexp.MustCompile(`(?s)<pre><code([^>]*)>(.*?)</code></pre>`)
	simpleCodeFenceInfo = regexp.MustCompile(`^[A-Za-z0-9_+.#-]+$`)
)

type MarkdownConverter struct {
	md goldmark.Markdown
}

func NewMarkdownConverter() *MarkdownConverter {
	return &MarkdownConverter{
		md: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithRendererOptions(gmhtml.WithHardWraps(), gmhtml.WithXHTML()),
		),
	}
}

func (c *MarkdownConverter) Convert(markdown string) (string, error) {
	body, err := c.convertMarkdown(normalizeMarkdown(markdown))
	if err != nil {
		return "", err
	}
	body, err = c.repairMixedCodeBlocks(body)
	if err != nil {
		return "", err
	}
	return removeEmptyCodeBlocks(body), nil
}

func (c *MarkdownConverter) convertMarkdown(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := c.md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("Markdown から HTML への変換に失敗しました: %w", err)
	}
	return buf.String(), nil
}

func (c *MarkdownConverter) repairMixedCodeBlocks(body string) (string, error) {
	var repairErr error
	repaired := preCodeBlockRegexp.ReplaceAllStringFunc(body, func(block string) string {
		if repairErr != nil {
			return block
		}
		matches := preCodeBlockRegexp.FindStringSubmatch(block)
		if len(matches) != 3 {
			return block
		}
		codeAttrs := matches[1]
		codeText := html.UnescapeString(matches[2])
		prefix, suffix, ok := splitMixedCodeBlock(codeText)
		if !ok {
			return block
		}

		var b strings.Builder
		if strings.TrimSpace(prefix) != "" {
			b.WriteString("<pre><code")
			b.WriteString(codeAttrs)
			b.WriteString(">")
			b.WriteString(html.EscapeString(prefix))
			b.WriteString("</code></pre>\n")
		}
		converted, err := c.convertMarkdown(normalizeMarkdown(suffix))
		if err != nil {
			repairErr = err
			return block
		}
		b.WriteString(converted)
		return b.String()
	})
	if repairErr != nil {
		return "", repairErr
	}
	return repaired, nil
}

func splitMixedCodeBlock(codeText string) (string, string, bool) {
	lines := strings.Split(codeText, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !looksLikeMarkdownProse(trimmed) {
			continue
		}
		prefix := strings.TrimRight(strings.Join(lines[:i], "\n"), "\n")
		if prefix != "" {
			prefix += "\n"
		}
		suffix := strings.TrimLeft(strings.Join(lines[i:], "\n"), "\n")
		return prefix, suffix, true
	}
	return "", "", false
}

func looksLikeMarkdownProse(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if headingLineRegexp.MatchString(trimmed) || isTableRow(trimmed) || isListLine(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed)
}

func isListLine(trimmed string) bool {
	return listLineRegexp.MatchString(trimmed)
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

	out = normalizeEmptyCodeFences(out)
	out = normalizeShellCodeBlocks(out)
	out = normalizeBareCodeBlocks(out)
	out = normalizeIndentedProse(out)
	out = normalizeMalformedTables(out)
	return strings.Join(out, "\n")
}

func normalizeEmptyCodeFences(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if i+2 >= len(lines) || strings.TrimSpace(lines[i]) != "```" || strings.TrimSpace(lines[i+1]) != "```" || !looksLikeCode(strings.TrimSpace(lines[i+2])) {
			out = append(out, lines[i])
			continue
		}

		out = append(out, "```")
		i += 2
		for i < len(lines) && shouldKeepInRepairedCodeBlock(strings.TrimSpace(lines[i])) {
			out = append(out, lines[i])
			i++
		}
		out = append(out, "```")
		i--
	}
	return out
}

func shouldKeepInRepairedCodeBlock(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if headingLineRegexp.MatchString(trimmed) {
		return false
	}
	if isTableRow(trimmed) {
		return false
	}
	if strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed) {
		return false
	}
	return looksLikeCode(trimmed) || looksLikeCodeContinuation(trimmed)
}

func normalizeBareCodeBlocks(lines []string) []string {
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, lines[i])
			continue
		}
		if inFence || !looksLikeCodeStart(trimmed) {
			out = append(out, lines[i])
			continue
		}

		end := i
		for end < len(lines) && shouldKeepInRepairedCodeBlock(strings.TrimSpace(lines[end])) {
			end++
		}
		if end-i < 2 {
			out = append(out, lines[i])
			continue
		}

		out = append(out, "```")
		out = append(out, lines[i:end]...)
		out = append(out, "```")
		i = end - 1
	}
	return out
}

func normalizeShellCodeBlocks(lines []string) []string {
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, lines[i])
			continue
		}
		if inFence || !isShellComment(trimmed) || i+1 >= len(lines) || !looksLikeShellCommand(strings.TrimSpace(lines[i+1])) {
			out = append(out, lines[i])
			continue
		}

		out = append(out, "```")
		for i < len(lines) {
			current := strings.TrimSpace(lines[i])
			if current == "" || (!isShellComment(current) && !looksLikeShellCommand(current) && !looksLikeCommandOutput(current)) {
				break
			}
			out = append(out, lines[i])
			i++
		}
		out = append(out, "```")
		i--
	}
	return out
}

func isShellComment(trimmed string) bool {
	return strings.HasPrefix(trimmed, "# ")
}

func looksLikeShellCommand(trimmed string) bool {
	prefixes := []string{
		"curl ", "chmod ", "npm ", "mkdir ", "snarkjs ", "echo ", "./",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func looksLikeCommandOutput(trimmed string) bool {
	prefixes := []string{
		"[INFO]", "Error:", "template instances:", "non-linear constraints:",
		"private inputs:", "public inputs:",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func normalizeIndentedProse(lines []string) []string {
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, line)
			continue
		}
		if !inFence && isIndentedMarkdownProse(line, trimmed) {
			out = append(out, strings.TrimLeft(line, " \t"))
			continue
		}
		out = append(out, line)
	}
	return out
}

func isIndentedMarkdownProse(line, trimmed string) bool {
	if trimmed == "" || !isMarkdownCodeIndent(line) {
		return false
	}
	if headingLineRegexp.MatchString(trimmed) || isTableRow(trimmed) || isListLine(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed)
}

func removeEmptyCodeBlocks(body string) string {
	return preCodeBlockRegexp.ReplaceAllStringFunc(body, func(block string) string {
		matches := preCodeBlockRegexp.FindStringSubmatch(block)
		if len(matches) != 3 {
			return block
		}
		if strings.TrimSpace(html.UnescapeString(matches[2])) == "" {
			return ""
		}
		return block
	})
}

func isMarkdownCodeIndent(line string) bool {
	spaceWidth := 0
	for _, r := range line {
		switch r {
		case ' ':
			spaceWidth++
		case '\t':
			spaceWidth += 4
		default:
			return spaceWidth >= 4
		}
	}
	return false
}

func looksLikeCodeContinuation(trimmed string) bool {
	if strings.HasPrefix(trimmed, "# ") && !looksLikeMarkdownHeading(trimmed) {
		return true
	}
	codePrefixes := []string{
		"@", ".", "private ", "public ", "protected ", "internal ", "override ",
		"data ", "sealed ", "object ", "interface ", "enum ", "companion ",
	}
	for _, prefix := range codePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return strings.ContainsAny(trimmed, "{}();:")
}

func looksLikeCodeStart(trimmed string) bool {
	if looksLikeCode(trimmed) {
		return true
	}
	return looksLikeCodeContinuation(trimmed) && !looksLikeMarkdownProse(trimmed)
}

func looksLikeMarkdownHeading(trimmed string) bool {
	if !headingLineRegexp.MatchString(trimmed) {
		return false
	}
	text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	return strings.ContainsAny(text, "。、ですます") || strings.ContainsAny(text, "ぁ-んァ-ヶ一-龠")
}

func normalizeMalformedTables(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if isTableDelimiterRow(line) && (i == 0 || !isTableRow(lines[i-1])) && i+1 < len(lines) && isTableRow(lines[i+1]) && !isTableDelimiterRow(lines[i+1]) {
			out = append(out, lines[i+1], line)
			i++
			continue
		}
		out = append(out, line)
	}
	return out
}

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

func isTableDelimiterRow(line string) bool {
	if !isTableRow(line) {
		return false
	}
	cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		cell = strings.Trim(cell, ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func shouldCloseFenceBefore(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if looksLikeMarkdownHeading(trimmed) {
		return true
	}
	if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
		return true
	}
	if strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed) {
		return true
	}
	return false
}

func looksLikeCode(trimmed string) bool {
	codePrefixes := []string{
		"//", "/*", "*", "}", "{", ")", "]", "return ", "if ", "for ",
		"while ", "switch ", "case ", "class ", "fun ", "func ", "val ", "var ",
		"let ", "const ", "import ", "package ", "type ", "domain ", "listen ",
		"upstream ", "A = ", "AAAA = ",
	}
	if strings.HasPrefix(trimmed, "# ") && !looksLikeMarkdownHeading(trimmed) {
		return true
	}
	for _, prefix := range codePrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return strings.Contains(trimmed, "=") && !strings.Contains(trimmed, "。")
}
