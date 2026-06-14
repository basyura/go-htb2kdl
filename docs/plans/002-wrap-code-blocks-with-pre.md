# コードブロックを pre タグで囲む修正計画

## 目的

Markdown から HTML へ変換する際、コードブロックが `pre` タグで囲まれることを明確に保証する。

## 当初の調査対象

- `internal/convert/convert.go`
- `internal/convert/convert_test.go`

## 実際の修正対象

- `internal/content/content.go`
- `internal/content/content_test.go`
- `go.mod`

## 修正案

1. 現在の `goldmark` 変換結果と正規化処理を確認し、コードブロックが `pre` タグなしで出力される経路があるか特定する。
2. 必要に応じて Markdown 正規化処理を調整し、コードブロックとして扱うべき入力が fenced code block として HTML 変換されるようにする。
3. コードブロック出力に `<pre><code...>` が含まれることをテストで確認する。
4. 既存の Markdown 変換テストを実行し、見出し、表、本文分割の挙動に影響がないことを確認する。

## 確認方法

```sh
go test ./internal/convert
```

## 実施結果

- `internal/content/content.go` で、抽出した HTML 内の複数行 `<code>` を `<pre>` で囲む正規化処理を追加した。
- 既存の `<pre><code>` とインライン `<code>` は変更しない。
- `internal/content/content_test.go` に、抽出経路と正規化関数のテストを追加した。

## 確認結果

```sh
go test ./...
```

成功。
