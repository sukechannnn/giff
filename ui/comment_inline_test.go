package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/comments"
)

func TestCommentBlockFitsItsWidth(t *testing.T) {
	cs := []comments.Comment{{
		Status: comments.StatusAnswered,
		Thread: []comments.Message{
			{From: comments.FromYou, Text: "なぜ英語にしたの？"},
			{From: comments.FromClaude, Text: "数字の 10 と 11 だけを英語に書き換えています。他の行は数字のままなので、表記が揃っていません。\n揃えるなら全部英語に。"},
			{From: comments.FromYou, Text: "全部英語にして"},
		},
	}}
	const width = 50
	block := commentBlock(cs, width)
	if len(block) < 5 {
		t.Fatalf("commentBlock() = %q, want the question, the answer and the borders", block)
	}
	for _, line := range block {
		if w := tview.TaggedStringWidth(line); w > width {
			t.Errorf("%q is %d wide, more than %d", line, w, width)
		}
	}
	all := strings.Join(block, "\n")
	// The first answer in full; the reply after it only in the chat
	if !strings.Contains(block[0], "answered") || strings.Count(all, "Q.") != 1 || strings.Count(all, "A.") != 1 || !strings.Contains(all, "揃えるなら全部英語に。") {
		t.Errorf("commentBlock() = %q, want the question and the whole first answer", block)
	}
	if strings.Contains(all, "全部英語にして") || !strings.Contains(all, "1 more message") || !strings.Contains(all, "I:chat") {
		t.Errorf("commentBlock() = %q, want the reply left to the chat, and the key to open it", block)
	}
}

func TestCommentBlockShowsALongAnswerInFull(t *testing.T) {
	answer := strings.Repeat("line\n", 30) + "the end"
	cs := []comments.Comment{{
		Status: comments.StatusAnswered,
		Thread: []comments.Message{{From: comments.FromYou, Text: "why?"}, {From: comments.FromClaude, Text: answer}},
	}}
	if all := strings.Join(commentBlock(cs, 60), "\n"); !strings.Contains(all, "the end") {
		t.Errorf("commentBlock() = %q, want the whole answer", all)
	}
}

func TestCommentChatText(t *testing.T) {
	c := comments.Comment{
		Path: "a.go", StartLine: 3, EndLine: 3, View: "unstaged diff", Code: "+x := 1",
		Status: comments.StatusInProgress,
		Thread: []comments.Message{
			{From: comments.FromYou, Text: "why?"},
			{From: comments.FromClaude, Text: "because"},
			{From: comments.FromYou, Text: "and then?"},
		},
	}
	text := commentChatText(c)
	for _, want := range []string{"a.go:3", "+x := 1", "You", "Claude Code", "because", "and then?", "answering"} {
		if !strings.Contains(text, want) {
			t.Errorf("commentChatText() = %q, want %q in it", text, want)
		}
	}
	if strings.Index(text, "because") > strings.Index(text, "and then?") {
		t.Errorf("commentChatText() = %q, want the messages in order", text)
	}
}

func TestDecorateCommentsInTheFileView(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("GIFF_COMMENTS_DIR", filepath.Join(t.TempDir(), "comments"))
	if _, err := comments.Add(repo, comments.Comment{Path: "a.txt", StartLine: 2, EndLine: 2, Question: "why?"}); err != nil {
		t.Fatal(err)
	}

	content := "one\ntwo\nthree\n"
	path := "a.txt"
	query := ""
	cursorY, selectStart, selectEnd := 0, -1, -1
	selecting, split, leftFocused := false, false, false
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetRect(0, 0, 60, 20)
	ctx := &DiffViewContext{
		diffView:        view,
		currentDiffText: &content,
		currentFile:     &path,
		repoRoot:        repo,
		cursorY:         &cursorY,
		selectStart:     &selectStart,
		selectEnd:       &selectEnd,
		isSelecting:     &selecting,
		isSplitView:     &split,
		leftPaneFocused: &leftFocused,
		foldState:       NewFoldState(),
	}
	ctx.viewUpdater = &FileViewUpdater{diffView: view, filePath: &path, searchQuery: &query}
	registerCommentViews(ctx)
	defer delete(commentHosts, view)

	plain := func() []string { return strings.Split(view.GetText(true), "\n") }

	// The cursor elsewhere: the mark only
	ctx.viewUpdater.UpdateWithCursor(content, 0)
	lines := plain()
	if !strings.Contains(lines[1], "●") || strings.Contains(lines[0], "●") {
		t.Errorf("lines = %q, want the mark on line 2 only", lines)
	}
	if strings.Contains(strings.Join(lines, "\n"), "why?") {
		t.Errorf("lines = %q, want no comment under a line the cursor is not on", lines)
	}

	// The cursor on line 2: the mark only, until i opens the comment
	cursorY = 1
	ctx.viewUpdater.UpdateWithCursor(content, 1)
	if lines = plain(); strings.Contains(strings.Join(lines, "\n"), "why?") {
		t.Errorf("lines = %q, want the comment closed until i", lines)
	}
	ctx.viewUpdater = &FileViewUpdater{diffView: view, filePath: &path, searchQuery: &query}
	toggleCommentAtCursor(ctx)
	lines = plain()
	if !strings.Contains(lines[3], "why?") || !strings.Contains(lines[len(lines)-2], "three") {
		t.Errorf("lines = %q, want the comment between lines 2 and 3", lines)
	}

	// Leaving the line closes it, and coming back does not open it again
	cursorY = 0
	ctx.viewUpdater.UpdateWithCursor(content, 0)
	cursorY = 1
	ctx.viewUpdater.UpdateWithCursor(content, 1)
	if lines = plain(); strings.Contains(strings.Join(lines, "\n"), "why?") {
		t.Errorf("lines = %q, want the comment closed once the cursor left it", lines)
	}

	// i again closes it
	toggleCommentAtCursor(ctx)
	toggleCommentAtCursor(ctx)
	if lines = plain(); strings.Contains(strings.Join(lines, "\n"), "why?") {
		t.Errorf("lines = %q, want the comment closed by i i", lines)
	}

	// Writing a comment under line 1 makes room under it
	ctx.commentEditing = &commentEditing{row: 0, height: 4}
	ctx.viewUpdater.UpdateWithCursor(content, 0)
	lines = plain()
	if !strings.Contains(lines[0], "one") || strings.TrimSpace(lines[1]) != "" || !strings.Contains(lines[5], "two") {
		t.Errorf("lines = %q, want 4 blank rows under line 1", lines)
	}
}
