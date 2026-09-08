<p align="center">
<img src="./docs/images/giff_icon.svg" width="120" alt="giff">
</p>

# giff

[English](./README.md) | 日本語

`giff` はターミナルで動く Git diff ビューアです。ワーキングツリーを監視して、差分をリアルタイムに表示します。

![giff screenshot](./docs/images/giff_demo.gif)

## インストール

**homebrew:**

```console
$ brew install --cask sukechannnn/tap/giff
```

**go install:**

```console
$ go install github.com/sukechannnn/giff@latest
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
| `t` | ターミナルを開く（tmux split） |
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
| `Ctrl+L` | `path:行番号` をコピー |
| `Ctrl+E` / `Ctrl+Y` | スクロール |
| `Esc` | ファイルリストに戻る |
| `q` | 終了 |

## ライセンス

MIT
