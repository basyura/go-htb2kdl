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

func TestMarkdownConverterConvertsSoftLineBreakToBR(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("AA\nBB")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "AA<br />\nBB") {
		t.Fatalf("soft line break was not converted to br: %s", got)
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

func TestMarkdownConverterDoesNotTreatIndentedProseAsCode(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("// CounterScreen.kt: Composable関数から直接Navigatorを呼び出して画面遷移\nButton(onClick = { navController.navigateSetting() }) {\n    Text(\"Setting\")\n}\n\n    方式が統一されていないため、新しい画面を実装する際にどの方式へ合わせるべきか判断しづらく、開発者ごとの実装のばらつきを招いていました。\n\n    ## 私たちのMVVMアーキテクチャの改善方針\n\n    | 課題 | 解決方針 |\n    | --- | --- |\n    | イベント通知と画面遷移の不統一 | イベント通知をChannelに統一する |\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<code>方式が統一されていないため") {
		t.Fatalf("prose was parsed as code: %s", got)
	}
	if !strings.Contains(got, "<p>方式が統一されていないため") {
		t.Fatalf("prose was not parsed as paragraph: %s", got)
	}
	if !strings.Contains(got, "<h2>私たちのMVVMアーキテクチャの改善方針</h2>") {
		t.Fatalf("heading was not parsed: %s", got)
	}
	if !strings.Contains(got, "<table>") {
		t.Fatalf("table was not parsed: %s", got)
	}
}

func TestMarkdownConverterDoesNotTreatIndentedListAsCode(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("## 目次\n\n    - [はじめに](#はじめに)\n    - [目次](#目次)\n      - [用語](#用語)\n    1. [まとめ](#まとめ)\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<pre><code>- [はじめに]") {
		t.Fatalf("list was parsed as code: %s", got)
	}
	if !strings.Contains(got, "<ul>") {
		t.Fatalf("unordered list was not parsed: %s", got)
	}
	if !strings.Contains(got, "<ol>") {
		t.Fatalf("ordered list was not parsed: %s", got)
	}
	if !strings.Contains(got, ">はじめに</a>") {
		t.Fatalf("list link was not parsed: %s", got)
	}
}

func TestMarkdownConverterClosesCodeBlockBeforeHeadingAfterCode(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\n// CounterScreen.kt: View側は単一のStateを購読するだけ\n@Composable\nfun CounterScreen(viewModel: CounterViewModel, /* ... */) {\n    val state by viewModel.state.collectAsStateWithLifecycle()\n\n    CounterScreenContent(\n        state = state,\n        onIncrement = viewModel::onIncrementClicked,\n        // ...\n    )\n}\n### ユーザー操作ごとのメソッド定義による責務の明確化\n\nUDFの原則に従い、ViewからのActionに反応してStateが更新されるシンプルな構造を考えました。\n\n```\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<code>### ユーザー操作") || strings.Contains(got, "<code>UDFの原則") {
		t.Fatalf("heading or prose was parsed as code: %s", got)
	}
	if !strings.Contains(got, "<h3>ユーザー操作ごとのメソッド定義による責務の明確化</h3>") {
		t.Fatalf("heading was not parsed: %s", got)
	}
	if !strings.Contains(got, "<p>UDFの原則に従い") {
		t.Fatalf("prose was not parsed: %s", got)
	}
}

func TestMarkdownConverterRepairsMixedCodeBlockAfterHTMLConversion(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("    // CounterScreen.kt: View側は単一のStateを購読するだけ\n    @Composable\n    fun CounterScreen(viewModel: CounterViewModel, /* ... */) {\n        val state by viewModel.state.collectAsStateWithLifecycle()\n    }\n    ### ユーザー操作ごとのメソッド定義による責務の明確化\n\n    UDFの原則に従い、ViewからのActionに反応してStateが更新されるシンプルな構造を考えました。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<code>### ユーザー操作") || strings.Contains(got, "<code>UDFの原則") {
		t.Fatalf("heading or prose remains in code block: %s", got)
	}
	if !strings.Contains(got, "CounterScreen.kt") {
		t.Fatalf("code prefix was not preserved: %s", got)
	}
	if !strings.Contains(got, "<h3>ユーザー操作ごとのメソッド定義による責務の明確化</h3>") {
		t.Fatalf("heading was not repaired: %s", got)
	}
	if !strings.Contains(got, "<p>UDFの原則に従い") {
		t.Fatalf("prose was not repaired: %s", got)
	}
}

func TestMarkdownConverterKeepsBareCodeWithJapaneseStringLiteral(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("LaunchedEffect(counter) {\nif (counter >= 10) {\nToast.makeText(context, \"10に到達しました\", Toast.LENGTH_SHORT).show()\n    }\n}\n\n本文です。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<p>LaunchedEffect") || strings.Contains(got, "<p>}</p>") {
		t.Fatalf("bare code was split into paragraphs: %s", got)
	}
	if !strings.Contains(got, "<pre><code>LaunchedEffect(counter)") {
		t.Fatalf("bare code was not parsed as code: %s", got)
	}
	if !strings.Contains(got, "10に到達しました") {
		t.Fatalf("Japanese string literal was not preserved: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose after code was not preserved: %s", got)
	}
}

func TestMarkdownConverterRemovesEmptyCodeBlocks(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("```\n```\n\n本文です。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<pre><code>") {
		t.Fatalf("empty code block remains: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose was not preserved: %s", got)
	}
}

func TestMarkdownConverterKeepsShellCommandsWithCommentsAsCode(t *testing.T) {
	converter := NewMarkdownConverter()
	got, err := converter.Convert("# circom 2.x（回路コンパイラ）\ncurl -sL https://example.com/circom -o circom\nchmod +x circom\n# snarkjs（証明ツールキット）\nnpm install -g snarkjs\n\n本文です。\n")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(got, "<h1>circom") || strings.Contains(got, "<p>curl -sL") {
		t.Fatalf("shell commands were not parsed as code: %s", got)
	}
	if !strings.Contains(got, "<pre><code># circom 2.x") {
		t.Fatalf("shell code block missing: %s", got)
	}
	if !strings.Contains(got, "npm install -g snarkjs") {
		t.Fatalf("shell command was not preserved: %s", got)
	}
	if !strings.Contains(got, "<p>本文です。</p>") {
		t.Fatalf("prose after shell code was not preserved: %s", got)
	}
}
