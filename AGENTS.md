# Repository Guidelines

## Project Structure & Module Organization

This repository is a Go CLI that fetches Hatena Bookmark RSS entries, extracts article content, converts it through Markdown/HTML, and writes an EPUB.

- `main.go` wires the executable to `internal/cli`.
- `internal/cli` handles flags, orchestration, and user-facing errors.
- `internal/hatena` fetches and parses Hatena Bookmark RSS.
- `internal/content` extracts readable article content.
- `internal/convert` converts Markdown to HTML.
- `internal/book` builds and normalizes EPUB output.
- `style.css` is the default EPUB stylesheet.
- Tests live next to their packages as `*_test.go`.

## Build, Test, and Development Commands

- `go test ./...` runs all package tests.
- `go build ./...` checks that all packages compile.
- `go run . --user <hatena-id> --from <yyyyMMdd>` runs the CLI locally.
- `go run . --user <hatena-id> --from <yyyyMMdd> --css style.css` runs with an explicit stylesheet.
- `gofmt -w <files>` formats changed Go files before review.

Generated `.epub` files are local artifacts and should not be committed unless explicitly required.

## Coding Style & Naming Conventions

Use standard Go formatting and idioms. Keep package names short and lowercase, and keep responsibilities aligned with the existing package boundaries. Prefer small helper functions when behavior needs focused tests, as in CLI sorting or EPUB normalization.

Return wrapped errors with clear Japanese messages for user-facing failures. Avoid broad refactors when making feature changes; keep edits scoped to the behavior being changed.

## Testing Guidelines

Use Go's standard `testing` package. Name tests `TestXxx` and place them in the same package directory as the code under test. Favor table tests when adding multiple input/output cases.

For EPUB behavior, inspect generated zip entries or relevant XHTML/OPF content rather than only checking that a file exists. For HTML conversion, assert specific rendered fragments.

For EPUB validation, run `epubcheck <file.epub>` when available. Also verify that generated XHTML/OPF/NCX files are XML-well-formed, manifest references exist, and image references are embedded.

Run `go test ./...` before submitting changes.

## Commit & Pull Request Guidelines

Existing history uses short summaries in Japanese or English, for example `Markdown 経由の HTML 生成に統一` and `improved README.md`. Keep commit titles concise and focused on the behavioral change.

Pull requests should include a short description, test results such as `go test ./...`, and any notes about generated EPUB output or changed CLI behavior. Link related issues when available.
