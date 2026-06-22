# 任意 URL から EPUB を生成するデバッグ機能

## 目的

デバッグ用途として、はてなブックマーク RSS を経由せず、任意の URL を 1 件指定して EPUB を生成できるようにする。

## 修正案

- `./htb2kdl <url>` のように位置引数で URL を 1 件指定できるようにする。
- URL の位置引数がある場合は、`--user` と `--from` を不要にする。
- URL の位置引数がある場合は Hatena RSS の取得や `bookmarks.yml` の蓄積処理を行わず、指定 URL だけを記事抽出対象にする。
- URL の位置引数がある場合でも既存の `--out`、`--css` は利用できるようにする。
- デバッグ EPUB のタイトルと著者は、デバッグ用途であることが分かる固定値にする。
- `parseArgs` と実行分岐のテストを追加し、通常の Hatena 取得モードに影響しないことを確認する。
- README にデバッグ用の実行例を追記する。

## 想定する利用例

```sh
./htb2kdl https://coliss.com/articles/build-websites/operation/work/chrome-149-adds-9-new-css-feature.html
./htb2kdl https://example.com/article --out debug.epub
```

## 確認

- `go test ./...` を実行する。
