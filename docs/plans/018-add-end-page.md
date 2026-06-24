# 終了ページ追加計画

## 目的

生成される EPUB の最後に終了ページを追加し、ページ中央に `END` を表示する。

## 修正案

- `internal/book/book.go` に `end.xhtml` を追加する。
- すべての章を追加した後に `writer.AddContent("end.xhtml", ...)` を呼び出し、spine の最後に入るようにする。
- `renderEndPage` を追加し、XHTML 内の CSS で縦横中央に `END` を配置する。
- `internal/book/book_test.go` の `TestWrite` を拡張し、`content.opf` の manifest と spine、および `end.xhtml` の内容を検証する。

## 確認

- `go test ./...` を実行する。
