# --send オプションによる EPUB メール送信

## 目的

`--send` オプションが指定された場合、生成した EPUB を Gmail の SMTP
サーバー経由で設定済みの送信先メールアドレスへ送る。

送信先メールアドレスと Gmail のアプリパスワードは `bookmarks.yml`
に記載済みとする。

## 修正案

1. CLI オプションに `--send` を追加する。
   - 未指定時は従来通り EPUB 生成のみ行う。
   - 指定時は EPUB 生成成功後にメール送信する。
2. `bookmarks.yml` の読み込み対象にメール送信設定を追加する。
   - 例:

```yaml
mail:
  from: "sender@gmail.com"
  to: "kindle@example.com"
  app_password: "xxxx xxxx xxxx xxxx"
users:
  tatsuya:
    bookmarks: []
    completed: []
```

3. SMTP 送信用の小さな内部パッケージを追加する。
   - Gmail SMTP は `smtp.gmail.com:587` を使用する。
   - EPUB は添付ファイルとして送る。
   - 標準ライブラリの `net/smtp` と MIME 生成で実装し、依存を増やさない。
4. EPUB 生成処理は生成したファイルパスを返すように調整する。
   - `--send` の場合だけ、そのパスを送信用に使う。
5. `--send` 指定時の設定不足や送信失敗は日本語のエラーで返す。
6. メール送信に成功した場合は、生成済み EPUB ファイルを削除する。
   - 送信失敗時は EPUB ファイルを残す。
   - 削除失敗時は日本語のエラーで返す。
7. README に `--send` と `bookmarks.yml` の設定例を追記する。
8. テストを追加する。
   - `--send` のパース。
   - メール設定の読み込み。
   - 添付メールの MIME 生成。
   - 送信成功後に使う EPUB 削除処理。

## コマンド例

```sh
go run . --user <はてなID> --from <yyyyMMdd> --send
```

## 想定編集ファイル

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `internal/bookmarks/bookmarks.go`
- `internal/bookmarks/bookmarks_test.go`
- `internal/mail` 配下の新規ファイル
- `README.md`

## 確認方法

```sh
go test ./...
```

実 SMTP 送信は外部サービス依存のため自動テストでは行わず、送信処理の入力検証と
MIME 生成を中心に確認する。

## 実装状況

- `--send` オプションを追加済み。
- `bookmarks.yml` の `mail` 設定を読み込む処理を追加済み。
- Gmail SMTP で EPUB を添付送信する `internal/mail` を追加済み。
- メール送信成功後に EPUB ファイルを削除する処理を追加済み。
- README に利用例と設定例を追記済み。
- `go test ./...` 実行済み。
