# --from の相対日数指定対応計画

## 目的

`--from` オプションで従来の `yyyyMMdd` に加えて、`-5` のような
負の整数を指定できるようにする。負の整数 `n` は本日を基準に
`n` 日前の日付として扱い、既存の `yyyyMMdd` 指定と同じ取得条件にする。

例:

```sh
./htb2kdl --user basyura --from -2
```

本日が `2026-05-22` の場合は `2026-05-20` と解釈し、次と同じ扱いにする。

```sh
./htb2kdl --user basyura --from 20260520
```

## 修正案

1. `internal/cli/cli.go` の `--from` パース処理をヘルパー関数に分離する。
2. `yyyyMMdd` は従来どおり `time.Local` で日付として解釈する。
3. `-5` のように負の整数だけで構成される値は、今日のローカル日付から
   指定日数を引いた 0 時 0 分 0 秒として解釈する。
4. `-0`、`+2`、`abc`、`2026-05-20` などは既存のエラー方針に合わせて
   `--from は yyyyMMdd 形式または -n 形式で指定してください` のような
   明確なエラーにする。
5. `internal/cli/cli_test.go` に相対日数指定のテストを追加する。
   実行日に依存しないよう、今日の日付を差し替え可能な形にする。

## 確認方法

```sh
go test ./...
```

## 影響範囲

- CLI 引数の解釈のみ。
- Hatena RSS の取得や EPUB 生成処理には直接手を入れない。

## 追加修正: completed の保持件数制限

### 目的

`bookmarks.yml` の `completed` に残す URL は、直近追加した 100 件を
上限にする。100 件を超えた場合は古い URL から削除する。

### 修正案

1. `internal/bookmarks/bookmarks.go` に `completed` の最大保持件数を
   定数として定義する。
2. `CompleteFirst` で URL を `completed` に追加した後、100 件を超えた
   分を先頭から削除する。
3. 重複除外の既存挙動は維持する。
4. `internal/bookmarks/bookmarks_test.go` に 100 件上限のテストを追加する。

### 確認方法

```sh
go test ./...
```
