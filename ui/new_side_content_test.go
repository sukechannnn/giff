package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sukechannnn/giff/git"
)

func TestNewBlobHash(t *testing.T) {
	tests := []struct {
		diff string
		want string
	}{
		{"diff --git a/f b/f\nindex 1111111..2222222 100644\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n", "2222222"},
		{"diff --git a/f b/f\nnew file mode 100644\nindex 0000000..3333333\n--- /dev/null\n", "3333333"},
		{"diff --git a/f b/f\ndeleted file mode 100644\nindex 4444444..0000000\n", ""},
		{"diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n index 5..6\n", ""},
	}
	for _, tt := range tests {
		if got := newBlobHash(tt.diff); got != tt.want {
			t.Errorf("newBlobHash(%q) = %q, want %q", tt.diff, got, tt.want)
		}
	}
}

func TestFoldsOfAStagedDiffShowTheStagedFile(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	write := func(lines []string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, strings.Repeat("x", i))
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	write(lines)
	run("add", ".")
	run("commit", "-m", "first")

	// Line 2 changed and staged, then a line added at the end, not staged
	lines[1] = "two"
	write(lines)
	run("add", ".")
	write(append(lines, "not staged"))

	staged, err := git.GetStagedDiff("f.txt", repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := getFileTotalLines(staged, "f.txt", repo); got != 20 {
		t.Errorf("getFileTotalLines(staged) = %d, want 20, the lines of the staged file", got)
	}
	if got, want := readFileLines(staged, "f.txt", repo, 19, 21), []string{lines[18], lines[19], ""}; !reflect.DeepEqual(got, want) {
		t.Errorf("readFileLines(staged, 19-21) = %q, want %q", got, want)
	}

	// The diff of the working tree shows the working tree
	unstaged, err := git.GetFileDiff("f.txt", repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := getFileTotalLines(unstaged, "f.txt", repo); got != 21 {
		t.Errorf("getFileTotalLines(unstaged) = %d, want 21", got)
	}
}
