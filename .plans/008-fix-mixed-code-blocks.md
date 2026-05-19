# コードブロックと本文の混在修正計画

## 背景

生成済み `basyura-20260329.epub` の `epub/chapter-001.xhtml` で、P.6 付近の本文・見出し・Markdown テーブルが `<pre><code>` の中に混入している。

## 原因仮説

`internal/convert/convert.go` の Markdown 正規化処理が、壊れたコードフェンスや裸の複数行コードを補正する際、本文へ戻る境界を一部判定できていない。

特に以下の境界が重要。

- 日本語本文だけの行
- Markdown 見出し行
- Markdown テーブル行
- 空行後に続く本文

## 修正案

1. `internal/convert/convert.go` のコードブロック継続判定を見直す。
2. 見出し・表・日本語本文が出た時点で、補正中のコードブロックを閉じる。
3. Kotlin の複数行コードや日本語文字列リテラルを誤って分断しないよう、既存のコード判定は維持する。
4. `internal/convert/convert_test.go` に P.6 相当の再現ケースを追加する。
5. `go test ./...` で既存挙動と追加テストを確認する。

## 影響範囲

- Markdown から HTML への変換処理
- EPUB の章 XHTML に出力される `<pre><code>` の範囲

## 確認事項

- 本文が `<pre><code>` に混入しないこと。
- Markdown 見出しと表が通常の HTML として変換されること。
- コードブロック内の Kotlin コードが途中で分断されないこと。

## 追加対応

- インデントされた Markdown リストをコードブロックにしない。
- 空の `<pre><code>` を出力しない。
- シェルコマンド内の `#` コメントを見出しとして扱わない。
- SVG 画像の OPF media-type を `image/svg+xml` にする。
- 記事内のルート相対リンクを元ページの origin 付き絶対 URL に変換する。
- `epubcheck` で EPUB 全体を検証する。
