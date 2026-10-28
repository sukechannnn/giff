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
| `/` | Grep the diffs (narrows the list to files with matching changed lines) |
| `Ctrl+F` | Filter files by name (globs, and comma separated lists) |
| `!` | Hide files by path — the inverse of `Ctrl+F`, e.g. `generated,node_modules` |
| `v` | Open in $EDITOR (at the current hit while a `/` grep is active) |
| `c` | Open in VS Code (at the current hit while a `/` grep is active) |
| `n` / `N` | Next / prev grep hit (while a `/` grep is active) |
| `Ctrl+L` | Git log |
| `pr` | PR diff — the branch's changes as GitHub shows them, even after they are committed and pushed |
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
| `n` / `N` | Next / prev match (steps into the next file while a `/` grep is active) |
| `e` | Toggle fold |
| `s` | Split view |
| `w` | Hide whitespace |
| `y` | Yank lines |
| `Y` | Copy file path |
| `f` | Show the whole file at the cursor (`f` again returns to the diff) |
| `Ctrl+L` | Copy `path:line` |
| `pr` | PR diff |
| `Ctrl+E` / `Ctrl+Y` | Scroll |
| `Esc` | Back to file list |
| `q` | Quit |

The PR diff (`pr`) and the commits of the Git log (`Ctrl+L`) are read-only: the keys above work there too, except staging, discarding, undo, and opening the Git log or the PR diff. In the PR diff, `r` reloads it after new commits.
The PR diff (`pr`) and the commits of the Git log (`Ctrl+L`) are read-only: the keys above work there too, except staging, discarding, undo, showing the whole file (`f`), and opening the Git log or the PR diff. In the PR diff, `r` reloads it after new commits.

## License

MIT
