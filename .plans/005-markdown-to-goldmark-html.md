# Markdown 経由で HTML 生成する計画

## 目的

本文抽出後の EPUB 生成では、常に go-readability で Markdown を生成し、その Markdown を goldmark で HTML に変換する仕様に統一する。

## 現状

- `internal/content` は `readability.ToMarkdown(article.Root)` と `readability.ToHTML(article.Root)` の両方を返している。
- `internal/cli` は `article.HTML` が空でない場合にそれを優先して EPUB 本文に使っている。
- そのため、取得元 HTML 由来の `style` 属性などが EPUB に残る場合がある。

## 修正案

1. `internal/cli` で `article.HTML` を優先する分岐をやめ、常に `article.Markdown` を `convert.MarkdownConverter` で HTML に変換する。
2. `internal/content` の `Article.HTML` と `readability.ToHTML` 経由の処理を削除する。
3. `normalizeCodeBlocks` が HTML 抽出専用になっている場合は削除する。
4. `README.md` の仕様説明を Markdown 経由の HTML 生成と同梱 CSS の既定適用に合わせる。
5. `internal/content` のテストは Markdown 生成を確認する内容に整理する。
6. `internal/convert` の既存テストでコードブロック変換を確認し、必要なら追加する。
7. `go test ./...` で検証する。

## 編集対象

- `internal/content/content.go`
- `internal/content/content_test.go`
- `internal/cli/cli.go`
- `README.md`
- 必要に応じて `internal/convert/convert_test.go`

## 注意点

- EPUB 本文に渡す HTML は goldmark の出力に限定する。
- `book.Write` の EPUB 生成処理は本文 HTML を受け取るだけなので、基本的には変更しない。
- コードフェンスが欠けたコード片は、通常の本文を誤ってコード化しないように複数行のコードらしい連続に限定して補正する。

## 実施結果

- `internal/content` から `Article.HTML` と `readability.ToHTML` 経由の処理を削除した。
- `internal/cli` で常に `article.Markdown` を goldmark で HTML に変換するようにした。
- HTML 抽出専用だった `normalizeCodeBlocks` 一式を削除した。
- `internal/content` のテストを Markdown 抽出の確認に整理した。
- `internal/convert` で区切り行が先頭に来た Markdown テーブルを補正するようにした。
- 上記のテーブル補正に対する回帰テストを追加した。
- `internal/convert` で空のコードフェンス直後に続くコード行をコードブロックへ補正するようにした。
- 上記のコードブロック補正に対する回帰テストを追加した。
- コードフェンス内の日本語文字列を含むコード行を本文扱いせず、コードブロックを分割しないようにした。
- 上記のコードブロック分割防止に対する回帰テストを追加した。
- `README.md` を Markdown 経由の HTML 生成と同梱 CSS の既定適用に合わせて修正した。
- 見出し判定の正規表現を使い回すようにし、変換時の不要な再コンパイルを避けた。
- `style.css` の `background-color` に有効な値を指定するようにした。
- コードフェンスがない Kotlin/Compose 風の複数行コード片をコードブロックに補正した。
- 上記の裸コードブロック補正に対する回帰テストを追加した。
- `go test ./...` が成功した。
