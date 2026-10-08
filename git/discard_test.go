package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const discardBase = "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"

// setupDiscardRepo commits discardBase, stages "line 1" -> "ONE" and leaves
// the given working tree content unstaged
func setupDiscardRepo(t *testing.T, working string) string {
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
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	write(discardBase)
	run("add", ".")
	run("commit", "-m", "initial")
	write(strings.Replace(discardBase, "line 1\n", "ONE\n", 1))
	run("add", "a.txt")
	write(working)
	return repo
}

// diffLineIndex returns the index of the diff line equal to want
func diffLineIndex(t *testing.T, diffText, want string) int {
	t.Helper()
	for i, line := range strings.Split(diffText, "\n") {
		if line == want {
			return i
		}
	}
	t.Fatalf("diff line %q not found in:\n%s", want, diffText)
	return -1
}

func readA(t *testing.T, repo string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func stagedDiff(t *testing.T, repo string) string {
	t.Helper()
	diff, err := GetStagedDiff("a.txt", repo)
	if err != nil {
		t.Fatal(err)
	}
	return diff
}

func TestDiscardSelectedChanges(t *testing.T) {
	// "line 2" modified, "new" inserted after "line 5", "line 8" deleted
	working := "ONE\ntwo\nline 3\nline 4\nline 5\nnew\nline 6\nline 7\nline 9\nline 10\n"

	tests := []struct {
		name       string
		first      string // first selected diff line
		last       string // last selected diff line
		wantResult string
	}{
		{
			name:       "modified line pair",
			first:      "-line 2",
			last:       "+two",
			wantResult: "ONE\nline 2\nline 3\nline 4\nline 5\nnew\nline 6\nline 7\nline 9\nline 10\n",
		},
		{
			name:       "only the added line of a pair",
			first:      "+two",
			last:       "+two",
			wantResult: "ONE\nline 3\nline 4\nline 5\nnew\nline 6\nline 7\nline 9\nline 10\n",
		},
		{
			name:       "pure addition",
			first:      "+new",
			last:       "+new",
			wantResult: "ONE\ntwo\nline 3\nline 4\nline 5\nline 6\nline 7\nline 9\nline 10\n",
		},
		{
			name:       "pure deletion",
			first:      "-line 8",
			last:       "-line 8",
			wantResult: "ONE\ntwo\nline 3\nline 4\nline 5\nnew\nline 6\nline 7\nline 8\nline 9\nline 10\n",
		},
		{
			name:       "range spanning context lines",
			first:      "+new",
			last:       "-line 8",
			wantResult: "ONE\ntwo\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupDiscardRepo(t, working)
			stagedBefore := stagedDiff(t, repo)
			diffText, err := GetFileDiff("a.txt", repo)
			if err != nil {
				t.Fatal(err)
			}

			start := diffLineIndex(t, diffText, tt.first)
			end := diffLineIndex(t, diffText, tt.last)
			if err := DiscardSelectedChanges("a.txt", repo, diffText, start, end); err != nil {
				t.Fatal(err)
			}

			if got := readA(t, repo); got != tt.wantResult {
				t.Errorf("working tree:\ngot  %q\nwant %q", got, tt.wantResult)
			}
			if got := stagedDiff(t, repo); got != stagedBefore {
				t.Errorf("staged changes must be kept:\ngot  %q\nwant %q", got, stagedBefore)
			}
		})
	}
}

// With whitespace changes hidden, lines outside the selection keep their
// whitespace changes instead of being reset to the index version
func TestDiscardSelectedChangesKeepsHiddenWhitespaceChanges(t *testing.T) {
	working := "ONE\ntwo\nline 3\n  line 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	repo := setupDiscardRepo(t, working)

	diffText, err := GetFileDiffWithOptions("a.txt", repo, true)
	if err != nil {
		t.Fatal(err)
	}
	start := diffLineIndex(t, diffText, "-line 2")
	end := diffLineIndex(t, diffText, "+two")
	if err := DiscardSelectedChanges("a.txt", repo, diffText, start, end); err != nil {
		t.Fatal(err)
	}

	want := "ONE\nline 2\nline 3\n  line 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if got := readA(t, repo); got != want {
		t.Errorf("working tree:\ngot  %q\nwant %q", got, want)
	}
}

func TestDiscardSelectedChangesUntrackedFile(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "new.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Same shape as util.FormatAsAddedLines
	diffText := "diff --git a/new.txt b/new.txt\nnew file mode 100644\nindex 0000000..0000000\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,4 @@\n+a\n+b\n+c\n+\n"

	idx := diffLineIndex(t, diffText, "+b")
	if err := DiscardSelectedChanges("new.txt", repo, diffText, idx, idx); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "a\nc\n" {
		t.Errorf("got %q, want %q", got, "a\nc\n")
	}
}

func TestDiscardSelectedChangesRefusesStaleDiff(t *testing.T) {
	repo := setupDiscardRepo(t, "ONE\ntwo\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n")
	diffText, err := GetFileDiff("a.txt", repo)
	if err != nil {
		t.Fatal(err)
	}

	// The file changes after the diff was taken
	changed := "ONE\nTWO\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	start := diffLineIndex(t, diffText, "-line 2")
	end := diffLineIndex(t, diffText, "+two")
	if err := DiscardSelectedChanges("a.txt", repo, diffText, start, end); err == nil {
		t.Fatal("expected an error for a diff that no longer matches the file")
	}
	if got := readA(t, repo); got != changed {
		t.Errorf("file must be left untouched, got %q", got)
	}
}

func TestDiscardFileChanges(t *testing.T) {
	repo := setupDiscardRepo(t, "ONE\ntwo\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n")
	stagedBefore := stagedDiff(t, repo)

	if err := DiscardFileChanges("a.txt", repo); err != nil {
		t.Fatal(err)
	}

	// Back to the index version: the staged "ONE" stays, the unstaged "two" is gone
	want := strings.Replace(discardBase, "line 1\n", "ONE\n", 1)
	if got := readA(t, repo); got != want {
		t.Errorf("working tree:\ngot  %q\nwant %q", got, want)
	}
	if got := stagedDiff(t, repo); got != stagedBefore {
		t.Errorf("staged changes must be kept:\ngot  %q\nwant %q", got, stagedBefore)
	}
}

func TestDeleteUntrackedFile(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "new.txt")
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteUntrackedFile("new.txt", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("new.txt must be deleted, stat error: %v", err)
	}
}
