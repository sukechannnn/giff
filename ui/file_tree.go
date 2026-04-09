package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/ui/commands"
)

// moveFileListSelection moves the file list selection.
// If currently on a directory, moves between directories. If on a file, moves between files.
// matchesFilter checks if a file entry matches the current filter query
func matchesFilter(entry FileEntry, filterQuery string) bool {
	if filterQuery == "" {
		return true
	}
	return !entry.IsDirectory && strings.Contains(strings.ToLower(entry.Path), strings.ToLower(filterQuery))
}

func moveFileListSelection(ctx *FileListKeyContext, direction int) {
	if *ctx.currentSelection < 0 || *ctx.currentSelection >= len(*ctx.fileList) {
		return
	}
	onDirectory := (*ctx.fileList)[*ctx.currentSelection].IsDirectory

	next := *ctx.currentSelection + direction
	for next >= 0 && next < len(*ctx.fileList) {
		entry := (*ctx.fileList)[next]
		if entry.IsDirectory == onDirectory {
			// Skip entries that don't match filter
			if *ctx.filterQuery != "" && !matchesFilter(entry, *ctx.filterQuery) {
				next += direction
				continue
			}
			*ctx.currentSelection = next
			ctx.updateFileListView()
			if !onDirectory {
				// Debounce diff update: cancel previous timer and start new one
				if ctx.diffDebounceTimer != nil {
					ctx.diffDebounceTimer.Stop()
				}
				ctx.diffDebounceTimer = time.AfterFunc(80*time.Millisecond, func() {
					ctx.app.QueueUpdateDraw(func() {
						ctx.updateSelectedFileDiff()
					})
				})
			}
			return
		}
		next += direction
	}

	// If moving up on a file and no more files found, scroll to top
	if !onDirectory && direction < 0 {
		ctx.fileListView.ScrollTo(0, 0)
	}
}

// findParentDirectory finds the parent directory entry for the given entry
func findParentDirectory(fileList *[]FileEntry, currentIdx int) int {
	entry := (*fileList)[currentIdx]
	for i := currentIdx - 1; i >= 0; i-- {
		candidate := (*fileList)[i]
		if candidate.IsDirectory && candidate.StageStatus == entry.StageStatus &&
			strings.HasPrefix(entry.Path, candidate.Path+"/") {
			return i
		}
	}
	return -1
}

// handleFileListLeft handles left/H key: move to parent directory
func handleFileListLeft(ctx *FileListKeyContext) {
	if *ctx.currentSelection < 0 || *ctx.currentSelection >= len(*ctx.fileList) {
		return
	}
	if parent := findParentDirectory(ctx.fileList, *ctx.currentSelection); parent >= 0 {
		*ctx.currentSelection = parent
		ctx.updateFileListView()
	}
}

// handleFileListRight handles right/l key (VSCode-like):
// collapsed dir → expand, expanded dir → first child, file → no-op
func handleFileListRight(ctx *FileListKeyContext) {
	if *ctx.currentSelection < 0 || *ctx.currentSelection >= len(*ctx.fileList) {
		return
	}
	fileEntry := (*ctx.fileList)[*ctx.currentSelection]

	if !fileEntry.IsDirectory || ctx.dirCollapseState == nil {
		return
	}

	if ctx.dirCollapseState.IsCollapsed(fileEntry.StageStatus, fileEntry.Path) {
		// If collapsed, expand it
		ctx.dirCollapseState.SetCollapsed(fileEntry.StageStatus, fileEntry.Path, false)
		ctx.updateFileListView()
		return
	}

	// Already expanded -> move to next entry (first child)
	if *ctx.currentSelection+1 < len(*ctx.fileList) {
		*ctx.currentSelection = *ctx.currentSelection + 1
		ctx.updateFileListView()
		if !(*ctx.fileList)[*ctx.currentSelection].IsDirectory {
			ctx.updateSelectedFileDiff()
		}
	}
}

// buildFileTree converts a list of file paths into a tree structure
func buildFileTree(files []git.FileInfo) *TreeNode {
	return buildFileTreeFromGitFiles(files)
}

// renderFileTree recursively renders the tree structure with proper indentation
func renderFileTree(
	node *TreeNode,
	depth int,
	sb *strings.Builder,
	fileList *[]FileEntry,
	stageStatus string,
	regionIndex *int,
	currentSelection int,
	focusedPane bool,
	lineNumberMap map[int]int,
	currentLine *int,
	fileInfos []git.FileInfo,
	collapseState *DirCollapseState,
) {
	// Build status map for O(1) lookup
	statusMap := make(map[string]string, len(fileInfos))
	for _, fi := range fileInfos {
		statusMap[fi.Path] = fi.ChangeStatus
	}
	renderFileTreeForGitFiles(
		node,
		depth,
		"",
		sb,
		fileList,
		stageStatus,
		regionIndex,
		currentSelection,
		focusedPane,
		lineNumberMap,
		currentLine,
		fileInfos,
		collapseState,
		statusMap,
	)
}

// BuildFileListContent builds the colored file list content
func BuildFileListContent(
	stagedFiles, modifiedFiles, untrackedFiles []git.FileInfo,
	currentSelection int,
	focusedPane bool,
	fileList *[]FileEntry,
	lineNumberMap map[int]int,
	collapseState *DirCollapseState,
	filterQuery string,
) string {
	// Rebuild fileList
	// Clear slice contents (keep the reference)
	*fileList = (*fileList)[:0]
	for k := range lineNumberMap {
		delete(lineNumberMap, k)
	}

	// Filter files if query is set
	filterFn := func(files []git.FileInfo) []git.FileInfo {
		if filterQuery == "" {
			return files
		}
		q := strings.ToLower(filterQuery)
		var filtered []git.FileInfo
		for _, f := range files {
			if strings.Contains(strings.ToLower(f.Path), q) {
				filtered = append(filtered, f)
			}
		}
		return filtered
	}
	filteredStaged := filterFn(stagedFiles)
	filteredModified := filterFn(modifiedFiles)
	filteredUntracked := filterFn(untrackedFiles)

	var coloredContent strings.Builder
	regionIndex := 0
	currentLine := 0

	// Staged files
	if len(filteredStaged) > 0 {
		coloredContent.WriteString("[green]Changes to be committed:[white]\n")
		currentLine++
		tree := buildFileTree(filteredStaged)
		renderFileTree(tree, 1, &coloredContent, fileList,
			"staged", &regionIndex, currentSelection, focusedPane, lineNumberMap, &currentLine, filteredStaged, collapseState)
		coloredContent.WriteString("\n")
		currentLine++
	}

	// Modified files (unstaged)
	if len(filteredModified) > 0 {
		coloredContent.WriteString("[yellow]Changes not staged for commit:[white]\n")
		currentLine++
		tree := buildFileTree(filteredModified)
		renderFileTree(tree, 1, &coloredContent, fileList,
			"unstaged", &regionIndex, currentSelection, focusedPane, lineNumberMap, &currentLine, filteredModified, collapseState)
		coloredContent.WriteString("\n")
		currentLine++
	}

	// Untracked files
	if len(filteredUntracked) > 0 {
		coloredContent.WriteString("[red]Untracked files:[white]\n")
		currentLine++
		tree := buildFileTree(filteredUntracked)
		renderFileTree(tree, 1, &coloredContent, fileList,
			"untracked", &regionIndex, currentSelection, focusedPane, lineNumberMap, &currentLine, filteredUntracked, collapseState)
	}

	return coloredContent.String()
}

// BuildFileListContentForCommit builds file list content from commit file entries
func BuildFileListContentForCommit(
	commitFiles []FileEntry,
	currentSelection int,
	focusedPane bool,
	fileList *[]FileEntry,
	lineNumberMap map[int]int,
	collapseState *DirCollapseState,
) string {
	*fileList = (*fileList)[:0]
	for k := range lineNumberMap {
		delete(lineNumberMap, k)
	}

	tree := buildFileTreeFromFileEntries(commitFiles)

	// Build status map for O(1) lookup
	statusMap := make(map[string]string, len(commitFiles))
	for _, f := range commitFiles {
		statusMap[f.Path] = f.ChangeStatus
	}

	var content strings.Builder
	regionIndex := 0
	currentLine := 0

	renderFileTreeForFileEntries(
		tree,
		0,
		"",
		&content,
		fileList,
		"commit",
		&regionIndex,
		currentSelection,
		focusedPane,
		lineNumberMap,
		&currentLine,
		commitFiles,
		collapseState,
		statusMap,
	)

	return content.String()
}

// FileListKeyContext contains all the context needed for file list key bindings
type FileListKeyContext struct {
	// UI Components
	fileListView    *tview.TextView
	diffView        *tview.TextView
	beforeView      *tview.TextView
	afterView       *tview.TextView
	splitViewFlex   *tview.Flex
	unifiedViewFlex *tview.Flex
	contentFlex     *tview.Flex
	app             *tview.Application
	mainView        tview.Primitive // reference to the main view

	// State
	currentSelection  *int
	cursorY           *int
	isSelecting       *bool
	selectStart       *int
	selectEnd         *int
	isSplitView       *bool
	leftPaneFocused   *bool
	currentFile       *string
	currentStatus     *string
	currentDiffText   *string
	preserveScrollRow *int  // preserve file list scroll position
	ignoreWhitespace  *bool // ignore whitespace mode

	// Collections
	fileList *[]FileEntry

	// Directory collapse state
	dirCollapseState *DirCollapseState

	// Paths
	repoRoot string

	// Diff view context
	diffViewContext *DiffViewContext

	// Debounce timer for diff updates
	diffDebounceTimer *time.Timer

	// Mode
	readOnly bool // if true, disable staging/discard operations

	// File filter state
	isFilterMode *bool
	filterInput  string
	filterQuery  *string // active filter (empty = no filter)

	// Callbacks
	updateFileListView     func()
	updateSelectedFileDiff func()
	refreshFileList        func()
	updateCurrentDiffText  func(string, string, string, *string, bool)
	updateGlobalStatus     func(string, string)
	updateStatusTitle      func()
	setGlobalStatusText    func(string)
	onEsc                  func() // if non-nil, called on Esc key
	openTerminal           func() // if non-nil, opens terminal command input
}

// applyFileFilter updates the file list selection to match the filter query
func applyFileFilter(ctx *FileListKeyContext) {
	if ctx.filterInput == "" {
		// Clear filter: reset to show all and select first file
		*ctx.filterQuery = ""
		ctx.updateFileListView()
		if ctx.setGlobalStatusText != nil {
			ctx.setGlobalStatusText("[white]/[-]")
		}
		return
	}
	*ctx.filterQuery = ctx.filterInput
	// Find first matching file
	query := strings.ToLower(ctx.filterInput)
	matched := 0
	firstMatch := -1
	for i, entry := range *ctx.fileList {
		if !entry.IsDirectory && strings.Contains(strings.ToLower(entry.Path), query) {
			matched++
			if firstMatch < 0 {
				firstMatch = i
			}
		}
	}
	if firstMatch >= 0 {
		*ctx.currentSelection = firstMatch
	}
	ctx.updateFileListView()
	ctx.updateSelectedFileDiff()
	if ctx.setGlobalStatusText != nil {
		if matched > 0 {
			ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d matched][-]", tview.Escape(ctx.filterInput), matched))
		} else {
			ctx.setGlobalStatusText(fmt.Sprintf("[tomato]/%s [no match][-]", tview.Escape(ctx.filterInput)))
		}
	}
}

// SetupFileListKeyBindings sets up key bindings for file list view
func SetupFileListKeyBindings(ctx *FileListKeyContext) {
	ctx.fileListView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Filter mode input handling
		if *ctx.isFilterMode {
			switch event.Key() {
			case tcell.KeyEnter:
				// Confirm filter and apply
				*ctx.isFilterMode = false
				applyFileFilter(ctx)
			case tcell.KeyEsc:
				// Cancel filter
				*ctx.isFilterMode = false
				ctx.filterInput = ""
				*ctx.filterQuery = ""
				ctx.updateFileListView()
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(fileListKeyMessage)
				}
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(ctx.filterInput) > 0 {
					runes := []rune(ctx.filterInput)
					ctx.filterInput = string(runes[:len(runes)-1])
				}
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s[-]", tview.Escape(ctx.filterInput)))
				}
			case tcell.KeyRune:
				ctx.filterInput += string(event.Rune())
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s[-]", tview.Escape(ctx.filterInput)))
				}
			}
			return nil
		}

		switch event.Key() {
		case tcell.KeyEsc:
			// If filter is active, clear it first
			if *ctx.filterQuery != "" {
				*ctx.filterQuery = ""
				ctx.filterInput = ""
				*ctx.currentSelection = 0
				ctx.updateFileListView()
				ctx.updateSelectedFileDiff()
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(fileListKeyMessage)
				}
				return nil
			}
			if ctx.onEsc != nil {
				ctx.onEsc()
				return nil
			}
			return event
		case tcell.KeyUp:
			moveFileListSelection(ctx, -1)
			return nil
		case tcell.KeyDown:
			moveFileListSelection(ctx, 1)
			return nil
		case tcell.KeyLeft:
			handleFileListLeft(ctx)
			return nil
		case tcell.KeyRight:
			handleFileListRight(ctx)
			return nil
		case tcell.KeyEnter:
			if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
				fileEntry := (*ctx.fileList)[*ctx.currentSelection]

				// Toggle collapse for directories
				if fileEntry.IsDirectory {
					if ctx.dirCollapseState != nil {
						ctx.dirCollapseState.ToggleCollapsed(fileEntry.StageStatus, fileEntry.Path)
						ctx.updateFileListView()
						// Adjust if currentSelection is out of range
						if *ctx.currentSelection >= len(*ctx.fileList) {
							*ctx.currentSelection = len(*ctx.fileList) - 1
							ctx.updateFileListView()
						}
					}
					return nil
				}

				file := fileEntry.Path
				status := fileEntry.StageStatus

				// Check if it's the same file
				sameFile := (*ctx.currentFile == file && *ctx.currentStatus == status)

				// Update current file info
				*ctx.currentFile = file
				*ctx.currentStatus = status

				// Reset cursor and selection only for a different file
				if !sameFile {
					*ctx.cursorY = 0
					*ctx.isSelecting = false
					*ctx.selectStart = -1
					*ctx.selectEnd = -1

					ctx.updateCurrentDiffText(file, status, ctx.repoRoot, ctx.currentDiffText, *ctx.ignoreWhitespace)
				}

				// Update viewer (for cursor display)
				if *ctx.isSplitView {
					updateSplitViewWithCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.cursorY, *ctx.currentFile)
				} else {
					foldState := ctx.diffViewContext.foldState
					updateDiffViewWithCursor(ctx.diffView, *ctx.currentDiffText, *ctx.cursorY, foldState, *ctx.currentFile, ctx.repoRoot)
				}

				// Save scroll position before moving to viewer
				if ctx.preserveScrollRow != nil {
					currentRow, _ := ctx.fileListView.GetScrollOffset()
					*ctx.preserveScrollRow = currentRow
				}

				// Move focus to right pane
				*ctx.leftPaneFocused = false
				if restoreStatusFunc != nil {
					restoreStatusFunc()
				}
				ctx.updateFileListView()
				if *ctx.isSplitView {
					ctx.app.SetFocus(ctx.splitViewFlex)
				} else {
					ctx.app.SetFocus(ctx.diffView)
				}
			}
			return nil
		case tcell.KeyCtrlE:
			// Ctrl+E: scroll diff view down by one line (no cursor)
			if ctx.diffViewContext != nil {
				dctx := ctx.diffViewContext
				if *dctx.isSplitView {
					row, _ := dctx.beforeView.GetScrollOffset()
					dctx.beforeView.ScrollTo(row+1, 0)
					dctx.afterView.ScrollTo(row+1, 0)
				} else {
					row, _ := dctx.diffView.GetScrollOffset()
					dctx.diffView.ScrollTo(row+1, 0)
				}
			}
			return nil
		case tcell.KeyCtrlY:
			// Ctrl+Y: scroll diff view up by one line (no cursor)
			if ctx.diffViewContext != nil {
				dctx := ctx.diffViewContext
				if *dctx.isSplitView {
					row, _ := dctx.beforeView.GetScrollOffset()
					if row > 0 {
						dctx.beforeView.ScrollTo(row-1, 0)
						dctx.afterView.ScrollTo(row-1, 0)
					}
				} else {
					row, _ := dctx.diffView.GetScrollOffset()
					if row > 0 {
						dctx.diffView.ScrollTo(row-1, 0)
					}
				}
			}
			return nil
		case tcell.KeyCtrlL:
			if ctx.readOnly {
				return nil
			}
			// Create Git Log View
			gitLogView := NewGitLogView(ctx.app, ctx.repoRoot, func() {
				ctx.app.SetRoot(ctx.mainView, true)
				ctx.app.SetFocus(ctx.fileListView)
			})
			ctx.app.SetRoot(gitLogView.GetView(), true)
			return nil
		case tcell.KeyCtrlA:
			if ctx.readOnly {
				return nil
			}
			cmd := exec.Command("git", "-c", "core.quotepath=false", "add", "--all")
			cmd.Dir = ctx.repoRoot
			if err := cmd.Run(); err != nil {
				if ctx.updateGlobalStatus != nil {
					ctx.updateGlobalStatus("Failed to stage all files", "tomato")
				}
				return nil
			}

			ctx.refreshFileList()
			ctx.updateFileListView()
			if len(*ctx.fileList) == 0 {
				*ctx.currentSelection = 0
			} else if *ctx.currentSelection >= len(*ctx.fileList) {
				*ctx.currentSelection = len(*ctx.fileList) - 1
				ctx.updateFileListView()
			}
			ctx.updateSelectedFileDiff()
			if ctx.updateGlobalStatus != nil {
				ctx.updateGlobalStatus("Staged all files", "forestgreen")
			}
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k':
				moveFileListSelection(ctx, -1)
				return nil
			case 'j':
				moveFileListSelection(ctx, 1)
				return nil
			case 'H':
				handleFileListLeft(ctx)
				return nil
			case 'L':
				handleFileListRight(ctx)
				return nil
			case 'J': // Toggle between directory and file: on file -> next dir, on dir -> next file
				onDir := (*ctx.fileList)[*ctx.currentSelection].IsDirectory
				for i := *ctx.currentSelection + 1; i < len(*ctx.fileList); i++ {
					if (*ctx.fileList)[i].IsDirectory != onDir {
						*ctx.currentSelection = i
						ctx.updateFileListView()
						if !(*ctx.fileList)[i].IsDirectory {
							if ctx.diffDebounceTimer != nil {
								ctx.diffDebounceTimer.Stop()
							}
							ctx.diffDebounceTimer = time.AfterFunc(80*time.Millisecond, func() {
								ctx.app.QueueUpdateDraw(func() {
									ctx.updateSelectedFileDiff()
								})
							})
						}
						break
					}
				}
				return nil
			case 'K': // Move to previous directory
				for i := *ctx.currentSelection - 1; i >= 0; i-- {
					if (*ctx.fileList)[i].IsDirectory {
						*ctx.currentSelection = i
						ctx.updateFileListView()
						break
					}
				}
				return nil
			case 's':
				// Toggle split view
				*ctx.isSplitView = !*ctx.isSplitView

				if *ctx.isSplitView {
					// Show split view
					updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile)
					ctx.contentFlex.RemoveItem(ctx.unifiedViewFlex)
					ctx.contentFlex.AddItem(ctx.splitViewFlex, 0, DiffViewFlexRatio, false)
					// Update viewUpdater for split view
					if ctx.diffViewContext != nil {
						ctx.diffViewContext.viewUpdater = NewSplitViewUpdater(ctx.beforeView, ctx.afterView, ctx.currentFile)
					}
				} else {
					// Return to normal diff view
					ctx.contentFlex.RemoveItem(ctx.splitViewFlex)
					ctx.contentFlex.AddItem(ctx.unifiedViewFlex, 0, DiffViewFlexRatio, false)
					foldState := ctx.diffViewContext.foldState
					updateDiffViewWithoutCursor(ctx.diffView, *ctx.currentDiffText, foldState, *ctx.currentFile, ctx.repoRoot)
					// Update viewUpdater for unified view
					if ctx.diffViewContext != nil {
						ctx.diffViewContext.viewUpdater = NewUnifiedViewUpdater(ctx.diffView, foldState, ctx.currentFile, ctx.repoRoot)
					}
				}
				return nil
			case 'y': // copy filename only
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]
					err := commands.CopyFileName(fileEntry.Path)
					if ctx.updateGlobalStatus != nil {
						if err == nil {
							ctx.updateGlobalStatus("Copied filename to clipboard", "forestgreen")
						} else {
							ctx.updateGlobalStatus("Failed to copy filename to clipboard", "tomato")
						}
					}
				}
				return nil
			case 'Y': // copy file path
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]
					err := commands.CopyFilePath(fileEntry.Path)
					if ctx.updateGlobalStatus != nil {
						if err == nil {
							ctx.updateGlobalStatus("Copied path to clipboard", "forestgreen")
						} else {
							ctx.updateGlobalStatus("Failed to copy path to clipboard", "tomato")
						}
					}
				}
				return nil
			case '/':
				// Start file filter mode
				*ctx.isFilterMode = true
				ctx.filterInput = ""
				ctx.updateFileListView() // Redraw without cursor highlight
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText("[white]/[-]")
				}
				return nil
			case 'w':
				// Toggle ignore-whitespace mode
				*ctx.ignoreWhitespace = !*ctx.ignoreWhitespace

				// Re-fetch the diff
				if *ctx.currentFile != "" {
					ctx.updateCurrentDiffText(*ctx.currentFile, *ctx.currentStatus, ctx.repoRoot, ctx.currentDiffText, *ctx.ignoreWhitespace)
				}

				// Update the display
				if len(strings.TrimSpace(*ctx.currentDiffText)) == 0 {
					if *ctx.isSplitView {
						ctx.beforeView.SetText("")
						ctx.afterView.SetText("No differences")
					} else {
						ctx.diffView.SetText("No differences")
					}
				} else if *ctx.isSplitView {
					updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile)
				} else {
					foldState := ctx.diffViewContext.foldState
					updateDiffViewWithoutCursor(ctx.diffView, *ctx.currentDiffText, foldState, *ctx.currentFile, ctx.repoRoot)
				}

				// Update status title
				if ctx.updateStatusTitle != nil {
					ctx.updateStatusTitle()
				}

				if *ctx.ignoreWhitespace {
					ctx.updateGlobalStatus("Whitespace changes hidden", "forestgreen")
				} else {
					ctx.updateGlobalStatus("Whitespace changes shown", "forestgreen")
				}
				return nil
			case 'a': // 'a' to git add/reset the current file/directory
				if ctx.readOnly {
					return nil
				}
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]
					file := fileEntry.Path
					status := fileEntry.StageStatus

					// For directories, stage/unstage the entire directory
					if fileEntry.IsDirectory {
						file = fileEntry.Path + "/"
					}

					var cmd *exec.Cmd
					if status == "staged" {
						// Unstage the staged file
						cmd = exec.Command("git", "-c", "core.quotepath=false", "reset", "HEAD", "--", file)
						cmd.Dir = ctx.repoRoot
					} else {
						// Stage the unstaged/untracked file
						cmd = exec.Command("git", "-c", "core.quotepath=false", "add", file)
						cmd.Dir = ctx.repoRoot
					}

					// Retry to handle git index lock conflicts
					var err error
					for retry := 0; retry < 3; retry++ {
						err = cmd.Run()
						if err == nil {
							break
						}
						// Wait briefly before retrying
						time.Sleep(50 * time.Millisecond)
						// Re-create command (Cmd cannot be reused after execution)
						if status == "staged" {
							cmd = exec.Command("git", "-c", "core.quotepath=false", "reset", "HEAD", "--", file)
						} else {
							cmd = exec.Command("git", "-c", "core.quotepath=false", "add", file)
						}
						cmd.Dir = ctx.repoRoot
					}
					if err != nil {
						if ctx.updateGlobalStatus != nil {
							if status == "staged" {
								ctx.updateGlobalStatus("Failed to unstage file. Please retry.", "tomato")
							} else {
								ctx.updateGlobalStatus("Failed to stage file. Please retry.", "tomato")
							}
						}
						return nil
					}

					// Save current scroll position
					currentRow, _ := ctx.fileListView.GetScrollOffset()
					if ctx.preserveScrollRow != nil {
						*ctx.preserveScrollRow = currentRow
					}

					// Determine the next target after the current cursor position
					// File -> next file, Directory -> same directory
					var nextTarget string
					nextIsDir := fileEntry.IsDirectory
					if fileEntry.IsDirectory {
						// For directories, find the same directory
						nextTarget = fileEntry.Path
					} else {
						// For files, find the next file
						for ni := *ctx.currentSelection + 1; ni < len(*ctx.fileList); ni++ {
							if !(*ctx.fileList)[ni].IsDirectory {
								nextTarget = (*ctx.fileList)[ni].Path
								break
							}
						}
					}

					// Update file list
					ctx.refreshFileList()
					ctx.updateFileListView()

					// Find target by path (ignore status as it may change)
					foundNext := false
					if nextTarget != "" {
						for i, fe := range *ctx.fileList {
							if fe.Path == nextTarget && fe.IsDirectory == nextIsDir {
								*ctx.currentSelection = i
								foundNext = true
								break
							}
						}
					}

					if !foundNext {
						if *ctx.currentSelection >= len(*ctx.fileList) {
							*ctx.currentSelection = len(*ctx.fileList) - 1
						}
					}

					// Update display while preserving scroll position
					if ctx.preserveScrollRow != nil {
						*ctx.preserveScrollRow = currentRow
					}
					ctx.updateFileListView()
					ctx.updateSelectedFileDiff()
				}
				return nil
			case 'd': // 'd' to discard changes of the selected file (delete if untracked)
				if ctx.readOnly {
					return nil
				}
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]

					// Skip directories
					if fileEntry.IsDirectory {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Cannot discard directory. Select individual files.", "tomato")
						}
						return nil
					}

					// Show error message for staged files
					if fileEntry.StageStatus == "staged" {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Cannot discard staged changes. Use 'a' to unstage first.", "tomato")
						}
						return nil
					}

					// Set confirmation message
					var confirmMsg string
					var buttonLabel string
					if fileEntry.StageStatus == "untracked" {
						confirmMsg = "Delete " + fileEntry.Path + "?"
						buttonLabel = "Delete"
					} else {
						confirmMsg = "Discard changes in " + fileEntry.Path + "?"
						buttonLabel = "Discard"
					}

					// Create a small confirmation modal
					modal := tview.NewModal().
						SetText(confirmMsg).
						AddButtons([]string{buttonLabel, "Cancel"}).
						SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							if buttonLabel == "Discard" || buttonLabel == "Delete" {
								params := commands.CommandDParams{
									CurrentFile:   fileEntry.Path,
									CurrentStatus: fileEntry.StageStatus,
									RepoRoot:      ctx.repoRoot,
								}

								err := commands.CommandD(params)
								if err != nil {
									if ctx.updateGlobalStatus != nil {
										ctx.updateGlobalStatus(err.Error(), "tomato")
									}
								} else {
									ctx.refreshFileList()
									ctx.updateFileListView()
									ctx.updateSelectedFileDiff()
									if ctx.updateGlobalStatus != nil {
										if fileEntry.StageStatus == "untracked" {
											ctx.updateGlobalStatus("File deleted successfully!", "forestgreen")
										} else {
											ctx.updateGlobalStatus("Changes discarded successfully!", "forestgreen")
										}
									}
								}
							}
							// Return to the original view
							ctx.app.SetRoot(ctx.mainView, true)
							ctx.app.SetFocus(ctx.fileListView)
						})

					// Display mainView fullscreen with modal overlay
					pages := tview.NewPages().
						AddPage("main", ctx.mainView, true, true).
						AddPage("modal", modal, true, true)

					ctx.app.SetRoot(pages, true)
				}
				return nil
			case 'v': // 'v' to open file in vim
				if ctx.readOnly {
					return nil
				}
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]

					// Skip directories
					if fileEntry.IsDirectory {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Cannot open directory in vim. Select a file.", "tomato")
						}
						return nil
					}

					filePath := fileEntry.Path

					// Suspend application and launch $EDITOR
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "vim"
					}
					ctx.app.Suspend(func() {
						cmd := exec.Command(editor, filePath)
						cmd.Dir = ctx.repoRoot
						cmd.Stdin = os.Stdin
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						cmd.Run()
					})

					// Update file list after returning from editor
					ctx.refreshFileList()
					ctx.updateFileListView()
					ctx.updateSelectedFileDiff()
				}
				return nil
			case 'c': // 'c' to open file in VSCode
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					fileEntry := (*ctx.fileList)[*ctx.currentSelection]
					if fileEntry.IsDirectory {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Cannot open directory in VSCode. Select a file.", "tomato")
						}
						return nil
					}
					cmd := exec.Command("code", fileEntry.Path)
					cmd.Dir = ctx.repoRoot
					if err := cmd.Start(); err != nil {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Failed to open VSCode", "tomato")
						}
					}
				}
				return nil
			case 't': // 't' to open terminal command input
				if ctx.openTerminal != nil {
					ctx.openTerminal()
				}
				return nil
			case 'q': // 'q' to quit application
				go func() {
					time.Sleep(100 * time.Millisecond)
					os.Exit(0)
				}()
				ctx.app.Stop()
			}
		}
		return event
	})
}
