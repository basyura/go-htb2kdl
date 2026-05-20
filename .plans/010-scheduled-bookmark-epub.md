# 定期取得と bookmarks.yml 管理による EPUB 生成

## 目的

タスクスケジューラや cron から定期実行できるようにし、指定ユーザーの
はてなブックマークを `bookmarks.yml` に蓄積する。蓄積された URL 数が
指定しきい値に達した場合だけ EPUB を生成し、生成に成功した URL を
`completed` に移動する。

## 修正案

1. `bookmarks.yml` 管理用の内部パッケージを追加する。
   - 実装上の重複判定キーとして URL を使い、同じ URL を重複登録しない。
   - `bookmarks` は URL のみの配列として保存する。
   - `completed` は EPUB 生成済み URL の配列として保存する。
   - 生成先 EPUB ファイル名は `bookmarks.yml` には保存しない。
   - `version` は持たせない。
   - YAML の読み書きは `gopkg.in/yaml.v3` を利用する。

2. `bookmarks.yml` の形式は次の形にする。

   ```yaml
   users:
     tatsuya:
       bookmarks:
         - "https://example.com/article-1"
         - "https://example.com/article-2"
       completed:
         - "https://example.com/completed-1"
   ```

3. CLI に定期実行向けオプションを追加する。
   - `--limit` で EPUB 生成に必要な未生成ブックマーク数を指定する。
   - `--limit` に 0 未満を指定した場合はエラーにする。
   - `--limit` 未指定または `0` の場合は従来モード、
     `1` 以上の場合は蓄積モードとして扱う。
   - `--from` は従来互換のため維持し、定期実行時の初回取得範囲として使う。
   - `--bookmarks-file` 引数は追加せず、蓄積モードでは常に
     htb2kdl の実行ファイルと同じディレクトリにある `bookmarks.yml` を使用する。

4. 実行フローを整理する。
   - 蓄積モードでは、先に `bookmarks.yml` を読み込む。
   - `bookmarks.yml` の読み込み・解析に成功した後で RSS を取得する。
   - RSS から取得したブックマークを `bookmarks.yml` にマージする。
   - 蓄積モードでも RSS 取得自体に失敗した場合はエラー終了し、
     `bookmarks.yml` に十分な件数があっても EPUB は生成しない。
     この場合、`bookmarks.yml` も変更しない。
   - 蓄積モードでは、RSS 取得結果が 0 件でもエラーにしない。
     `bookmarks.yml` の蓄積件数が `--limit` 以上なら EPUB を生成し、
     `--limit` 未満なら EPUB を生成せず正常終了する。
   - `bookmarks.yml` に蓄積された URL 数が `--limit` 未満なら EPUB を生成せず、
     `queued: <現在件数>/<limit>` を標準出力して正常終了する。
   - しきい値以上なら、古いブックマークから `--limit` 件だけ選んで EPUB を生成する。
   - 例として、既存 3 件に新規 5 件を追加して合計 8 件になった場合、
     `--limit 5` なら先頭 5 件だけを EPUB に含め、残り 3 件は
     `bookmarks.yml` に残す。
   - EPUB 生成に成功したら、EPUB に含めた URL だけを `bookmarks` から削除し、
     `completed` へ追加する。
   - 既存の `bookmarks.yml` の順序は維持する。
   - RSS 取得分はブックマーク日時の古い順に並べ、`bookmarks` と `completed` の
     どちらにも存在しない URL だけを `bookmarks` の末尾に追加する。
   - EPUB 化時は先頭から `--limit` 件を対象にする。
   - 該当記事を取得できない場合もスキップせず、その URL 用の章を作成する。
     章には記事を取得できなかった旨と対象 URL を出力し、EPUB 化済みとして扱う。
   - 取得失敗した URL も、EPUB 生成に成功した場合は `completed` へ移動する。
   - `bookmarks.yml` から URL を削除するかどうかは、EPUB ファイル自体が
     完成したかどうかで判断する。
   - 記事取得失敗ページを含んだ EPUB が完成した場合は成功扱いとし、
     今回対象にした URL を `completed` へ移動する。
   - 出力先への書き込み失敗や EPUB 組み立て失敗などで EPUB ファイル自体が
     完成しなかった場合は、`bookmarks.yml` を変更しない。
   - `bookmarks.yml` の更新は一時ファイルに書き込んだ後で置き換え、
     書き込み途中の失敗で既存ファイルが壊れないようにする。
   - `bookmarks.yml` が存在しない場合は空の状態として扱い、
     蓄積モードで URL を追加する必要がある場合に新規作成する。
   - `bookmarks.yml` が存在するが読み込みや YAML 解析に失敗した場合は
     エラー終了し、RSS 取得や EPUB 生成は行わず、`bookmarks.yml` も変更しない。
   - URL の重複判定は正規化せず、RSS から得た URL 文字列の完全一致で行う。
   - `bookmarks.yml` のパスは `os.Executable()` から実行ファイルの場所を取得して決める。
   - `--limit` 未指定または `0` の場合は従来通り RSS 取得分を即 EPUB 化し、
     `bookmarks.yml` は使用しない。

5. テストを追加・更新する。
   - YAML の読み書き、重複排除、URL 追加、生成後の `completed` 移動をテストする。
   - CLI のオプション解析としきい値判定をテストする。
   - 記事取得に失敗した URL でもエラーページ章が生成され、
     EPUB 化済みとして `completed` 移動対象になることをテストする。

6. README を更新する。
   - cron 等からの利用例を追加する。
   - `bookmarks.yml` と `--limit` の挙動を説明する。

## 確認方法

- `go test ./...`
- 必要に応じて `go build -o htb2kdl .` でビルドし、
  `./htb2kdl --user <はてなID> --from <yyyyMMdd> --limit <数>` で動作確認する。
