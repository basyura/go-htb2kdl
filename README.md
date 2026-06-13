# htb2kdl

任意のユーザーのはてなブックマークを取得し、各ページのメインコンテンツを
markdown 経由で html に変換して epub を生成し、kindle に送る。

## Todo

- [x] ファイル名に生成日時を設定
- [x] 一定の url 数に達したら epub 生成
- [x] メール送信

## 使い方

```sh
go run . --user <はてなID> --from <yyyyMMdd>
```

`--from` には `20260520` のような `yyyyMMdd` 形式の日付、または
`-2` のような相対日数を指定できる。相対日数は実行日を基準に n 日前の
0:00 を開始日として扱う。

```sh
go run . --user <はてなID> --from -2
```

EPUB には同梱の `style.css` が既定で適用される。
任意の CSS を適用する場合は `--css` で CSS ファイルを指定する。

```sh
go run . --user <はてなID> --from <yyyyMMdd> --css style.css
```

実行時のログは標準出力に出力される。また、実行ファイルと同じディレクトリの
`htb2kdl.log` にも出力される。ログファイルが 5MiB を超える場合は
`htb2kdl.log.1` から `htb2kdl.log.3` まで 3 世代でローテーションする。

`--send` を指定すると、生成した EPUB を Gmail の SMTP サーバー経由で
送信する。送信先と Gmail のアプリパスワードは `bookmarks.yml` の `mail`
に設定する。

```sh
go run . --user <はてなID> --from <yyyyMMdd> --send
```

### 定期実行向けの蓄積モード

`--limit` に 1 以上を指定すると、RSS から取得したブックマーク URL を
`bookmarks.yml` に蓄積する。蓄積 URL 数が
しきい値に達した場合、先頭からしきい値件数だけ EPUB 化し、生成に成功した
URL を `bookmarks` から `completed` へ移動する。

`--limit` の扱いは次の通り。

- 未指定または `0`: `bookmarks.yml` を使わず、取得したブックマークをすぐ EPUB 化する
- `1` 以上: `bookmarks.yml` に蓄積し、指定件数に達したらその件数だけ EPUB 化する
- 蓄積件数が `--limit` 未満の場合: EPUB は生成せず、`queued: <現在件数>/<limit>` を出力して終了する

`--file` で `bookmarks.yml` の位置を指定できる。未指定の場合は、`htb2kdl`
実行ファイルと同じディレクトリの `bookmarks.yml` を生成・更新する。

```sh
go build -o htb2kdl .
./htb2kdl --user <はてなID> --from <yyyyMMdd> --limit 5
./htb2kdl --user <はてなID> --from <yyyyMMdd> --limit 5 --file /path/to/bookmarks.yml
./htb2kdl --user basyura --from -2 --limit 10 --send
```

`bookmarks.yml` は次の形式で管理する。

```yaml
mail:
  from: "sender@gmail.com"
  to: "kindle@example.com"
  app_password: "xxxx xxxx xxxx xxxx"
users:
  tatsuya:
    bookmarks:
      - "https://example.com/article-1"
      - "https://example.com/article-2"
    completed:
      - "https://example.com/completed-1"
```

`completed` は EPUB 生成済み URL の履歴として使う。ユーザーごとに最新
100 件のみ保持し、100 件を超えた場合は古い URL から削除する。
保存期間による削除は行わない。

cron などから実行する場合は、`--out` を省略すると日時入りの EPUB ファイル名で
出力される。

## はてなブックマーク

認証無しで rss の url から取得する
- https://b.hatena.ne.jp/{user}/rss

## 該当サイトのメインコンテンツ取得

はてなブックマークに登録されているサイト (url) のメインコンテンツは
`github.com/mackee/go-readability` を使って取得し、markdown 形式に変換する。

## html 生成

go-readability を使って生成した markdown を `github.com/yuin/goldmark` で html 変換する。

## epub 生成

`github.com/raitucarp/epub` で epub を生成。

## kindle へ送信

`--send` を指定すると Gmail の SMTP サーバー `smtp.gmail.com:587` を使って
生成した EPUB を送信する。`mail.from` には Gmail アドレス、`mail.to` には
送信先メールアドレス、`mail.app_password` には Gmail のアプリパスワードを
設定する。
