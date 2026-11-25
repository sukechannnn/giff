package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetChangedFilesRenamedThenModified(t *testing.T) {
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
			t.Fatal(err)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	write("old.tf", "a\nb\nc\nd\ne\n")
	write("gone.tf", "1\n2\n3\n4\n5\n")
	run("add", ".")
	run("commit", "-m", "first")

	// Renamed and staged, then changed again: git status reads RM and RD
	run("mv", "old.tf", "new.tf")
	run("mv", "gone.tf", "moved.tf")
	write("new.tf", "a\nb\nc\nd\ne\nf\n")
	if err := os.Remove(filepath.Join(repo, "moved.tf")); err != nil {
		t.Fatal(err)
	}

	staged, modified, untracked, err := GetChangedFiles(repo)
	if err != nil {
		t.Fatalf("GetChangedFiles() error = %v", err)
	}
	wantStaged := []FileInfo{
		{Path: "gone.tf", ChangeStatus: "deleted"},
		{Path: "moved.tf", ChangeStatus: "added"},
		{Path: "old.tf", ChangeStatus: "deleted"},
		{Path: "new.tf", ChangeStatus: "added"},
	}
	wantModified := []FileInfo{
		{Path: "moved.tf", ChangeStatus: "deleted"},
		{Path: "new.tf", ChangeStatus: "modified"},
	}
	if !reflect.DeepEqual(staged, wantStaged) {
		t.Errorf("staged = %v, want %v", staged, wantStaged)
	}
	if !reflect.DeepEqual(modified, wantModified) {
		t.Errorf("modified = %v, want %v", modified, wantModified)
	}
	if len(untracked) != 0 {
		t.Errorf("untracked = %v, want none", untracked)
	}
}
