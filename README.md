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
