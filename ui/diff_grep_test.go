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
