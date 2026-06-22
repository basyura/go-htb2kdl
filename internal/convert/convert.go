package convert

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
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

// MarkdownConverter converts extracted Markdown into XHTML-compatible HTML.
type MarkdownConverter struct {
	md goldmark.Markdown
}

// NewMarkdownConverter creates a converter with GFM support and XHTML output.
func NewMarkdownConverter() *MarkdownConverter {
	return &MarkdownConverter{
		md: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithRendererOptions(gmhtml.WithHardWraps(), gmhtml.WithXHTML()),
		),
	}
}

// Convert normalizes Markdown, renders it to HTML, and repairs malformed code
// blocks produced by extraction.
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

// convertMarkdown renders Markdown to HTML using the configured goldmark
// renderer.
func (c *MarkdownConverter) convertMarkdown(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := c.md.Convert([]byte(markdown), &buf); err != nil {
		return "", fmt.Errorf("Markdown から HTML への変換に失敗しました: %w", err)
	}
	return buf.String(), nil
}

// repairMixedCodeBlocks splits code blocks that accidentally contain prose back
// into code and Markdown sections.
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

// splitMixedCodeBlock separates the code prefix from the prose suffix in a
// malformed mixed code block.
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

// looksLikeMarkdownProse reports whether a trimmed line appears to be prose or
// Markdown structure rather than code.
func looksLikeMarkdownProse(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if headingLineRegexp.MatchString(trimmed) || isTableRow(trimmed) || isListLine(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed)
}

// isListLine reports whether a trimmed line starts with a Markdown list marker.
func isListLine(trimmed string) bool {
	return listLineRegexp.MatchString(trimmed)
}

// normalizeMarkdown repairs common malformed Markdown patterns from extracted
// article content.
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
	out = normalizeRawHTMLCodeBlocks(out)
	out = normalizeLineNumberCodeTables(out)
	out = normalizeBareCodeBlocks(out)
	out = normalizeIndentedProse(out)
	out = normalizeMalformedTables(out)
	return strings.Join(out, "\n")
}

// normalizeEmptyCodeFences fills empty fences with following code-like lines
// when extraction split the fence incorrectly.
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

// shouldKeepInRepairedCodeBlock reports whether a line should remain inside a
// reconstructed code block.
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

// normalizeBareCodeBlocks wraps consecutive code-like lines that were extracted
// without Markdown fences.
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

// normalizeShellCodeBlocks wraps shell command examples that start with comment
// lines and continue with command output.
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

// isShellComment reports whether a line looks like a shell script comment.
func isShellComment(trimmed string) bool {
	return strings.HasPrefix(trimmed, "# ")
}

// looksLikeShellCommand reports whether a line starts with a known shell command
// prefix.
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

// looksLikeCommandOutput reports whether a line looks like output from a shell
// command example.
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

// normalizeIndentedProse removes code indentation from lines that are actually
// Markdown prose.
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

// isIndentedMarkdownProse reports whether an indented line should be treated as
// prose rather than a code block.
func isIndentedMarkdownProse(line, trimmed string) bool {
	if trimmed == "" || !isMarkdownCodeIndent(line) {
		return false
	}
	if headingLineRegexp.MatchString(trimmed) || isTableRow(trimmed) || isListLine(trimmed) {
		return true
	}
	return strings.ContainsAny(trimmed, "。、ですます") && !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed)
}

// removeEmptyCodeBlocks removes rendered pre/code blocks that contain no
// meaningful content.
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

// isMarkdownCodeIndent reports whether a line has Markdown's four-space code
// indentation.
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

// looksLikeCodeContinuation reports whether a trimmed line can continue an
// existing code block.
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

// looksLikeCodeStart reports whether a trimmed line can start a reconstructed
// bare code block.
func looksLikeCodeStart(trimmed string) bool {
	if looksLikeCode(trimmed) {
		return true
	}
	return looksLikeCodeContinuation(trimmed) && !looksLikeMarkdownProse(trimmed)
}

// looksLikeMarkdownHeading reports whether a heading-like line appears to be
// article text instead of a code comment.
func looksLikeMarkdownHeading(trimmed string) bool {
	if !headingLineRegexp.MatchString(trimmed) {
		return false
	}
	text := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	return strings.ContainsAny(text, "。、ですます") || strings.ContainsAny(text, "ぁ-んァ-ヶ一-龠")
}

// normalizeMalformedTables repairs tables whose delimiter row appears before
// the header row.
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

// normalizeRawHTMLCodeBlocks wraps raw HTML examples as fenced code blocks and
// removes a following duplicated line-number display table when present.
func normalizeRawHTMLCodeBlocks(lines []string) []string {
	out := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			out = append(out, lines[i])
			continue
		}
		if inFence || !looksLikeRawHTMLCodeLine(trimmed) {
			out = append(out, lines[i])
			continue
		}

		end := i
		for end < len(lines) && looksLikeRawHTMLCodeLine(strings.TrimSpace(lines[end])) {
			end++
		}
		if end-i < 2 {
			out = append(out, lines[i])
			continue
		}

		out = append(out, "```html")
		out = append(out, lines[i:end]...)
		out = append(out, "```")
		if next, ok := consumeDuplicatedLineNumberDisplay(lines, end); ok {
			i = next - 1
			continue
		}
		i = end - 1
	}
	return out
}

// consumeDuplicatedLineNumberDisplay skips a line-number display table that
// duplicates the raw HTML code example immediately before it.
func consumeDuplicatedLineNumberDisplay(lines []string, start int) (int, bool) {
	i := start
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !isTwoColumnDelimiterRow(lines[i]) {
		return start, false
	}
	i++

	sawNumber := false
	sawCode := false
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if looksLikeMarkdownHeading(trimmed) || isListLine(trimmed) {
			break
		}
		if strings.ContainsAny(trimmed, "。、ですます") && !looksLikeLooseHTMLCodeDisplayLine(trimmed) {
			break
		}
		if looksLikeStandaloneLineNumber(strings.Trim(trimmed, "| ")) {
			sawNumber = true
			i++
			continue
		}
		if strings.Contains(trimmed, "|") {
			for _, part := range strings.Split(trimmed, "|") {
				part = strings.TrimSpace(part)
				if looksLikeStandaloneLineNumber(part) {
					sawNumber = true
				}
				if looksLikeLooseHTMLCodeDisplayLine(part) {
					sawCode = true
				}
			}
			i++
			continue
		}
		if looksLikeLooseHTMLCodeDisplayLine(trimmed) {
			sawCode = true
			i++
			continue
		}
		break
	}
	if !sawNumber || !sawCode {
		return start, false
	}
	return i, true
}

func looksLikeRawHTMLCodeLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">")
}

func looksLikeLooseHTMLCodeDisplayLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	return looksLikeHTMLTagLine(trimmed) ||
		(strings.HasPrefix(trimmed, "< ") && strings.Contains(trimmed, " >")) ||
		(strings.Contains(trimmed, "< /") && strings.Contains(trimmed, " >"))
}

// normalizeLineNumberCodeTables converts extracted line-number code tables into
// fenced code blocks before GFM table parsing sees them.
func normalizeLineNumberCodeTables(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		code, end, ok := parseLineNumberCodeTable(lines, i)
		if !ok {
			out = append(out, lines[i])
			continue
		}
		out = append(out, "```")
		out = append(out, code...)
		out = append(out, "```")
		i = end - 1
	}
	return out
}

// parseLineNumberCodeTable parses a two-column table whose first column is
// only line numbers and whose second column is source code.
func parseLineNumberCodeTable(lines []string, start int) ([]string, int, bool) {
	if start >= len(lines) || !isTwoColumnDelimiterRow(lines[start]) {
		return nil, start, false
	}
	i := start + 1
	firstCode, next, ok := parseLineNumberCodeStart(lines, i)
	if !ok {
		return nil, start, false
	}
	numberLines := append([]string{}, firstCode.numbers...)
	codeLines := append([]string{}, firstCode.code...)
	i = next

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			break
		}
		if isTableRow(trimmed) || looksLikeMarkdownHeading(trimmed) || isListLine(trimmed) {
			break
		}
		if looksLikeStandaloneLineNumber(trimmed) {
			numberLines = append(numberLines, trimmed)
			i++
			continue
		}
		if !looksLikeCode(trimmed) && !looksLikeCodeContinuation(trimmed) && !looksLikeHTMLTagLine(trimmed) {
			break
		}
		codeLines = append(codeLines, strings.TrimRight(lines[i], " \t"))
		i++
	}

	if len(numberLines) < 2 || len(codeLines) < 2 || !isConsecutiveLineNumbers(numberLines) {
		return nil, start, false
	}
	return codeLines, i, true
}

type lineNumberCodeRow struct {
	numbers []string
	code    []string
}

// splitLineNumberCodeRow splits the first row of a malformed line-number code
// table into line-number and code cells.
func splitLineNumberCodeRow(line string) (lineNumberCodeRow, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(line), `\`)
	cells := strings.Split(strings.Trim(trimmed, "|"), "|")
	if len(cells) != 2 {
		return lineNumberCodeRow{}, false
	}
	numbers := splitNonEmptyLines(cells[0])
	if len(numbers) == 0 {
		return lineNumberCodeRow{}, false
	}
	for _, number := range numbers {
		if !looksLikeStandaloneLineNumber(number) {
			return lineNumberCodeRow{}, false
		}
	}
	code := splitNonEmptyLines(cells[1])
	if len(code) == 0 {
		return lineNumberCodeRow{}, false
	}
	return lineNumberCodeRow{numbers: numbers, code: code}, true
}

// parseLineNumberCodeStart parses the first row even when the line-number cell
// was split across multiple Markdown lines.
func parseLineNumberCodeStart(lines []string, start int) (lineNumberCodeRow, int, bool) {
	if start >= len(lines) {
		return lineNumberCodeRow{}, start, false
	}
	if isTableRow(lines[start]) {
		row, ok := splitLineNumberCodeRow(lines[start])
		return row, start + 1, ok
	}

	var numbers []string
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			return lineNumberCodeRow{}, start, false
		}
		line = strings.TrimPrefix(line, `\|`)
		line = strings.TrimPrefix(line, "|")
		line = strings.TrimSpace(line)
		left, right, ok := strings.Cut(line, "|")
		if !ok {
			if !looksLikeStandaloneLineNumber(line) {
				return lineNumberCodeRow{}, start, false
			}
			numbers = append(numbers, line)
			continue
		}

		left = strings.TrimSpace(left)
		if !looksLikeStandaloneLineNumber(left) {
			return lineNumberCodeRow{}, start, false
		}
		numbers = append(numbers, left)
		code := splitNonEmptyLines(strings.Trim(right, " |"))
		if len(code) == 0 {
			return lineNumberCodeRow{}, start, false
		}
		return lineNumberCodeRow{numbers: numbers, code: code}, i + 1, true
	}
	return lineNumberCodeRow{}, start, false
}

func splitNonEmptyLines(value string) []string {
	parts := strings.Split(value, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func looksLikeStandaloneLineNumber(line string) bool {
	if line == "" {
		return false
	}
	for _, r := range line {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isConsecutiveLineNumbers(lines []string) bool {
	for i, line := range lines {
		number, err := strconv.Atoi(line)
		if err != nil || number != i+1 {
			return false
		}
	}
	return true
}

func isTwoColumnDelimiterRow(line string) bool {
	if !isTableDelimiterRow(line) {
		return false
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(line), `\`)
	cells := strings.Split(strings.Trim(trimmed, "|"), "|")
	return len(cells) == 2
}

func looksLikeHTMLTagLine(trimmed string) bool {
	return (strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">")) ||
		(strings.HasPrefix(trimmed, "&lt;") && strings.Contains(trimmed, "&gt;"))
}

// isTableRow reports whether a line has Markdown table pipe delimiters.
func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, `\`)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

// isTableDelimiterRow reports whether a table row is the Markdown delimiter row.
func isTableDelimiterRow(line string) bool {
	if !isTableRow(line) {
		return false
	}
	trimmed := strings.TrimPrefix(strings.TrimSpace(line), `\`)
	cells := strings.Split(strings.Trim(trimmed, "|"), "|")
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

// shouldCloseFenceBefore reports whether a fence should be closed before a
// prose or table line.
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

// looksLikeCode reports whether a trimmed line matches common source-code
// patterns.
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
