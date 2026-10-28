package ui

import (
	"testing"

	"github.com/sukechannnn/giff/git"
)

// A file name filter in the file browser opens nested directories too, not
// only the top-level ones that are visible while everything is collapsed.
func TestExpandAllDirectoriesOpensNestedDirectories(t *testing.T) {
	files := []git.FileInfo{
		{Path: "ui/commands/a.go"},
		{Path: "ui/root_editor.go"},
		{Path: "main.go"},
	}

	var fileList []FileEntry
	lineNumberMap := make(map[int]int)
	collapseState := NewDirCollapseState()
	filterQuery := ""
	build := func() {
		BuildFileListContentForBrowser(files, 0, true, &fileList, lineNumberMap, collapseState, filterQuery)
	}

	// The browser starts with every directory collapsed.
	build()
	for _, dir := range []string{"ui", "ui/commands"} {
		collapseState.SetCollapsed("browser", dir, true)
	}
	build()

	filterQuery = "a.go"
	build()
	expandAllDirectories(&FileListKeyContext{
		fileList:           &fileList,
		dirCollapseState:   collapseState,
		updateFileListView: build,
	})

	var paths []string
	for _, entry := range fileList {
		paths = append(paths, entry.Path)
	}
	want := []string{"ui", "ui/commands", "ui/commands/a.go"}
	if len(paths) != len(want) {
		t.Fatalf("file list = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("file list = %v, want %v", paths, want)
		}
	}
}
