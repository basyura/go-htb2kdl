# Repository Guidelines 作成計画

## 目的

このリポジトリの contributor guide として、ルートに `AGENTS.md` を作成する。

## 作成方針

- タイトルは `Repository Guidelines` とする。
- Markdown 見出しで構成する。
- 200〜400 words 程度の簡潔な英語ドキュメントにする。
- この Go CLI プロジェクトに固有の構成、コマンド、テスト、コミット方針を記載する。

## 記載予定内容

1. Project Structure & Module Organization
   - `main.go`
   - `internal/cli`
   - `internal/hatena`
   - `internal/content`
   - `internal/convert`
   - `internal/book`
   - `style.css`

2. Build, Test, and Development Commands
   - `go test ./...`
   - `go build ./...`
   - `go run . --user <user> --from <yyyyMMdd>`
   - `gofmt -w <files>`

3. Coding Style & Naming Conventions
   - Go 標準の `gofmt`
   - パッケージ単位の責務分離
   - エラー文言の扱い

4. Testing Guidelines
   - 標準 `testing` パッケージ
   - `*_test.go` と `TestXxx`
   - EPUB や HTML 出力の検証方針

5. Commit & Pull Request Guidelines
   - 既存履歴に合わせた短い命令形または日本語要約
   - 変更概要、テスト結果、関連 issue の記載

## 検証

- `AGENTS.md` の語数と構成を確認する。
- 必要に応じて内容を読み返し、リポジトリ実態と矛盾がないか確認する。
