# htb2kdl

任意のユーザーのはてなブックマークを取得し、各ページのメインコンテンツを
markdown 経由で html に変換して epub を生成し、kindle に送る。

## Todo

- [ ] ファイル名に送信日時を設定
- [ ] 一定の url 数に達したらメール送信
  - settings.json 等で管理。ファイル数、チェック日、送信候補、最終送信対象など。

## 使い方

```sh
go run . --user <はてなID> --from <yyyyMMdd>
```

EPUB には同梱の `style.css` が既定で適用される。
任意の CSS を適用する場合は `--css` で CSS ファイルを指定する。

```sh
go run . --user <はてなID> --from <yyyyMMdd> --css style.css
```

### 定期実行向けの蓄積モード

`--limit` に 1 以上を指定すると、RSS から取得したブックマーク URL を
実行ファイルと同じディレクトリの `bookmarks.yml` に蓄積する。蓄積 URL 数が
しきい値に達した場合、先頭からしきい値件数だけ EPUB 化し、生成に成功した
URL を `bookmarks` から `completed` へ移動する。

```sh
go build -o htb2kdl .
./htb2kdl --user <はてなID> --from <yyyyMMdd> --limit 5
```

`bookmarks.yml` は次の形式で管理する。

```yaml
users:
  tatsuya:
    bookmarks:
      - "https://example.com/article-1"
      - "https://example.com/article-2"
    completed:
      - "https://example.com/completed-1"
```

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

gmail の smtp サーバーを使うことする。利用方法は別途検討とする。
