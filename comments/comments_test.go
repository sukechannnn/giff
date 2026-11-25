package comments

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes an empty Git repository and keeps the comments of the test in a
// directory of their own
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIFF_COMMENTS_DIR", filepath.Join(t.TempDir(), "comments"))
	return initRepo(t)
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return repo
}

func TestCommentsAreKeptPerRepository(t *testing.T) {
	repoA := newRepo(t)
	repoB := initRepo(t)

	if _, err := Add(repoA, Comment{Path: "a.go", StartLine: 3, EndLine: 3, Question: "what is this?"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	pathA, _ := PathFor(repoA)
	pathB, _ := PathFor(repoB)
	if pathA == pathB {
		t.Fatalf("two repositories share the comments file %s", pathA)
	}
	if n, err := Next(repoB); err != nil || n != nil {
		t.Errorf("Next() of another repository = %+v, %v, want none", n, err)
	}
	if n, err := Next(repoA); err != nil || n == nil || n.Question != "what is this?" {
		t.Errorf("Next() = %+v, %v, want the comment", n, err)
	}

	// A subdirectory and a symbolic link reach the same comments
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repoA, link); err != nil {
		t.Fatal(err)
	}
	if p, _ := PathFor(link); p != pathA {
		t.Errorf("PathFor(link) = %s, want %s", p, pathA)
	}
}

func TestCommentLifecycle(t *testing.T) {
	repo := newRepo(t)
	first, _ := Add(repo, Comment{Path: "a.go", StartLine: 1, EndLine: 2, Question: "first"})
	second, _ := Add(repo, Comment{Path: "b.go", StartLine: 5, EndLine: 5, Question: "second"})

	n, _ := Next(repo)
	if n == nil || n.ID != first.ID {
		t.Fatalf("Next() = %+v, want the first comment", n)
	}
	// Picked up but not answered, it is handed out again
	if again, _ := Next(repo); again == nil || again.ID != first.ID {
		t.Fatalf("Next() again = %+v, want the first comment still", again)
	}
	if err := SetAnswer(repo, first.ID, "an answer"); err != nil {
		t.Fatalf("SetAnswer() error = %v", err)
	}
	if n, _ := Next(repo); n == nil || n.ID != second.ID {
		t.Fatalf("Next() = %+v, want the second comment", n)
	}

	f, _ := Read(repo)
	if got := f.Comments[0]; got.Status != StatusAnswered || got.Answer != "an answer" || got.AnsweredAt == nil {
		t.Errorf("first comment = %+v, want it answered", got)
	}
	if err := SetAnswer(repo, "nope", "x"); err != ErrNotFound {
		t.Errorf("SetAnswer(unknown) error = %v, want ErrNotFound", err)
	}
	if removed, _ := RemoveIf(repo, func(n Comment) bool { return n.Status == StatusAnswered }); removed != 1 {
		t.Errorf("RemoveIf() removed %d, want 1", removed)
	}
	if err := Remove(repo, second.ID); err != nil {
		t.Errorf("Remove() error = %v", err)
	}
	if f, _ := Read(repo); len(f.Comments) != 0 {
		t.Errorf("comments left = %+v, want none", f.Comments)
	}
}

func TestReplyAndOldComments(t *testing.T) {
	repo := newRepo(t)
	c, _ := Add(repo, Comment{Path: "a.go", StartLine: 1, EndLine: 1, Question: "why?"})
	Next(repo)
	SetAnswer(repo, c.ID, "because")
	if err := Reply(repo, c.ID, "and then?"); err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	n, _ := Next(repo)
	if n == nil || n.ID != c.ID || n.Question != "and then?" || n.Answer != "" || len(n.Thread) != 3 {
		t.Fatalf("Next() after a reply = %+v, want the reply to answer, with the thread", n)
	}
	if n.Thread[0].From != FromYou || n.Thread[1].From != FromClaude || n.Thread[1].Text != "because" || n.Thread[2].Text != "and then?" {
		t.Errorf("thread = %+v", n.Thread)
	}
	if err := Reply(repo, "nope", "x"); err != ErrNotFound {
		t.Errorf("Reply(unknown) error = %v, want ErrNotFound", err)
	}

	// A comment kept before threads gets one from its question and answer
	path, _ := PathFor(repo)
	old := `{"repo":"r","comments":[{"id":"x1","path":"b.go","startLine":2,"endLine":2,"question":"q?","status":"answered","answer":"a."}]}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(repo)
	if err != nil {
		t.Fatal(err)
	}
	if th := f.Comments[0].Thread; len(th) != 2 || th[0].Text != "q?" || th[1].From != FromClaude || th[1].Text != "a." {
		t.Errorf("thread of an old comment = %+v", th)
	}
}

func TestCLI(t *testing.T) {
	repo := newRepo(t)
	comment, _ := Add(repo, Comment{Path: "a.go", StartLine: 7, EndLine: 9, View: "unstaged diff", Code: "+x := 1", Question: "why?"})

	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	run := func(stdin string, args ...string) (int, string) {
		var out, errOut bytes.Buffer
		code := Run(args, strings.NewReader(stdin), &out, &errOut)
		return code, out.String() + errOut.String()
	}

	code, out := run("", "next")
	if code != 0 {
		t.Fatalf("next = %d %s", code, out)
	}
	var got commentJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("next printed %q: %v", out, err)
	}
	if got.ID != comment.ID || got.Path != "a.go" || got.StartLine != 7 || got.Code != "+x := 1" || got.Repo == "" {
		t.Errorf("next = %+v", got)
	}

	if code, out := run("because\nof this\n", "answer", comment.ID); code != 0 {
		t.Fatalf("answer = %d %s", code, out)
	}
	f, _ := Read(repo)
	if f.Comments[0].Answer != "because\nof this" {
		t.Errorf("answer = %q", f.Comments[0].Answer)
	}
	if code, out := run("", "list"); code != 0 || !strings.Contains(out, "No comments") {
		t.Errorf("list = %d %q, want no comments left to answer", code, out)
	}
	if code, out := run("", "list", "--all"); code != 0 || !strings.Contains(out, "a.go:7-9") {
		t.Errorf("list --all = %d %q", code, out)
	}
	if code, _ := run("", "next"); code != 1 {
		t.Errorf("next with nothing pending = %d, want 1", code)
	}
	if code, out := run("", "clear"); code != 0 || !strings.Contains(out, "Removed 1") {
		t.Errorf("clear = %d %q", code, out)
	}
	if code, _ := run("", "bogus"); code != 2 {
		t.Errorf("unknown command = %d, want 2", code)
	}
}
