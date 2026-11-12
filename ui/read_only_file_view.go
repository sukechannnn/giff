package ui

import (
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// readOnlyFileViewState is what the whole file view of a read-only diff
// works on
type readOnlyFileViewState struct {
	app             *tview.Application
	repoRoot        string
	diffView        *tview.TextView
	ctx             *DiffViewContext
	fileListContext *FileListKeyContext

	fileList         *[]FileEntry
	selection        *int
	searchQuery      *string
	searchMatches    *[]int
	searchMatchIndex *int

	rebuildContentFlex     func()
	updateFileListView     func()
	updateSelectedFileDiff func()
	updateGlobalStatus     func(string, string)

	// Set to what puts the diff updater back, for updateSelectedFileDiff to
	// call before it shows a diff
	leaveFileView *func()
}

// setupReadOnlyFileView lets the diff pane of a read-only diff whose after
// side is the working tree show whole files of it: 'f' switches between the
// diff and the whole file under the cursor, 'go' shows a definition, and
// Ctrl+O goes back to where 'go' was pressed. Showing a file of the list
// again puts the diff back. It returns what puts the diff back from the
// whole file, keeping the diff pane's cursor where it was.
func setupReadOnlyFileView(s readOnlyFileViewState) func() {
	ctx := s.ctx
	diffTitle := s.diffView.GetTitle()

	// What the diff pane was before it showed a whole file
	on := false
	var updaterBefore DiffViewUpdater
	splitBefore := false
	cursorBefore := 0

	// Where 'go' was pressed, for Ctrl+O to go back to: a row of a whole
	// file, or of the diff
	type jumpPoint struct {
		path     string
		row      int
		fileView bool
	}
	var history []jumpPoint

	clearSearch := func() {
		*s.searchQuery = ""
		*s.searchMatches = nil
		*s.searchMatchIndex = -1
	}

	leave := func() {
		if !on {
			return
		}
		on = false
		history = nil
		clearSearch()
		s.diffView.SetTitle(diffTitle)
		ctx.viewUpdater = updaterBefore
		if splitBefore {
			*ctx.isSplitView = true
			s.rebuildContentFlex()
		}
	}
	*s.leaveFileView = leave

	backToDiff := func() {
		if !on {
			return
		}
		inDiffPane := !*ctx.leftPaneFocused
		s.updateSelectedFileDiff()
		if inDiffPane {
			*ctx.cursorY = cursorBefore
			ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
			if *ctx.isSplitView {
				s.app.SetFocus(ctx.splitViewFlex)
			} else {
				s.app.SetFocus(s.diffView)
			}
		}
	}

	// show shows line (1-based) of path, as it is in the working tree
	show := func(path string, line int) bool {
		var content string
		isBinary, err := util.IsBinaryFile(path, s.repoRoot)
		if err == nil {
			if isBinary {
				content = util.FormatBinaryNotice(path)
			} else {
				content, err = util.ReadFileContent(path, s.repoRoot)
			}
		}
		if err != nil {
			s.updateGlobalStatus("Cannot open "+tview.Escape(path)+": it is not in the working tree", "tomato")
			return false
		}

		*ctx.leftPaneFocused = false
		if !on {
			on = true
			updaterBefore = ctx.viewUpdater
			splitBefore = *ctx.isSplitView
			cursorBefore = *ctx.cursorY
			// The whole file is drawn in the unified pane
			if splitBefore {
				*ctx.isSplitView = false
				s.rebuildContentFlex()
			}
			ctx.viewUpdater = &FileViewUpdater{
				diffView:    s.diffView,
				filePath:    ctx.currentFile,
				searchQuery: s.searchQuery,
			}
		}
		clearSearch()
		*ctx.currentFile = path
		*ctx.currentDiffText = content
		*ctx.isSelecting = false
		*ctx.selectStart = -1
		*ctx.selectEnd = -1
		*ctx.cursorY = max(min(line-1, len(fileViewLines(content))-1), 0)
		s.diffView.SetTitle(util.ColoredTitle(util.PaneTitleColor, path+" (f: back to diff)"))

		// The list points at the file when it is one of the diff's
		for i, entry := range *s.fileList {
			if !entry.IsDirectory && entry.Path == path {
				*s.selection = i
				break
			}
		}
		s.updateFileListView()
		ctx.viewUpdater.UpdateWithCursor(content, *ctx.cursorY)
		s.app.SetFocus(s.diffView)
		return true
	}

	ctx.toggleFileView = func() {
		if on {
			backToDiff()
		} else if *ctx.currentFile != "" {
			show(*ctx.currentFile, getCursorFileLineNumber(ctx))
		}
	}

	ctx.jumpTo = func(loc git.Location) {
		from := jumpPoint{path: *ctx.currentFile, row: *ctx.cursorY, fileView: on}
		if show(loc.Path, loc.Line) {
			history = append(history, from)
		}
	}

	ctx.jumpBack = func() bool {
		if len(history) == 0 {
			return false
		}
		back := history[len(history)-1]
		history = history[:len(history)-1]
		if back.fileView {
			show(back.path, back.row+1)
		} else {
			backToDiff()
		}
		return true
	}

	return backToDiff
}
