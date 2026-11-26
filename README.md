<p align="center">
<br><br><br>
<img src="./docs/images/giff_icon.svg" width="120" alt="giff">
<br><br><br>
</p>

# giff

English | [日本語](./README.ja.md)

`giff` is a terminal-based Git diff viewer. It watches your working tree and shows diffs in real-time.

![giff screenshot](./docs/images/preview_1.png)

## Install

giff runs on macOS and Linux (amd64 / arm64).

**homebrew (macOS / Linux):**

```console
$ brew install --cask sukechannnn/tap/giff
```

**go install:**

```console
$ go install github.com/sukechannnn/giff@latest
```

**prebuilt binary:**

Download the archive for your platform from the [releases page](https://github.com/sukechannnn/giff/releases), extract it, and put `giff` on your `PATH`:

```console
$ tar -xzf giff_<version>_linux_amd64.tar.gz
$ sudo mv giff /usr/local/bin/
```

**build from source:**

```console
$ git clone https://github.com/sukechannnn/giff.git
$ cd giff
$ go build -o giff
```

## Usage

```console
$ giff            # view current changes
$ giff --watch    # watch mode: auto-refresh on file changes
$ giff -v         # show version
```

### File List

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor |
| `H` / `L` | Collapse/expand directory |
| `a` | Stage/unstage file |
| `d` | Discard changes |
| `Ctrl+A` | Stage all |
| `Ctrl+K` | Commit |
| `Ctrl+J` | Amend |
| `s` | Split view |
| `w` | Hide whitespace |
| `/` | Grep the diffs (narrows the list to files with matching changed lines; in the file browser, files with matching lines) |
| `Ctrl+F` | Filter files by name (globs, and comma separated lists) |
| `!` | Hide files by path — the inverse of `Ctrl+F`, e.g. `generated,node_modules` |
| `v` | Open in $EDITOR (at the current hit while a `/` grep is active) |
| `c` | Open in VS Code (at the current hit while a `/` grep is active) |
| `n` / `N` | Next / prev grep hit (while a `/` grep is active) |
| `Ctrl+L` | Git log |
| `pr` | PR diff — the branch's changes as GitHub shows them, even after they are committed and pushed |
| `b` | Switch branches — the local branches and those of the open pull requests, with their titles (the bases of the current branch's pull request first; a pull request's branch is checked out up to date with `gh pr checkout`). In the file browser (`f`), `b` shows or hides the blame instead: the commit that last changed each line on its left, and the one of the cursor's line in the status bar |
| `f` | File browser — the files tracked in the repository, opened at the file under the cursor (`f` again returns to the diffs) |
| `I` | The comments left with `i` in the diff view and their answers |
| `t` | Open a terminal pane — run a command, `Tab` scrolls its output |
| `Enter` | Switch to diff view |
| `q` | Quit |

### Diff View

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor |
| `gg` / `G` | Top / bottom |
| `V` | Select lines |
| `a` | Stage selected lines |
| `d` | Discard selected lines (unstaged changes only, cannot be undone) |
| `A` | Stage/unstage file |
| `/` | Search |
| `n` / `N` | Next / prev match of the search, or of the word under the `]` cursor (steps into the next file while a `/` grep is active) |
| `]` / `[` | Move a cursor word by word, on to the next / prev line at the end, highlighting every occurrence of the word, which `n` / `N` walk; `Esc` drops it |
| `l` / `h` (on a word) | Stretch the word under the `]` cursor over the next / previous word, with what lies between, e.g. `scheduled` → `scheduled-ecs` → `scheduled-ecs-task` (outside a word, they scroll sideways) |
| `go` | Jump to the definition of the word under the `]` cursor, found with `git grep` (several candidates are listed with a preview of their code to pick from; on the `source` of a Terraform module, the module's files; see [the details](./docs/definition-jump.md)) |
| `Ctrl+O` | Back to where `go` was pressed |
| `i` | Leave a question or a request on the line under the cursor, or the lines selected with `V`, for Claude Code to answer (see [Asking Claude Code](#asking-claude-code)). On a line with a comment, open or close it |
| `I` | The chat with Claude Code on the comment of the line under the cursor, or else the list of all the comments |
| `D` | Delete the comment on the line under the cursor |
| `e` | Toggle fold |
| `s` | Split view |
| `w` | Hide whitespace |
| `y` | Yank lines, without the indentation they share (the word while `]` / `[` is on one) |
| `Y` | Copy file path |
| `f` | Show the whole file at the cursor (`f` again returns to the diff) |
| `Ctrl+L` | Copy `path:line` |
| `pr` | PR diff |
| `b` | Switch branches (in the file browser, show / hide the blame) |
| `Ctrl+F` | Filter files by name (the focus goes to the file list) |
| `Ctrl+E` / `Ctrl+Y` | Scroll |
| `Esc` | Back to file list |
| `q` | Quit |

The PR diff (`pr`) and the commits of the Git log (`Ctrl+L`) are read-only: the keys above work there too, except staging, discarding, undo, opening the Git log or the PR diff, and switching branches (`b`). Showing the whole file (`f`) and jumping to a definition (`go`) work in the PR diff, whose files are those of the branch checked out, but not in the Git log. In the PR diff, `r` reloads it after new commits.

## Asking Claude Code

`i` in the diff view (or the file view) opens a text area right under the line under the cursor, or the lines selected with `V`, as GitHub does for a review comment, to leave a question or a request on them. `Enter` saves it as a comment (`Shift+Enter` for a new line, or `Option+Enter` in a terminal that cannot tell `Shift+Enter` from `Enter`), with the file, its lines and their code. A Claude Code session started in the same repository picks the comments up and answers them, and giff tells when an answer arrives.

A line with comments has a mark on its line number, yellow while they wait for an answer and green once answered. `i` on it opens the comment and its answer in full under it, until `i` again or the cursor leaves the line (a comment just written stays open the same way). `I` there opens the chat on it, to go on with Claude Code: `Enter` sends a reply, Claude Code answers it with the whole exchange in mind, and the answer shows up in the chat as it arrives (`Tab` moves to the chat to scroll it, `Esc` closes it). `D` on the line deletes the comment. Elsewhere, `I` lists all the comments (`Enter` shows a comment's lines, `i` opens its chat, `D` deletes it).

Install the `giff` skill for Claude Code once:

```console
$ giff comment install-skill
```

Then run `/giff` in Claude Code, in the repository you review with giff. It waits for comments, answers each one (making the change when the comment asks for one) and writes the answer back.

The comments are kept in `~/.giff/comments`, one file per repository (and per worktree): a Claude Code session only ever sees the comments of the repository it works in. `giff comment` reads and writes them from the command line:

```console
$ giff comment list [--all]       # the comments not answered yet (--all: and the answered ones)
$ giff comment next [--wait 10]   # pick up the next comment, as JSON, waiting 10 minutes for one
$ giff comment next --forever     # the same, waiting until there is one (what the skill runs)
$ giff comment answer <id>        # answer a comment, the text read from stdin
$ giff comment reply <id>         # reply to the answer, the text read from stdin
$ giff comment rm <id>            # delete a comment
$ giff comment clear [--all]      # delete the answered comments (--all: every comment)
```

## License

MIT
