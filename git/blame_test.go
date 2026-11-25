package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBlame(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_AUTHOR_DATE=2026-01-02T03:04:05Z",
			"GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=a@example.com", "GIT_COMMITTER_DATE=2026-01-02T03:04:05Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write("one\ntwo\n")
	run("add", ".")
	run("commit", "-qm", "first lines")
	write("one\ntwo\nthree\n")

	lines, err := Blame(repo, "a.txt")
	if err != nil {
		t.Fatalf("Blame() error = %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("Blame() returned %d lines, want 3", len(lines))
	}
	for i, l := range lines[:2] {
		if l.Uncommitted() || l.Author != "Alice" || l.Summary != "first lines" || l.Time.UTC().Format("2006-01-02") != "2026-01-02" {
			t.Errorf("line %d = %+v, want Alice's first lines of 2026-01-02", i+1, l)
		}
	}
	if !lines[2].Uncommitted() {
		t.Errorf("line 3 = %+v, want not committed yet", lines[2])
	}

	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Blame(repo, "new.txt"); err == nil {
		t.Error("Blame() of an untracked file succeeded, want an error")
	}
}
