# style.css 同梱と既定適用の計画

## 目的

リポジトリ直下の `style.css` を実行ファイルに同梱し、`--css` が指定されていない場合でも、その CSS を適用した EPUB を生成する。

## 現状

- `internal/cli/cli.go` は `--css` が指定された場合のみ `os.ReadFile` で CSS を読み込む。
- `internal/book/book.go` は `Options.Stylesheet` が空でない場合に `epub/style.css` を追加し、各章から参照させる。
- `internal/book/book_test.go` には CSS あり、CSS なしの EPUB 生成テストがある。

## 修正案

1. `internal/cli/cli.go` に `embed` を追加し、`../../style.css` は指定できないため、CLI パッケージ配下に埋め込み用 CSS を配置するか、埋め込み責務をルートパッケージ側へ寄せる。
2. 既存構成を崩さないため、`internal/cli` 配下に `style.css` のコピーを持つのではなく、ルートパッケージで `style.css` を埋め込み、`cli.Run` に既定 CSS を渡せる形に変更する。
3. `--css` が指定された場合は従来どおり指定ファイルを優先する。
4. `--css` が未指定の場合は同梱 CSS を `book.Write` の `Stylesheet` に渡す。
5. CLI の単体テストを追加または更新し、既定 CSS と `--css` 優先の挙動を確認する。
6. `go test ./...` で検証する。

## 編集対象

- `main.go`
- `internal/cli/cli.go`
- 必要に応じて `internal/cli` のテスト

## 注意点

- `embed` はパッケージディレクトリ外のファイルを直接埋め込めないため、ルートの `main` パッケージで `style.css` を埋め込む。
- `book.Write` の低レベル API では、空の `Stylesheet` なら CSS を入れない挙動を維持する。

## 実施結果

- `main.go` で `style.css` を `go:embed` し、`cli.Run` に既定 CSS として渡すようにした。
- `internal/cli/cli.go` に `WithDefaultStylesheet` と CSS 読み込み処理を追加した。
- `--css` 指定時は指定ファイルを優先し、未指定時は同梱 CSS を使うようにした。
- `internal/cli/cli_test.go` を追加し、既定 CSS、指定 CSS 優先、読み込み失敗を検証した。
- `go test ./...` が成功した。
