package ui

import (
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

// readOnlyReloadState is what a read-only view shares with its reload: the
// file list and diff to swap out, and the redraws that follow
type readOnlyReloadState struct {
	app    *tview.Application
	reload func() ([]FileEntry, error)

	files       *[]FileEntry // every file of the diff, before any narrowing
	fileList    *[]FileEntry // the rows of the file list
	selection   *int
	currentFile *string
	diffText    *string
	cursorY     *int
	isSelecting *bool
	selectStart *int
	selectEnd   *int
	leftFocused *bool

	fileListView    *tview.TextView
	diffContext     *DiffViewContext
	fileListContext *FileListKeyContext

	fetchDiff              func(filePath string) string
	regrep                 func() // re-runs an active '/' grep against the new diff
	updateFileListView     func()
	updateSelectedFileDiff func()
	updateGlobalStatus     func(string, string)
	restoreStatus          func()
}

// setupReadOnlyReload binds 'r' to re-read the view's file list and the
// open file's diff
func setupReadOnlyReload(s readOnlyReloadState) {
	reloadNow := func() {
		newFiles, err := s.reload()
		if err != nil {
			s.updateGlobalStatus("Failed to reload: "+err.Error(), "tomato")
			return
		}
		var newDiff string
		if *s.currentFile != "" {
			newDiff = s.fetchDiff(*s.currentFile)
		}
		applyReadOnlyReload(s, newFiles, newDiff)
		s.updateGlobalStatus("Reloaded", "forestgreen")
	}
	s.diffContext.reload = reloadNow
	s.fileListContext.reload = reloadNow
}

// applyReadOnlyReload swaps in a re-read file list and diff. The selection
// stays on the same file, and so does the diff pane's cursor when the diff
// pane has the focus.
func applyReadOnlyReload(s readOnlyReloadState, newFiles []FileEntry, newDiff string) {
	selectedPath := ""
	if *s.selection >= 0 && *s.selection < len(*s.fileList) {
		selectedPath = (*s.fileList)[*s.selection].Path
	}

	*s.files = newFiles
	s.regrep()
	s.updateFileListView()

	newSelection := -1
	for i, entry := range *s.fileList {
		if entry.Path == selectedPath {
			newSelection = i
			break
		}
	}

	if newSelection < 0 || *s.leftFocused {
		if newSelection >= 0 {
			*s.selection = newSelection
		} else {
			// The file left the diff: fall back to the file list.
			if *s.selection >= len(*s.fileList) {
				*s.selection = len(*s.fileList) - 1
			}
			if *s.selection < 0 {
				*s.selection = 0
			}
			if !*s.leftFocused {
				*s.leftFocused = true
				s.app.SetFocus(s.fileListView)
				s.restoreStatus()
			}
		}
		s.updateFileListView()
		s.updateSelectedFileDiff()
		return
	}

	*s.selection = newSelection
	s.updateFileListView()
	if newDiff == *s.diffText {
		return
	}
	*s.diffText = newDiff
	lineCount := len(util.SplitLines(ColorizeDiff(newDiff)))
	if *s.cursorY >= lineCount {
		*s.cursorY = max(lineCount-1, 0)
	}
	*s.isSelecting = false
	*s.selectStart = -1
	*s.selectEnd = -1
	if s.diffContext.viewUpdater != nil {
		s.diffContext.viewUpdater.UpdateWithCursor(newDiff, *s.cursorY)
	}
}
