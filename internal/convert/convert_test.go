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

func TestMarkdownConverterRepairsLeadingDelimiterTable(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("以下の表です。\n\n| --- | --- | --- |\n|  | 変換元 |  |\n| `Instant` | `ZonedDateTime` | `PlainDateTime` |\n")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "<table>") {
		t.Fatalf("table was not parsed: %s", got)
	}
	if strings.Contains(got, "| --- | --- | --- |") {
		t.Fatalf("table delimiter remains: %s", got)
	}
	if !strings.Contains(got, "<code>Instant</code>") {
		t.Fatalf("table body was not preserved: %s", got)
	}
}

func TestMarkdownConverterRepairsEmptyFenceBeforeCode(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\n```\n// CounterViewModel.kt: 単一のStateFlowで管理し、ユーザー操作ごとにメソッドを定義\nclass CounterViewModel : ViewModel() {\nprivate val _state = MutableStateFlow(CounterUiState())\nval state: StateFlow<CounterUiState> = _state.asStateFlow()\n}\n\n本文です。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<pre><code></code></pre>") {
		t.Fatalf("empty code block remains: %s", got)
	}
	if !strings.Contains(got, "<pre><code>// CounterViewModel.kt") {
		t.Fatalf("code was not parsed as code block: %s", got)
	}
	if strings.Contains(got, "<!-- raw HTML omitted -->") {
		t.Fatalf("raw HTML omission remains: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose after code block was not preserved: %s", got)
	}
}

func TestMarkdownConverterKeepsJapaneseStringLiteralInCodeBlock(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\n// ViewがViewModelのプロパティを直接読み取ってToast表示を制御している\nval context = LocalContext.current\nButton(\n    onClick = {\n        viewModel.increment()\n        if (viewModel.currentCount == 10) {\n            Toast.makeText(context, \"10に到達しました\", Toast.LENGTH_SHORT).show()\n        }\n    }\n) {\n    Text(\"Increment\")\n}\nこのようにViewがViewModelの構造を知りすぎていました。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(got, "<pre><code>") != 1 {
		t.Fatalf("code block was split: %s", got)
	}
	if strings.Contains(got, "<p>) {") {
		t.Fatalf("code was parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "Toast.makeText") {
		t.Fatalf("code with Japanese string literal was not preserved: %s", got)
	}
	if !strings.Contains(got, "<p>このようにViewがViewModelの構造を知りすぎていました。</p>") {
		t.Fatalf("prose after code block was not preserved: %s", got)
	}
}

func TestMarkdownConverterRepairsBareMultilineCodeBlock(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("// 1つのユーザー操作に対してView側が複数メソッドを組み合わせて呼んでいる\nButton(\nonClick = {\nviewModel.increment()\nviewModel.checkLimit()\n}\n) {\nText(\"Increment\")\n}\n\n本文です。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<p>// 1つのユーザー操作") {
		t.Fatalf("bare code was parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "<pre><code>// 1つのユーザー操作") {
		t.Fatalf("bare code was not repaired as code block: %s", got)
	}
	if !strings.Contains(got, "viewModel.checkLimit()") {
		t.Fatalf("code body was not preserved: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose after bare code block was not preserved: %s", got)
	}
}
