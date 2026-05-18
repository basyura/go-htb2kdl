package convert

import (
	"strings"
	"testing"
)

func TestMarkdownConverterConvert(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("# Title\n\nbody")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<h1>Title</h1>") {
		t.Fatalf("missing heading: %s", got)
	}
	if !strings.Contains(got, "<p>body</p>") {
		t.Fatalf("missing paragraph: %s", got)
	}
}

func TestMarkdownConverterConvertGFMTable(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("| Name | Value |\n| --- | --- |\n| foo | bar |\n")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<table>") {
		t.Fatalf("missing table: %s", got)
	}
	if strings.Contains(got, "| Name | Value |") {
		t.Fatalf("table markdown remains: %s", got)
	}
}

func TestMarkdownConverterConvertFencedCodeBlock(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```go\nfmt.Println(\"hello\")\n```\n")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<pre><code class=\"language-go\">") {
		t.Fatalf("missing fenced code block: %s", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("code fence remains: %s", got)
	}
}

func TestMarkdownConverterSplitsTrailingTextAfterClosingFence(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\ncode\n``` 次の本文\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "```") {
		t.Fatalf("code fence remains: %s", got)
	}
	if !strings.Contains(got, "<p>次の本文</p>") {
		t.Fatalf("trailing text was not parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "<table>") {
		t.Fatalf("table after code fence was not parsed: %s", got)
	}
}

func TestMarkdownConverterTreatsFenceWithProseAsProse(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("``` 本文です\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "```") {
		t.Fatalf("code fence remains: %s", got)
	}
	if strings.Contains(got, "language-本文です") {
		t.Fatalf("prose was parsed as code fence info: %s", got)
	}
	if !strings.Contains(got, "<p>本文です</p>") {
		t.Fatalf("prose was not parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "<table>") {
		t.Fatalf("table after prose was not parsed: %s", got)
	}
}

func TestMarkdownConverterClosesFenceBeforeProseAndTable(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\ncode()\n本文です。\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "```") {
		t.Fatalf("code fence remains: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose was not parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "<table>") {
		t.Fatalf("table was not parsed: %s", got)
	}
}
