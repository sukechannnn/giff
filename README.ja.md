<p align="center">
<img src="./docs/images/giff_icon.svg" width="120" alt="giff">
</p>

# giff

[English](./README.md) | 日本語

`giff` はターミナルで動く Git diff ビューアです。ワーキングツリーを監視して、差分をリアルタイムに表示します。

![giff screenshot](./docs/images/preview_1.png)

## インストール

macOS と Linux（amd64 / arm64）で動作します。

**homebrew (macOS / Linux):**

```console
$ brew install --cask sukechannnn/tap/giff
```

**go install:**

```console
$ go install github.com/sukechannnn/giff@latest
```

**ビルド済みバイナリ:**

[リリースページ](https://github.com/sukechannnn/giff/releases)から環境に合ったアーカイブをダウンロードして展開し、`giff` を `PATH` の通った場所に置いてください:

```console
$ tar -xzf giff_<version>_linux_amd64.tar.gz
$ sudo mv giff /usr/local/bin/
```

**ソースからビルド:**

```console
$ git clone https://github.com/sukechannnn/giff.git
$ cd giff
$ go build -o giff
```

## 使い方

```console
$ giff            # 現在の変更を表示
$ giff --watch    # ウォッチモード: ファイル変更時に自動更新
$ giff -v         # バージョン表示
```

### ファイルリスト

| キー | 操作 |
|------|------|
| `j` / `k` | カーソル移動 |
| `H` / `L` | ディレクトリの折りたたみ/展開 |
| `a` | ステージ/アンステージ |
| `d` | 変更を破棄 |
| `Ctrl+A` | 全ファイルをステージ |
| `Ctrl+K` | コミット |
| `Ctrl+J` | amend |
| `s` | Split View |
| `w` | 空白変更を非表示 |
| `/` | 差分を grep（マッチした変更行を持つファイルだけに絞り込み） |
| `Ctrl+F` | ファイル名で絞り込み（glob・カンマ区切りの複数指定に対応） |
| `!` | パスで非表示 — `Ctrl+F` の逆（例: `generated,node_modules`） |
| `v` | $EDITOR で開く（`/` の grep 中は現在のヒット行で開く） |
| `c` | VS Code で開く（`/` の grep 中は現在のヒット行で開く） |
| `n` / `N` | 次/前の grep ヒット（`/` の grep 中） |
| `Ctrl+L` | Git ログ |
| `pr` | PR の差分（コミット・push 済みの変更も GitHub の PR と同じ形で表示） |
| `t` | ターミナルペインを開く — コマンドを実行、`Tab` で出力をスクロール |
| `Enter` | 差分ビューに切替 |
| `q` | 終了 |

### 差分ビュー

| キー | 操作 |
|------|------|
| `j` / `k` | カーソル移動 |
| `gg` / `G` | 先頭/末尾 |
| `V` | 行選択 |
| `a` | 選択行をステージ |
| `A` | ファイル全体をステージ/アンステージ |
| `/` | 検索 |
| `n` / `N` | 次/前の検索結果（`/` の grep 中はファイルをまたいで移動） |
| `e` | 折りたたみ切替 |
| `s` | Split View |
| `w` | 空白変更を非表示 |
| `y` | 行をコピー |
| `Y` | ファイルパスをコピー |
| `f` | カーソル位置のファイル全体を表示（もう一度 `f` で差分に戻る） |
| `Ctrl+L` | `path:行番号` をコピー |
| `pr` | PR の差分 |
| `Ctrl+E` / `Ctrl+Y` | スクロール |
| `Esc` | ファイルリストに戻る |
| `q` | 終了 |

PR の差分（`pr`）と Git ログ（`Ctrl+L`）のコミットは読み取り専用です。ステージ・破棄・取り消しと、Git ログ・PR の差分を開く操作以外は、上のキーがそのまま使えます。PR の差分では、`r` で新しいコミットを反映できます。
PR の差分（`pr`）と Git ログ（`Ctrl+L`）のコミットは読み取り専用です。ステージ・破棄・取り消し・ファイル全体の表示（`f`）と、Git ログ・PR の差分を開く操作以外は、上のキーがそのまま使えます。PR の差分では、`r` で新しいコミットを反映できます。

## ライセンス

MIT
