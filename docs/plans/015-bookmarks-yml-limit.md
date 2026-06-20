# bookmarks.yml の settings.limit 対応

## 目的

`bookmarks.yml` に次の設定を追加できるようにする。

```yaml
settings:
  limit: 10
```

実行時引数に `--limit` が指定されていない場合、かつ `bookmarks.yml` に
`settings.limit` が設定されている場合は、その値を蓄積モードのしきい値として使う。

## 修正案

1. `internal/bookmarks` の YAML モデルに `settings.limit` を追加する。
   - `limit` は未設定と `0` を区別できるようにポインタで保持する。
   - `settings.limit` が負数の場合は読み込み時にエラーにする。
2. CLI の `--limit` 指定有無を判定できるようにする。
   - `--limit` が指定された場合は従来通り引数の値を優先する。
   - `--limit` が未指定で、`settings.limit` が 1 以上なら蓄積モードで実行する。
   - `--limit` が未指定で、`settings.limit` が未設定または `0` なら従来通り即時生成する。
3. 蓄積モード開始時に `bookmarks.yml` を読み込み、必要に応じて有効な
   `limit` を決定する。
4. テストを追加・更新する。
   - `settings.limit` の読み込みと保存を確認する。
   - `--limit` 指定時は YAML より引数が優先されることを確認する。
   - YAML の負数 `settings.limit` はエラーになることを確認する。
5. README の `bookmarks.yml` 例と `--limit` の説明を更新する。

## 確認

- `go test ./...`

## 実施結果

- `internal/bookmarks` に `settings.limit` の読み書きと負数チェックを追加した。
- CLI で `--limit` の指定有無を判定し、未指定時のみ `settings.limit` を使うようにした。
- 明示的な `--limit 0` は YAML の `settings.limit` を使わず即時生成する扱いにした。
- README に `settings.limit` の形式と優先順位を追記した。
- `go test ./...` は成功した。

## 追加修正案

レビューで、`--limit` 未指定時に `bookmarks.yml` が存在しない場合でも、
即時生成モードとして動作すべきという指摘があった。

対応方針:

1. `--limit` 未指定時の `bookmarks.yml` 読み込みで、ファイルが存在しない場合は
   `settings.limit` 未設定と同じ扱いにして即時生成へ進める。
2. 既存の `bookmarks.yml` が壊れている場合や `settings.limit` が負数の場合は、
   設定ミスとしてエラーを返す。
3. ファイル未存在時にエラー扱いにならないことをユニットテストで固定する。

追加実施結果:

- `Run` の `--limit` 未指定分岐で `bookmarks.yml` の存在確認を行い、
  未存在なら即時生成に進むようにした。
- `bookmarks.yml` の存在確認ヘルパーを追加し、未存在と存在時のテストを追加した。
- `go test ./...` は成功した。
