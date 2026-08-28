package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseChangedLines(t *testing.T) {
	t.Run("collects added and removed lines per file", func(t *testing.T) {
		diff := `diff --git a/foo.go b/foo.go
index 1111111..2222222 100644
--- a/foo.go
+++ b/foo.go
@@ -1 +1,2 @@
-old line
+new line
+extra line
diff --git a/bar.go b/bar.go
index 3333333..4444444 100644
--- a/bar.go
+++ b/bar.go
@@ -5 +5 @@
-bar before
+bar after
`
		got := parseChangedLines(diff)
		want := map[string][]string{
			"foo.go": {"old line", "new line", "extra line"},
			"bar.go": {"bar before", "bar after"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseChangedLines() = %v, want %v", got, want)
		}
	})

	t.Run("uses the old path for deleted files", func(t *testing.T) {
		diff := `diff --git a/gone.go b/gone.go
deleted file mode 100644
index 1111111..0000000
--- a/gone.go
+++ /dev/null
@@ -1 +0,0 @@
-was here
`
		got := parseChangedLines(diff)
		want := map[string][]string{"gone.go": {"was here"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseChangedLines() = %v, want %v", got, want)
		}
	})

	t.Run("does not mistake changed lines for file headers", func(t *testing.T) {
		// A removed line whose content starts with "-- " looks like a "--- "
		// header, and an added "++ " line looks like a "+++ " header.
		diff := `diff --git a/doc.md b/doc.md
index 1111111..2222222 100644
--- a/doc.md
+++ b/doc.md
@@ -1,2 +1,2 @@
--- signature
+++ replacement
`
		got := parseChangedLines(diff)
		want := map[string][]string{"doc.md": {"-- signature", "++ replacement"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseChangedLines() = %v, want %v", got, want)
		}
	})

	t.Run("ignores the no-newline marker and empty output", func(t *testing.T) {
		diff := `diff --git a/foo.go b/foo.go
index 1111111..2222222 100644
--- a/foo.go
+++ b/foo.go
@@ -1 +1 @@
-a
\ No newline at end of file
+b
`
		got := parseChangedLines(diff)
		want := map[string][]string{"foo.go": {"a", "b"}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseChangedLines() = %v, want %v", got, want)
		}
		if len(parseChangedLines("")) != 0 {
			t.Error("expected no results for empty diff text")
		}
	})
}

func TestDiffHeaderPath(t *testing.T) {
	cases := map[string]string{
		"a/foo.go":             "foo.go",
		"b/dir/a/foo.go":       "dir/a/foo.go",
		"/dev/null":            "",
		"b/foo.go\t timestamp": "foo.go",
		"a/日本語 の file.go":      "日本語 の file.go",
	}
	for input, want := range cases {
		if got := diffHeaderPath(input); got != want {
			t.Errorf("diffHeaderPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCountMatches(t *testing.T) {
	lines := []string{"Logger.Debug", "no hit", "call logger twice logger", "LOGGER"}
	if got := countMatches(lines, "logger"); got != 3 {
		t.Errorf("countMatches() = %d, want 3 (case-insensitive, one hit per line)", got)
	}
	if got := countMatches(lines, "missing"); got != 0 {
		t.Errorf("countMatches() = %d, want 0", got)
	}
}

func TestGrepDiff(t *testing.T) {
	repo := setupGrepRepo(t)

	t.Run("finds hits across staged, unstaged and untracked files", func(t *testing.T) {
		untracked := []FileInfo{{Path: "fresh.txt", ChangeStatus: "untracked"}}
		hits, err := GrepDiff(repo, "needle", untracked, false)
		if err != nil {
			t.Fatalf("GrepDiff() error = %v", err)
		}
		want := map[string]int{
			DiffGrepKey(GrepStageStaged, "staged.txt"):     1,
			DiffGrepKey(GrepStageUnstaged, "unstaged.txt"): 2,
			DiffGrepKey(GrepStageUntracked, "fresh.txt"):   1,
		}
		if !reflect.DeepEqual(hits, want) {
			t.Errorf("GrepDiff() = %v, want %v", hits, want)
		}
	})

	t.Run("ignores context lines", func(t *testing.T) {
		hits, err := GrepDiff(repo, "untouched context", nil, false)
		if err != nil {
			t.Fatalf("GrepDiff() error = %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no hits for a context-only match, got %v", hits)
		}
	})

	t.Run("returns an empty map for an empty query", func(t *testing.T) {
		hits, err := GrepDiff(repo, "", nil, false)
		if err != nil {
			t.Fatalf("GrepDiff() error = %v", err)
		}
		if len(hits) != 0 {
			t.Errorf("expected no hits for an empty query, got %v", hits)
		}
	})
}

// setupGrepRepo creates a repository holding one staged change, one unstaged
// change and one untracked file, each with a "needle" line plus a shared
// context line that must never match.
func setupGrepRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")

	write("staged.txt", "untouched context\nbefore\n")
	write("unstaged.txt", "untouched context\nbefore\nbefore\n")
	run("add", ".")
	run("commit", "-m", "initial")

	write("staged.txt", "untouched context\nstaged needle\n")
	run("add", "staged.txt")

	write("unstaged.txt", "untouched context\nunstaged needle\nanother needle\n")
	write("fresh.txt", "untouched context\nuntracked needle\n")

	return repo
}
