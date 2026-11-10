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
| 2. Constant | An assignment whose name or place says it is a constant or a global | `FOO = 1` (Ruby: starts with an uppercase letter; Python: at the top of a module) |
| 3. Member | A field, property or method of a class declared without a keyword, or made by a call of the language | `foo() {`, `foo: string;` (JS / TS), `foo: int` (Python), `attr_reader :foo` (Ruby) |
| 4. Framework DSL | What a framework defines, mostly by a call naming it | `scope :foo` (Rails), `let(:foo)` (RSpec), `foo = models.CharField()` (Django) |

An assignment to a local variable is not a definition, as the same name is
assigned to all over a repository.

## Supported languages

| Language | Extensions | 1. Declaration | 2. Constant | 3. Member |
|---|---|---|---|---|
| Go | `.go` | `func`, `type`, `var`, `const` | | |
| Ruby | `.rb` `.rake` `.ru` `.gemspec` `.erb`, `Rakefile`, `Gemfile` | `def`, `class`, `module`, `alias` | `FOO = ...`, `FOO \|\|= ...` | `attr_reader` / `attr_writer` / `attr_accessor`, `alias_method`, `define_method` |
| Python | `.py` `.pyi` | `def`, `class` | `foo = ...` at the top of a module | Annotated fields, such as those of a dataclass: `foo: int`, `foo: str = ""` |
| JavaScript / TypeScript | `.js` `.jsx` `.mjs` `.cjs` `.ts` `.tsx` `.mts` `.cts` `.vue` `.svelte` | `function`, `class`, `interface`, `type`, `enum`, `const` / `let` / `var` | | Methods: `foo() {`, `async foo(a) {`, `static foo() {`, `#foo() {`. Fields: `foo: string;`, `private foo = 1`, `#foo = 1`, `foo = () => {`. Functions in an object: `foo: function (`, `foo: (a) =>` |
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
| FactoryBot | Ruby | `factory`, `trait`, `sequence` |
| Django | Python | `foo = models.CharField(...)`, and the fields of `serializers`, `forms` and `filters` |
| SQLAlchemy | Python | `foo = Column(...)`, `mapped_column`, `relationship`, `column_property`, `synonym` |

## Adding a language or a framework

Everything lives in [`git/definition.go`](../git/definition.go).

- **A language**: map its extensions to a key in `languageExtensions`, then add
  an entry to `languages` with its `declarations` (and `constants`, `members`,
  `definers` if it has them). Patterns put `NAME` where the name goes. Lines a
  pattern of a member could take for a definition, such as an `if` statement,
  go in `notDefinitions`.
- **A framework**: add an entry to `frameworks` with its language, and the
  `patterns` of the lines it defines things on or the calls that define
  things. A call naming what it defines by its first argument goes
  in a `definer`, and one naming each of its leading arguments, like
  `attr_reader :a, :b`, sets `everyArg`. The language has to say how such a
  call reads with `firstArgCall` and `everyArgCall`.

Add the lines that should and should not count to `TestDefinitionRegexp` or
`TestDefinitionMembers` in `git/definition_test.go`.
