# EPUB 生成時に CSS でスタイル変更できるようにする計画

## 目的

EPUB 生成時に任意の CSS ファイルを指定できるようにし、生成される章の XHTML にスタイルを適用できるようにする。

## 現状

- CLI は `--user`、`--from`、`--out` を受け取り、`book.Write` に渡している。
- EPUB 生成は `internal/book/book.go` の `Write` と `renderChapter` に集約されている。
- 章 XHTML の `<head>` には CSS 参照がなく、EPUB 内にスタイルシートも追加していない。
- EPUB の妥当性調整として `normalizeEPUB` で `content.opf` と `toc.xhtml` を補正している。

## 修正案

1. CLI に `--css <path>` オプションを追加する。
   - 指定がない場合は従来どおり CSS なしで生成する。
   - 指定がある場合は CSS ファイルを読み込み、`book.Options` に渡す。

2. `book.Options` に CSS 内容を渡すフィールドを追加する。
   - EPUB 内の固定ファイル名は `style.css` とする。
   - CSS 内容が空の場合はスタイルシートを追加しない。

3. EPUB 生成時に CSS を同梱する。
   - `writer.AddContent("style.css", ...)` で EPUB に追加する方針とする。
   - 必要に応じて `normalizeEPUB` で OPF の media-type を `text/css` に補正する。

4. 章 XHTML から CSS を参照する。
   - CSS が指定された場合だけ `<head>` に
     `<link rel="stylesheet" type="text/css" href="style.css" />`
     を追加する。

5. テストを追加・更新する。
   - CSS 指定時に EPUB 内へ `epub/style.css` が含まれることを確認する。
   - 章 XHTML に stylesheet link が含まれることを確認する。
   - CSS 未指定時の既存挙動が壊れていないことを確認する。

6. README を更新する。
   - `--css` オプションの使用例を追記する。

## 確認方法

- `go test ./...`
- 必要に応じて生成 EPUB の zip 内容を確認する。

## 実施結果

- CLI に `--css <path>` を追加した。
- 指定 CSS を EPUB 内の `epub/style.css` として同梱するようにした。
- CSS 指定時のみ章 XHTML から `style.css` を参照するようにした。
- OPF manifest に `style.css` を `text/css` として追加するようにした。
- CSS 指定時と未指定時のテストを追加した。
- README に `--css` の使用例を追記した。
- `go test ./...` が成功した。

## 編集予定ファイル

- `internal/cli/cli.go`
- `internal/book/book.go`
- `internal/book/book_test.go`
- `README.md`
