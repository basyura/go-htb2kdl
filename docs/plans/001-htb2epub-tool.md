# はてなブックマーク EPUB 生成ツール実装計画

## 目的

README.md の方針に従い、指定したはてな ID のブックマーク RSS を取得し、
各 URL のメインコンテンツを抽出して EPUB にまとめる CLI ツールを作成する。
Kindle 送信は README.md 上で別途検討とされているため、今回の主対象は
EPUB ファイル生成までとする。

## 前提

- CLI は `go run . --user <はてなID> --from <yyyyMMdd>` で実行する。
- はてなブックマークは認証なしで
  `https://b.hatena.ne.jp/{user}/rss` から取得する。
- メインコンテンツ抽出には `github.com/mackee/go-readability` を使う。
- Markdown から HTML への変換には `github.com/yuin/goldmark` を使う。
- EPUB 生成には `github.com/raitucarp/epub` を使う。
- Kindle 送信機能は今回の実装対象外とし、必要なら後続計画で扱う。

## 実装方針

`main.go` に処理をまとめず、CLI の入口だけを `main.go` に置く。
RSS 取得・解析、本文抽出、HTML 変換、EPUB 生成は責務ごとに
適切なパッケージへ分割する。

1. CLI 引数を実装する
   - `--user` を必須にする。
   - `--from` を `yyyyMMdd` 形式で受け取り、指定日以降のブックマークだけを対象にする。
   - EPUB 出力先はまず既定値を用意し、必要なら `--out` を追加する。

2. はてなブックマーク RSS を取得・解析する
   - RSS URL を組み立てて HTTP GET する。
   - RSS item からタイトル、URL、ブックマーク日時を取り出す。
   - `--from` より古い item を除外する。
   - ネットワークエラー、RSS 解析エラー、対象なしを明示的に扱う。

3. 各ページのメインコンテンツを抽出する
   - ブックマーク URL の HTML を取得する。
   - `go-readability` で本文、タイトル、サイト名などを抽出する。
   - 取得失敗した URL は全体を止めず、警告として記録して次に進む。

4. Markdown と HTML を生成する
   - 抽出本文を Markdown として扱い、`goldmark` で HTML に変換する。
   - EPUB 内の章として扱いやすい構造に整える。
   - 元 URL、タイトル、取得日などのメタ情報を章内に含める。

5. EPUB を生成する
   - 書籍タイトル、著者、章を設定する。
   - 章の順序は RSS の新しい順を基本にする。
   - 出力ファイル名はユーザー名と日付範囲から決める。

6. テストと検証を追加する
   - RSS 解析、日付フィルタ、HTML 変換まわりのユニットテストを追加する。
   - 外部 HTTP に依存する箇所は `httptest` で検証する。
   - 最後に `go test ./...` を実行する。

## 変更予定ファイル

- `main.go`
  - CLI エントリポイントのみを置き、実処理は内部パッケージへ委譲する。
- `go.mod`
  - README.md に記載された依存ライブラリを追加する。
- `go.sum`
  - 依存追加に伴い生成する。
- 追加予定の Go パッケージ
  - `internal/cli`
    - CLI 引数解析、実行フロー、ユーザー向けエラー表示を担当する。
  - `internal/hatena`
    - はてなブックマーク RSS の取得・解析、日付フィルタを担当する。
  - `internal/content`
    - 対象 URL の HTML 取得とメインコンテンツ抽出を担当する。
  - `internal/convert`
    - Markdown から HTML への変換を担当する。
  - `internal/book`
    - EPUB の章構成とファイル生成を担当する。
- 追加予定のテストファイル
  - RSS 解析、フィルタ、変換処理を中心に検証する。

## 確認事項

- Kindle 送信は今回の実装範囲から外す。
- EPUB 出力先オプション `--out` は使い勝手向上のため追加する想定。
- 実ページの抽出品質はサイトごとに差が出るため、取得失敗や抽出失敗を
  ログに残しつつ処理を継続する。

## 次の作業

この計画で問題なければ、以下の順で具体的な修正を進める。

1. 現在の依存関係で使える API を確認する。
2. CLI と RSS 取得・解析の最小実装を追加する。
3. メインコンテンツ抽出、HTML 変換、EPUB 生成を接続する。
4. テストを追加して `go test ./...` で検証する。

## 実装結果

- `main.go` は CLI 入口のみとし、実処理を `internal/cli` に委譲した。
- `internal/hatena` で RSS 取得・解析と `--from` の日付フィルタを実装した。
- `internal/content` で対象 URL の HTML 取得と go-readability による本文抽出を実装した。
- `internal/convert` で goldmark による Markdown から XHTML 互換 HTML への変換を実装した。
- `internal/book` で EPUB メタデータ、章、目次、カバー、出力処理を実装した。
- `--out` オプションを追加し、未指定時は `<user>-<yyyyMMdd>.epub` を出力する。
- RSS 解析、日付フィルタ、本文抽出、HTML 変換、EPUB 書き込みのテストを追加した。
- `go test ./...` が成功することを確認した。

## EPUB ビューアー互換性修正

- `go run main.go --user basyura --from 20260401` で生成した EPUB を確認した。
- `content.opf` に `dc:` 名前空間宣言がなく、XML として不正だったため補正した。
- `toc.xhtml` に `epub:type` 用の名前空間宣言がなかったため補正した。
- EPUB 仕様に合わせて `mimetype` を ZIP の先頭かつ無圧縮に再配置した。
- `spine toc` が実際の NCX manifest id と一致していなかったため `toc` に補正した。
- 修正後の生成物で `xmllint` による XML 整形式チェックが通ることを確認した。

## Markdown 変換修正

- goldmark の標準設定では GFM テーブル記法が HTML テーブルに変換されないため、
  `extension.GFM` を有効化した。
- テーブル記法が `<table>` に変換されることをテストに追加した。
- fenced code block が `<pre><code>` に変換され、バッククォート記法が残らないことを
  テストに追加した。
- `go run main.go --user basyura --from 20260401` で EPUB を再生成し、
  XML 整形式チェックが通ることを確認した。
- `go-readability` の Markdown 出力がコードブロックに本文やテーブルを巻き込む
  ケースがあったため、EPUB 本文は抽出済み HTML を優先して使い、
  Markdown 変換はフォールバックにした。
- `--user basyura --from 20251101` で生成した EPUB を検査し、章内に
  バッククォート記法、未変換のテーブル記法、壊れたコード言語属性が
  残らないことを確認した。
