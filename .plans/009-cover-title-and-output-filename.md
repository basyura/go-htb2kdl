# 表紙タイトルと出力ファイル名の日時化

## 目的

- 空白になっている EPUB 表紙に生成日とユーザー名を表示する。
- デフォルトの出力ファイル名を出力日時ベースに変更する。

## 期待する挙動

- 表紙には次の 3 行を表示する。
  - `はてブ`
  - `{user}`
  - `{yyyy/MM/dd HH:mm}`
- 表紙の次に、各ブックマークのタイトル一覧ページを表示する。
  - 各タイトルは対応する章へのリンクにする。
- `--out` が指定されていない場合のファイル名は次の形式にする。
  - `hateb-{user}-{出力日時:yyyyMMddHHmm}.epub`
- 例:
  - `hateb-basyura-202605181801.epub`
- `--out` が指定された場合は、従来どおり指定パスを優先する。

## 修正案

1. `internal/book/book.go`
   - `DefaultOutputPath` を `from` 基準ではなく出力日時基準で使える形に変更する。
   - ファイル名の先頭を `hateb-` にし、日時フォーマットを `200601021504` にする。
   - `blankCover` を表紙画像生成関数に変更し、画像中央付近に `はてブ`、ユーザー名、出力日時を 3 行で描画する。
   - EPUB の `Created` が未指定の場合でも、表紙とメタデータで同じ時刻を使う。
   - `cover.xhtml` の次に `bookmarks.xhtml` を追加し、各章へのリンク付きタイトル一覧を描画する。

2. `internal/cli/cli.go`
   - `created := time.Now()` を一度だけ取得する。
   - デフォルト出力パス生成と `book.Write` の `Created` に同じ `created` を渡す。

3. テスト
   - `internal/book/book_test.go` にデフォルト出力名の形式を確認するテストを追加する。
   - 既存の EPUB 生成テストでカバー画像が空白ではないこと、またはカバー画像生成関数単体で背景以外のピクセルがあることを確認する。
   - `bookmarks.xhtml` が spine で表紙の次、最初の章の前にあり、各章へのリンクを含むことを確認する。
   - 必要に応じて `internal/cli/cli_test.go` は既存テストの影響がない範囲で維持する。

## 確認方法

- `go test ./...`

## 実施状況

- `internal/book/book.go` のデフォルト出力名を `hateb-{user}-{yyyyMMddHHmm}.epub` に変更済み。
- `internal/cli/cli.go` で生成時刻を一度だけ取得し、出力名と EPUB の `Created` に共通利用するよう変更済み。
- EPUB 表紙に `はてブ`、ユーザー名、`yyyy/MM/dd HH:mm` を 3 行で描画する処理へ変更済み。
- 一部リーダーで 1 ページ目が空白になるため、spine の先頭に `cover.xhtml` を追加し、本文内の表紙ページとして cover 画像を表示するよう変更済み。
- cover image と本文内表紙ページが二重表示されるため、正規化時に OPF の cover-image 指定を外し、本文内の `cover.xhtml` のみを読書順の表紙として扱うよう変更済み。
- `cover.xhtml` の次に `bookmarks.xhtml` を追加し、各ブックマークタイトルから章へ移動できる一覧ページを追加済み。
- `internal/book/book_test.go` に出力名と表紙描画のテストを追加済み。
- `go test ./...` 実行済み。
