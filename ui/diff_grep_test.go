package ui

import (
	"strings"
	"testing"

	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// filePathsOf returns the non-directory entries of a rendered file list.
func filePathsOf(fileList []FileEntry) []string {
	var paths []string
	for _, entry := range fileList {
		if !entry.IsDirectory {
			paths = append(paths, entry.Path)
		}
	}
	return paths
}

func TestBuildFileListContentWithDiffGrep(t *testing.T) {
	staged := []git.FileInfo{{Path: "ui/hit.go", ChangeStatus: "modified"}}
	modified := []git.FileInfo{
		{Path: "ui/miss.go", ChangeStatus: "modified"},
		{Path: "ui/also_hit.go", ChangeStatus: "modified"},
	}
	untracked := []git.FileInfo{{Path: "fresh.txt", ChangeStatus: "untracked"}}

	build := func(filterQuery string, grepHits map[string]int) (string, []FileEntry) {
		var fileList []FileEntry
		content := BuildFileListContent(staged, modified, untracked, 0, true,
			&fileList, make(map[int]int), NewDirCollapseState(), filterQuery, grepHits)
		return stripTviewTags(content), fileList
	}

	t.Run("keeps every file when no grep is active", func(t *testing.T) {
		content, fileList := build("", nil)
		if got := filePathsOf(fileList); len(got) != 4 {
			t.Errorf("got %v, want all 4 files", got)
		}
		if strings.Contains(content, "[") {
			t.Errorf("hit counts must not appear without a grep: %q", content)
		}
	})

	t.Run("keeps only matching files and shows their hit counts", func(t *testing.T) {
		content, fileList := build("", map[string]int{
			git.DiffGrepKey(git.GrepStageStaged, "ui/hit.go"):        3,
			git.DiffGrepKey(git.GrepStageUnstaged, "ui/also_hit.go"): 1,
			git.DiffGrepKey(git.GrepStageUntracked, "fresh.txt"):     2,
		})

		got := filePathsOf(fileList)
		want := []string{"ui/hit.go", "ui/also_hit.go", "fresh.txt"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for _, path := range want {
			found := false
			for _, g := range got {
				found = found || g == path
			}
			if !found {
				t.Errorf("%s missing from %v", path, got)
			}
		}

		for _, want := range []string{"hit.go (•) [3]", "also_hit.go (•) [1]", "fresh.txt (+) [2]"} {
			if !strings.Contains(content, want) {
				t.Errorf("rendered list missing %q:\n%s", want, content)
			}
		}
	})

	t.Run("combines the grep with an active file name filter", func(t *testing.T) {
		_, fileList := build("*.txt", map[string]int{
			git.DiffGrepKey(git.GrepStageStaged, "ui/hit.go"):    3,
			git.DiffGrepKey(git.GrepStageUntracked, "fresh.txt"): 2,
		})
		if got := filePathsOf(fileList); len(got) != 1 || got[0] != "fresh.txt" {
			t.Errorf("got %v, want only fresh.txt", got)
		}
	})

	t.Run("tells the staged and unstaged copy of one file apart", func(t *testing.T) {
		// A file changed both in the index and in the working tree appears in
		// both sections; only the section whose diff matched may survive.
		dual := []git.FileInfo{{Path: "dual.go", ChangeStatus: "modified"}}
		var fileList []FileEntry
		BuildFileListContent(dual, dual, nil, 0, true,
			&fileList, make(map[int]int), NewDirCollapseState(), "",
			map[string]int{git.DiffGrepKey(git.GrepStageUnstaged, "dual.go"): 1})

		if got := filePathsOf(fileList); len(got) != 1 {
			t.Fatalf("got %v, want the unstaged copy only", got)
		}
		if fileList[0].StageStatus != "unstaged" {
			t.Errorf("StageStatus = %q, want unstaged", fileList[0].StageStatus)
		}
	})
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	content := &UnifiedViewContent{Lines: []UnifiedViewLine{
		{LineNumber: "1", Content: `[green]+Logger.Debug("x")[-]`},
		{LineNumber: "2", Content: `[white] untouched line[-]`},
	}}

	matches := searchInUnifiedContent(content, "logger")
	if len(matches) != 1 || matches[0] != 0 {
		t.Errorf("searchInUnifiedContent() = %v, want [0]", matches)
	}

	highlighted := highlightSearchInTaggedText(content.Lines[0].Content, "logger")
	if !strings.Contains(highlighted, util.SearchHighlightBg) {
		t.Errorf("expected a highlight in %q", highlighted)
	}
	if stripped := stripTviewTags(highlighted); stripped != `+Logger.Debug("x")` {
		t.Errorf("highlighting changed the text: %q", stripped)
	}
}

func TestGetCursorFileLineNumberUsesNewFileNumbers(t *testing.T) {
	diff := `diff --git a/f.go b/f.go
index 1111111..2222222 100644
--- a/f.go
+++ b/f.go
@@ -10,6 +10,6 @@ func Foo() {
 ctx1
 ctx2
-removed A
+added C
 ctx3
 ctx4
@@ -40,3 +40,4 @@ func Bar() {
 ctx5
+added D
 ctx6
`
	// Unified view layout, with the leading and middle gaps folded shut:
	//   0  fold indicator
	//   1  ctx1        new 10
	//   2  ctx2        new 11
	//   3  -removed A  gone from the new file
	//   4  +added C    new 12
	//   5  ctx3        new 13
	//   6  ctx4        new 14
	//   7  fold indicator
	//   8  ctx5        new 40
	//   9  +added D    new 41
	//  10  ctx6        new 42
	cases := []struct {
		name   string
		cursor int
		want   int
	}{
		{"context line", 1, 10},
		{"deleted line resolves to where the change landed", 3, 12},
		{"added line", 4, 12},
		{"context line after the change", 5, 13},
		{"line in the second hunk", 9, 41},
		{"fold indicator uses the diff line next to it", 7, 40},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cursorY := tt.cursor
			splitView := false
			filePath := "f.go"
			diffText := diff
			ctx := &DiffViewContext{
				currentDiffText: &diffText,
				currentFile:     &filePath,
				cursorY:         &cursorY,
				isSplitView:     &splitView,
				foldState:       NewFoldState(),
			}
			if got := getCursorFileLineNumber(ctx); got != tt.want {
				t.Errorf("getCursorFileLineNumber(cursor %d) = %d, want %d", tt.cursor, got, tt.want)
			}
		})
	}
}

func TestGetCursorFileLineNumberAfterLargeDeletion(t *testing.T) {
	// Twenty removed lines push the old file's numbering far ahead of the new
	// file's. The deleted line the cursor sits on is displayed with its old
	// number (22), but the file on disk only has three lines, so an editor
	// opened there would land nowhere near the change.
	diff := "diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -1,23 +1,2 @@\n ctx1\n"
	for i := 0; i < 20; i++ {
		diff += "-dropped line\n"
	}
	diff += "-needle here\n ctx2\n"

	cursorY := 21 // the "-needle here" line
	splitView := false
	filePath := "f.go"
	ctx := &DiffViewContext{
		currentDiffText: &diff,
		currentFile:     &filePath,
		cursorY:         &cursorY,
		isSplitView:     &splitView,
		foldState:       NewFoldState(),
	}
	if got := getCursorFileLineNumber(ctx); got != 2 {
		t.Errorf("getCursorFileLineNumber() = %d, want 2 (the following context line in the new file)", got)
	}
}
