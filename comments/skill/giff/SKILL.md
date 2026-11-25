---
name: giff
description: >
  Answer the questions and requests left on lines of code with `i` in giff,
  the Git diff viewer, and write each answer back for giff to show.
  Requires the giff CLI.
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
  - Edit
  - Write
---

# giff のコメントに答えるスキル

giff で差分やファイルを見ている人が、行にカーソルを当てて（または `V` で選んで）`i` を押し、その行への質問や依頼をコメントとして残します。このスキルではそれを取りに行って答え、答えを giff に返します。

コメントはリポジトリごとに分かれています。`giff comment` はカレントディレクトリの Git リポジトリのコメントだけを扱うので、作業しているリポジトリの中で実行してください。

## 基本フロー

コメントはバックグラウンドで待ち受けます。待っている間はターンを終えて、利用者との普通のチャットに応じられるようにしておきます。

1. Bash ツールで `giff comment next --wait 60` を **`run_in_background: true` で** 起動する（コメントが届くまで最大60分待つ）。起動したら「giff のコメントを待ち受けています」と一言伝えてターンを終える。結果を待って同じターンにとどまらない
2. コメントが届くとコマンドが終わり、通知が来る。その出力（JSON）を読む
   - `path` と `startLine` / `endLine`: コメントが付いたファイルと行（リポジトリのルートからの相対パス、1始まり）
   - `view`: giff で何を見ていたか（`unstaged diff`、`staged diff`、`file`、`PR diff`、`commit <hash>` など）。`commit` のときの行番号はそのコミットの版のもの
   - `code`: コメントを付けた行。差分なら `+` / `-` 付き
   - `question`: いま答える質問や依頼
   - `thread`: そのコメントのやり取り全体（`from` が `you` は giff の利用者、`claude` はこれまでの答え）。2件以上あるときは、前の答えへの返信なので、それまでの流れを踏まえて `question` に答える
3. 必要に応じてファイルの前後やリポジトリの他の箇所を読んで、`question` に答える
   - 質問なら、その答えを書く
   - コードの変更を頼まれたら、変更してから、何をどう変えたかを書く
4. 答えを giff に返す。答えは標準入力で渡す:
   ```bash
   giff comment answer <id> <<'EOF'
   （答え）
   EOF
   ```
5. もう一度 1 の待ち受けをバックグラウンドで起動して、ターンを終える
6. コマンドが `Timed out waiting for comments` で終わったら、待ち受けをやめたことを一言伝えて終える（再開は利用者がもう一度 `/giff` を実行する）

## 待ち受けのルール

- 利用者のチャットを優先する。待ち受け中に話しかけられたら、普通に応じる。待ち受けはバックグラウンドで続いている
- 待ち受けは同時に1つだけ。すでに `giff comment next --wait` がバックグラウンドで動いているなら、新しく起動しない
- 利用者に「待ち受けをやめて」と言われたら、そのバックグラウンドのコマンドを止める
- 他の作業の途中で通知が来たら、その作業の区切りでコメントに答える。取り出したコメントは答えるまで `in_progress` のまま残り、`giff comment next` を実行すると同じものがもう一度返る

## 答え方

- giff のプレーンテキストの欄に出るので、Markdown の表や見出しは使わず、短く書く。コードはそのまま書いてよい
- `question` と同じ言語で答える
- コメントに書かれていないことは勝手にやらない
- 答えられない、または判断に迷うときは、その旨と理由を答えとして返す
