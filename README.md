# htb2kdl

任意のユーザーのはてなブックマークを取得し、ブックマークの各ページの html を取得して epub を生成して kindle に送る。

## 使い方

```sh
go run . --user <はてなID> --from <yyyyMMdd>
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


