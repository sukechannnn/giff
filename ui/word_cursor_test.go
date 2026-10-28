package ui

import (
	"reflect"
	"testing"

	"github.com/rivo/tview"
)

func TestLineWords(t *testing.T) {
	tests := []struct {
		plain string
		want  []string
	}{
		{"+\tfoo := bar.Baz(qux_1)", []string{"foo", "bar", "Baz", "qux_1"}},
		{"  // café, naïve", []string{"café", "naïve"}},
		{"変数などをカーソルで", []string{"変数", "などを", "カーソル", "で"}},
		{"-- !!", nil},
	}
	for _, tt := range tests {
		var got []string
		for _, w := range lineWords(tt.plain, splitBefore) {
			got = append(got, w.text)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("lineWords(%q) = %q, want %q", tt.plain, got, tt.want)
		}
	}
}

// newWordCursorContext returns a unified diff view context on a small diff
func newWordCursorContext(diff string) *DiffViewContext {
	cursorY, selectStart, selectEnd := 0, -1, -1
	isSelecting, splitView := false, false
	filePath := "f.go"
	searchQuery, searchInput := "", ""
	var searchMatches []int
	searchMatchIndex, cursorBeforeSearch := -1, 0
	foldState := NewFoldState()
	diffView := tview.NewTextView()
	diffView.SetRect(0, 0, 80, 10)
	return &DiffViewContext{
		diffView:                  diffView,
		currentDiffText:           &diff,
		currentFile:               &filePath,
		cursorY:                   &cursorY,
		selectStart:               &selectStart,
		selectEnd:                 &selectEnd,
		isSelecting:               &isSelecting,
		isSplitView:               &splitView,
		foldState:                 foldState,
		searchQuery:               &searchQuery,
		searchInput:               &searchInput,
		searchMatches:             &searchMatches,
		searchMatchIndex:          &searchMatchIndex,
		searchCursorYBeforeSearch: &cursorBeforeSearch,
		setGlobalStatusText:       func(string) {},
		viewUpdater: &UnifiedViewUpdater{
			diffView:    diffView,
			foldState:   foldState,
			filePath:    &filePath,
			searchQuery: &searchQuery,
		},
	}
}

func TestMoveWordCursorCrossesRows(t *testing.T) {
	ctx := newWordCursorContext("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,3 @@\n foo(bar)\n-\n+baz := 1\n")

	type stop struct {
		row  int
		text string
	}
	var got []stop
	for i := 0; i < 5; i++ {
		moveWordCursor(ctx, 1)
		got = append(got, stop{*ctx.cursorY, ctx.word.text})
	}
	// The empty "-" row has no words, and the last word stays put.
	want := []stop{{0, "foo"}, {0, "bar"}, {2, "baz"}, {2, "1"}, {2, "1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("n stops = %v, want %v", got, want)
	}

	moveWordCursor(ctx, -1)
	moveWordCursor(ctx, -1)
	if *ctx.cursorY != 0 || ctx.word.text != "bar" {
		t.Errorf("N N = row %d %q, want row 0 \"bar\"", *ctx.cursorY, ctx.word.text)
	}
}

func TestMoveWordCursorRestartsAfterCursorMoves(t *testing.T) {
	ctx := newWordCursorContext("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n foo bar\n qux quux\n")
	moveWordCursor(ctx, 1)
	moveWordCursor(ctx, 1) // on "bar"
	*ctx.cursorY = 1       // j
	moveWordCursor(ctx, 1)
	if ctx.word.text != "qux" {
		t.Errorf("n after moving rows = %q, want the row's first word \"qux\"", ctx.word.text)
	}
}

func TestWordCursorSearchesItsWord(t *testing.T) {
	ctx := newWordCursorContext("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,4 +1,4 @@\n foo bar\n x\n call(Bar, y)\n bar\n")
	moveWordCursor(ctx, 1)
	moveWordCursor(ctx, 1) // on "bar"

	// Landing on a word searches for it, so its occurrences are highlighted.
	if *ctx.searchQuery != "bar" {
		t.Fatalf("searchQuery = %q, want \"bar\"", *ctx.searchQuery)
	}
	if want := []int{0, 2, 3}; !reflect.DeepEqual(*ctx.searchMatches, want) {
		t.Errorf("searchMatches = %v, want %v", *ctx.searchMatches, want)
	}
	if *ctx.searchMatchIndex != 0 {
		t.Errorf("searchMatchIndex = %d, want 0 (the word's own row)", *ctx.searchMatchIndex)
	}

	// Enter jumps to the next match, and n goes on from the word found there.
	nextWordMatch(ctx)
	if *ctx.cursorY != 2 || *ctx.searchMatchIndex != 1 || !wordCursorActive(ctx) {
		t.Fatalf("Enter = row %d, match %d, active %v; want row 2, match 1, active", *ctx.cursorY, *ctx.searchMatchIndex, wordCursorActive(ctx))
	}
	moveWordCursor(ctx, 1)
	if *ctx.cursorY != 2 || ctx.word.text != "y" {
		t.Errorf("n after Enter = row %d %q, want row 2 \"y\" (the word after \"Bar\")", *ctx.cursorY, ctx.word.text)
	}

	// Esc drops the search along with the word cursor.
	ctx.restoreStatus = func() {}
	clearWordCursor(ctx)
	if wordCursorActive(ctx) || *ctx.searchQuery != "" || *ctx.searchMatches != nil {
		t.Errorf("after Esc: active %v, query %q, matches %v; want all cleared", wordCursorActive(ctx), *ctx.searchQuery, *ctx.searchMatches)
	}
}

func TestScrollKeepsWordCursor(t *testing.T) {
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,4 +1,4 @@\n foo\n bar foo\n baz\n qux\n"

	// The cursor stays where it is: so does the word cursor and its search.
	ctx := newWordCursorContext(diff)
	*ctx.cursorY = 2
	moveWordCursor(ctx, 1) // on "baz"
	scrollKeepingWordCursor(ctx, 1)
	if !wordCursorActive(ctx) || ctx.word.text != "baz" || *ctx.searchQuery != "baz" {
		t.Errorf("after Ctrl+E: active %v, word %q, query %q; want the word cursor on \"baz\"", wordCursorActive(ctx), ctx.word.text, *ctx.searchQuery)
	}

	// The cursor follows the scroll down a row: the word cursor goes along,
	// still searching for its word.
	scrolled := func() *DiffViewContext {
		ctx := newWordCursorContext(diff)
		moveWordCursor(ctx, 1)      // on "foo" in row 0
		ctx.diffView.ScrollTo(0, 0) // an undrawn TextView reports no scroll offset
		scrollKeepingWordCursor(ctx, 1)
		if *ctx.cursorY != 1 || !wordCursorActive(ctx) || *ctx.searchQuery != "foo" {
			t.Fatalf("after Ctrl+E: row %d, active %v, query %q; want row 1 with the word cursor searching \"foo\"", *ctx.cursorY, wordCursorActive(ctx), *ctx.searchQuery)
		}
		return ctx
	}

	// n starts from the new row's first word.
	ctx = scrolled()
	moveWordCursor(ctx, 1)
	if *ctx.cursorY != 1 || ctx.word.text != "bar" {
		t.Errorf("n after Ctrl+E = row %d %q, want row 1 \"bar\"", *ctx.cursorY, ctx.word.text)
	}

	// Enter finds the word in the new row before going any further.
	ctx = scrolled()
	nextWordMatch(ctx)
	if *ctx.cursorY != 1 || ctx.word.start != 5 {
		t.Errorf("Enter after Ctrl+E = row %d at %d, want row 1 at 5 (its \"foo\")", *ctx.cursorY, ctx.word.start)
	}
}
