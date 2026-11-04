# 定義へのジャンプ（`go`）

[English](./definition-jump.md)

`n` / `N` で単語カーソルを名前に合わせて `go` を押すと、その名前が定義されている場所をファイル表示で開きます。`Ctrl+O` で `go` を押した場所に戻ります。定義が複数見つかったときは、コードのプレビュー付きの一覧から選びます（`j` / `k` で選択、`Enter` で開く、`Esc` でキャンセル）。

`git` 以外に何も要らないので、SSH 先や作りたてのコンテナでも同じように使えます。

## 定義の探し方

1. `git grep` で、その名前が単語として出てくる行を探します。追跡されていないファイルも対象です。
2. 各行を、そのファイルの言語のパターン（後述）に照らし、定義らしい行だけを残します。
3. 今読んでいるファイルと同じ言語の定義だけを候補にします。
4. 今読んでいるファイルの定義、同じディレクトリの定義、その他の順に並べます。

パターンは行頭だけを見るので、名前を呼び出している行やコメントは拾いません。ただしパーサーではなく正規表現での判定なので、変わった書き方の行が紛れ込むことはあります。

## 定義と見なす行

定義と見なす行は次の4種類です。1〜3 は言語ごと、4 はフレームワークごとに決まります。

| 種類 | 内容 | 例 |
|---|---|---|
| 1. 宣言 | キーワードで始まる行 | `def foo`、`func Foo`、`class Foo`、`fn foo` |
| 2. 定数 | 名前から定数と分かる代入 | `FOO = 1`（Ruby では大文字で始まる名前） |
| 3. 言語の機能で作るメンバー | 引数で名前を受け取って定義する、言語の呼び出し | `attr_reader :foo`、`alias_method :foo, :bar` |
| 4. フレームワークの DSL | 引数で名前を受け取って定義する、フレームワークの呼び出し | `scope :foo`（Rails）、`let(:foo)`（RSpec） |

ローカル変数への代入は定義と見なしません。同じ名前への代入がリポジトリ中にあふれているからです。

## 対応している言語

| 言語 | 拡張子 | 1. 宣言 | 2. 定数 | 3. メンバー |
|---|---|---|---|---|
| Go | `.go` | `func`、`type`、`var`、`const` | | |
| Ruby | `.rb` `.rake` `.ru` `.gemspec` `.erb`、`Rakefile`、`Gemfile` | `def`、`class`、`module`、`alias` | `FOO = ...`、`FOO \|\|= ...` | `attr_reader` / `attr_writer` / `attr_accessor`、`alias_method`、`define_method` |
| Python | `.py` `.pyi` | `def`、`class` | | |
| JavaScript / TypeScript | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.mts` `.cts` `.vue` `.svelte` | `function`、`class`、`interface`、`type`、`enum`、`const` / `let` / `var` | | |
| Rust | `.rs` | `fn`、`struct`、`enum`、`trait`、`type`、`const`、`static`、`mod` | | |

`.erb` は Ruby、`.vue` と `.svelte` は JavaScript / TypeScript として扱うので、テンプレートから呼んでいるヘルパーなどにも飛べます。これ以外の言語のファイルで `go` を押すと、未対応である旨を表示します。そうしたファイルの行が定義として候補に出ることもありません。

## 対応しているフレームワーク

リポジトリで使っているかどうかに関係なく、その言語のファイルで有効です。これらの呼び出しが別のものを指すことはほとんどないためです。

| フレームワーク | 言語 | 呼び出し |
|---|---|---|
| Rails | Ruby | `scope`、`has_many`、`has_one`、`belongs_to`、`has_and_belongs_to_many`、`delegate` |
| RSpec | Ruby | `let`、`let!`、`subject` |

## 言語・フレームワークを追加するには

すべて [`git/definition.go`](../git/definition.go) にあります。

- **言語**：`languageExtensions` で拡張子を言語のキーに対応させ、`languages` にその言語の `declarations`（あれば `constants`、`definers` も）を足します。パターンの中では、名前が入る場所を `NAME` と書きます。
- **フレームワーク**：`frameworks` に、言語と、定義を作る呼び出しの名前を足します。最初の引数で名前を受け取る呼び出しは `definer` に、`attr_reader :a, :b` のように先頭の引数それぞれが名前になる呼び出しは `everyArg` を付けて書きます。その言語の `firstArgCall` / `everyArgCall` に、こうした呼び出しの書き方が定義されている必要があります。

定義と見なすべき行・見なすべきでない行を、`git/definition_test.go` の `TestDefinitionRegexp` に足してください。
