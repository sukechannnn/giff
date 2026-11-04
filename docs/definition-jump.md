# Jump to definition (`go`)

[日本語](./definition-jump.ja.md)

Put the word cursor on a name with `n` / `N`, then press `go`. giff shows where
the name is defined in the file view, and `Ctrl+O` goes back to where `go` was
pressed. When there are several definitions, a list with a preview of their
code comes up to pick one from (`j` / `k` to select, `Enter` to open, `Esc` to
cancel).

It needs nothing besides `git`, so it works the same over SSH or in a fresh
container.

## How definitions are found

1. `git grep` finds every line the name appears on as a whole word, in tracked
   and untracked files alike.
2. Each line is kept when it reads like a definition in the language of its
   file, by the patterns described below.
3. Only the definitions in the language of the file being read are offered.
4. The ones in the file being read come first, then those in its directory.

The patterns only look at the start of a line, so calls and comments that
mention the name never count. As they are regular expressions, not a parser,
an odd line can still slip through.

## What counts as a definition

A definition is found on a line of one of four kinds. The first three are a
matter of the language, the last one of a framework.

| Kind | What it is | Examples |
|---|---|---|
| 1. Declaration | A line led by a keyword | `def foo`, `func Foo`, `class Foo`, `fn foo` |
| 2. Constant | An assignment whose name says it is a constant | `FOO = 1` (Ruby: starts with an uppercase letter) |
| 3. Member made by the language | A call of the language that defines what its arguments name | `attr_reader :foo`, `alias_method :foo, :bar` |
| 4. Framework DSL | A call of a framework that defines what its arguments name | `scope :foo` (Rails), `let(:foo)` (RSpec) |

An assignment to a local variable is not a definition, as the same name is
assigned to all over a repository.

## Supported languages

| Language | Extensions | 1. Declaration | 2. Constant | 3. Member |
|---|---|---|---|---|
| Go | `.go` | `func`, `type`, `var`, `const` | | |
| Ruby | `.rb` `.rake` `.ru` `.gemspec` `.erb`, `Rakefile`, `Gemfile` | `def`, `class`, `module`, `alias` | `FOO = ...`, `FOO \|\|= ...` | `attr_reader` / `attr_writer` / `attr_accessor`, `alias_method`, `define_method` |
| Python | `.py` `.pyi` | `def`, `class` | | |
| JavaScript / TypeScript | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.mts` `.cts` `.vue` `.svelte` | `function`, `class`, `interface`, `type`, `enum`, `const` / `let` / `var` | | |
| Rust | `.rs` | `fn`, `struct`, `enum`, `trait`, `type`, `const`, `static`, `mod` | | |

Ruby takes in `.erb` templates, and JavaScript / TypeScript `.vue` and
`.svelte` files, so a helper called from one can be jumped to. In a file of
any other language `go` says it is not supported, and the lines of such files
never count as definitions.

## Supported frameworks

They count in the files of their language, whether or not the repository uses
them: their calls are hardly ever a call of something else.

| Framework | Language | Calls |
|---|---|---|
| Rails | Ruby | `scope`, `has_many`, `has_one`, `belongs_to`, `has_and_belongs_to_many`, `delegate` |
| RSpec | Ruby | `let`, `let!`, `subject` |

## Adding a language or a framework

Everything lives in [`git/definition.go`](../git/definition.go).

- **A language**: map its extensions to a key in `languageExtensions`, then add
  an entry to `languages` with its `declarations` (and `constants`, `definers`
  if it has them). Patterns put `NAME` where the name goes.
- **A framework**: add an entry to `frameworks` with its language and the calls
  that define things. A call naming what it defines by its first argument goes
  in a `definer`, and one naming each of its leading arguments, like
  `attr_reader :a, :b`, sets `everyArg`. The language has to say how such a
  call reads with `firstArgCall` and `everyArgCall`.

Add the lines that should and should not count to `TestDefinitionRegexp` in
`git/definition_test.go`.
