# 全実装ファイルの関数・型説明追加計画

## 目的

テストを除く Go 実装ファイルの各関数、メソッド、型、主要なトップレベル要素に、役割が分かる説明を追加する。

## 修正案

- `defaultStylesheet` に、埋め込み CSS の用途を説明するコメントを追加する。
- `main` 関数に、CLI の起動処理とエラー終了の責務を説明するコメントを追加する。
- `internal/cli/cli.go` の各関数に、処理の入口、キュー処理、章生成、EPUB 生成、メール送信、引数解析などの役割を説明するコメントを追加する。
- `internal/cli/cli.go` の各型に、CLI オプションや実行設定としての役割を説明するコメントを追加する。
- `internal/cli/log.go` の各型と関数に、ランタイムログとローテーション処理の役割を説明するコメントを追加する。
- `internal/book`、`internal/bookmarks`、`internal/content`、`internal/convert`、`internal/hatena`、`internal/mail` の各型、関数、メソッドに短い説明コメントを追加する。
- テストファイルのテスト関数は対象外とする。
- 既存の処理内容は変更しない。

## 対象ファイル

- `main.go`
- `internal/cli/cli.go`
- `internal/cli/log.go`
- `internal/book/book.go`
- `internal/bookmarks/bookmarks.go`
- `internal/content/content.go`
- `internal/convert/convert.go`
- `internal/hatena/hatena.go`
- `internal/mail/mail.go`

## 確認方法

- `go test ./...`
