---
name: release
description: >
  Commit the pending work at a sensible granularity, push to main, then tag
  the release and push the tag. Use when asked to commit and release, or to
  cut a new version of giff.
allowed-tools:
  - Bash
  - Read
  - Grep
disable-model-invocation: true
---

# Release

Take the working tree from "the change is done" to "the tag is pushed".

## 1. Verify

All of this must pass before anything is committed. Fix what fails, then run it
again.

```bash
gofmt -l git ui util config main.go   # must print nothing
go build -o giff .
go vet ./...
go test ./...
```

`ui/git_log_view.go` is not gofmt-clean to begin with, so never run
`go fmt ./...` over the whole tree: it rewrites files the change never touched.
Format only the files you edited, with `gofmt -w`.

## 2. Commit

Read `git status` and `git diff`, then split the work by topic.

- Messages in English.
- Prefix with `add:` (new feature), `update:` (changed behavior), `refactor:`
  (no behavior change), `fix:` (bug), `docs:`, `test:` or `chore:`.
- Subject line, blank line, then the body.
- A feature and a pre-existing bug that was found while building it belong in
  separate commits.

### Splitting one file that carries two topics

`git add -p` is not available here, so rewind the working tree instead:

1. Copy the finished files to a scratch directory (`command cp -f`).
2. Revert only the changes that belong to the later commit.
3. Confirm the build and the tests still pass, then commit.
4. Restore the finished files, confirm again, then commit.

Every commit has to build and pass its tests on its own, not just the last one.

## 3. Push

This repository pushes straight to main.

```bash
git push origin main
```

## 4. Tag

```bash
git tag -l | sort -V | tail -3     # find the current latest
git tag vX.Y.Z                     # lightweight, never annotated
git push origin vX.Y.Z
```

- Bump the patch by default (v0.3.2 to v0.3.3).
- Ask before bumping the minor, even for a sizable feature.
- Tag the commit that is already on origin/main.

The Homebrew cask updates itself from a GitHub Actions workflow watching for new
tags, so pushing the tag is the entire release.

## Watch out for

- `README.md` and `README.ja.md` both carry the key binding tables, so a changed
  key means editing both.
- So do the status bar hints in `ui/root_editor.go` (`fileListKeyMessage` and
  `diffViewKeyMessage`).
