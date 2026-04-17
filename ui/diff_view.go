package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/ui/commands"
	"github.com/sukechannnn/giff/util"
)

// DiffViewContext contains all the context needed for diff view key bindings
type DiffViewContext struct {
	// UI Components
	diffView        *tview.TextView
	fileListView    *tview.TextView
	beforeView      *tview.TextView
	afterView       *tview.TextView
	splitViewFlex   *tview.Flex
	unifiedViewFlex *tview.Flex
	contentFlex     *tview.Flex
	app             *tview.Application

	// State
	cursorY               *int
	selectStart           *int
	selectEnd             *int
	isSelecting           *bool
	isSplitView           *bool
	leftPaneFocused       *bool
	currentDiffText       *string
	currentFile           *string
	currentStatus         *string
	savedTargetFile       *string
	preferUnstagedSection *bool
	currentSelection      *int
	fileList              *[]FileEntry
	preserveScrollRow     *int  // preserve file list scroll position
	ignoreWhitespace      *bool // ignore whitespace mode

	// Paths
	repoRoot  string
	patchPath string

	// Key handling state
	gPressed  *bool
	lastGTime *time.Time

	// View updater
	viewUpdater DiffViewUpdater

	// Fold state
	foldState *FoldState

	// Search state
	searchQuery               *string // current search query (empty = no search)
	searchMatches             *[]int  // list of matched line indices
	searchMatchIndex          *int    // current match index (position within searchMatches)
	isSearchMode              *bool   // whether in search input mode
	searchInput               *string // string being typed during search (uncommitted)
	searchCursorYBeforeSearch *int    // cursor position before search started

	// Mode
	readOnly bool // if true, disable staging/discard operations

	// Callbacks
	updateFileListView    func()
	updateGlobalStatus    func(string, string)
	setGlobalStatusText   func(string) // set status text directly (no timer)
	refreshFileList       func()
	onUpdate              func()
	updateCurrentDiffText func(string, string, string, *string, bool)
	updateStatusTitle     func()
	onEsc                 func() // if non-nil, call this instead of returning to left pane on Esc
	openTerminal          func() // if non-nil, opens terminal command input
	resizeFileList        func(delta int) // resize file list width
}

// scrollDiffView scrolls the diff view by the specified direction and handles cursor following
func scrollDiffView(ctx *DiffViewContext, direction int) {
	if *ctx.isSplitView {
		currentRow, _ := ctx.beforeView.GetScrollOffset()
		maxLines := getSplitViewLineCount(*ctx.currentDiffText)

		nextRow := currentRow + direction
		// Update scroll position (keep within range)
		if nextRow >= 0 && nextRow < maxLines {
			ctx.beforeView.ScrollTo(nextRow, 0)
			ctx.afterView.ScrollTo(nextRow, 0)

			// Follow cursor if it goes off screen
			if direction > 0 && *ctx.cursorY < nextRow {
				// Scrolling down: follow if cursor is at the top of the screen
				*ctx.cursorY = nextRow
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
				}
			} else if direction < 0 && *ctx.cursorY > nextRow+20 {
				// Scrolling up: follow if cursor is at the bottom of the screen (assuming 20 lines height)
				*ctx.cursorY = nextRow + 20
				if *ctx.cursorY >= maxLines {
					*ctx.cursorY = maxLines - 1
				}
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
				}
			}
		}
	} else {
		// Unified view case
		currentRow, _ := ctx.diffView.GetScrollOffset()
		maxLines := GetUnifiedViewLineCount(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)

		nextRow := currentRow + direction
		// Update scroll position (keep within range)
		if nextRow >= 0 && nextRow < maxLines {
			ctx.diffView.ScrollTo(nextRow, 0)

			// Follow cursor if it goes off screen
			if direction > 0 && *ctx.cursorY < nextRow {
				// Scrolling down: follow if cursor is at the top of the screen
				*ctx.cursorY = nextRow
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
				}
			} else if direction < 0 && *ctx.cursorY > nextRow+20 {
				// Scrolling up: follow if cursor is at the bottom of the screen (assuming 20 lines height)
				*ctx.cursorY = nextRow + 20
				if *ctx.cursorY >= maxLines {
					*ctx.cursorY = maxLines - 1
				}
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
				}
			}
		}
	}
}

// SetupDiffViewKeyBindings sets up key bindings for diff view
func SetupDiffViewKeyBindings(ctx *DiffViewContext) {
	// Set viewUpdater in initial state
	if ctx.viewUpdater == nil {
		if *ctx.isSplitView {
			ctx.viewUpdater = NewSplitViewUpdater(ctx.beforeView, ctx.afterView, ctx.currentFile)
		} else {
			ctx.viewUpdater = &UnifiedViewUpdater{
				diffView:    ctx.diffView,
				foldState:   ctx.foldState,
				filePath:    ctx.currentFile,
				repoRoot:    ctx.repoRoot,
				searchQuery: ctx.searchQuery,
			}
		}
	}

	// Common key handler function
	keyHandler := func(event *tcell.EventKey) *tcell.EventKey {
		// Key handling during search mode
		if *ctx.isSearchMode {
			switch event.Key() {
			case tcell.KeyEnter:
				// Confirm search
				*ctx.searchQuery = *ctx.searchInput
				*ctx.isSearchMode = false
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}
				if len(*ctx.searchMatches) > 0 {
					ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d/%d] matched[-]", tview.Escape(*ctx.searchQuery), *ctx.searchMatchIndex+1, len(*ctx.searchMatches)))
				} else {
					ctx.setGlobalStatusText(fmt.Sprintf("[tomato]/%s [no match][-]", tview.Escape(*ctx.searchQuery)))
				}
			case tcell.KeyEsc:
				// Cancel search: restore cursor to original position
				*ctx.isSearchMode = false
				*ctx.searchInput = ""
				*ctx.searchQuery = ""
				*ctx.searchMatches = nil
				*ctx.searchMatchIndex = -1
				*ctx.cursorY = *ctx.searchCursorYBeforeSearch
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}
				ctx.setGlobalStatusText(diffViewKeyMessage)
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(*ctx.searchInput) > 0 {
					// Delete one character
					runes := []rune(*ctx.searchInput)
					*ctx.searchInput = string(runes[:len(runes)-1])
				}
				performSearch(ctx)
			case tcell.KeyRune:
				*ctx.searchInput += string(event.Rune())
				performSearch(ctx)
			}
			return nil
		}

		switch event.Key() {
		case tcell.KeyEsc:
			// If search results exist, first Esc clears them
			if *ctx.searchQuery != "" {
				*ctx.searchQuery = ""
				*ctx.searchMatches = nil
				*ctx.searchMatchIndex = -1
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}
				return nil
			}
			// If in selection mode, deselect
			if *ctx.isSelecting {
				*ctx.isSelecting = false
				*ctx.selectEnd = -1
				*ctx.selectStart = -1
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}
				return nil
			}
			// Return to left pane
			*ctx.leftPaneFocused = true
			if restoreStatusFunc != nil {
				restoreStatusFunc()
			}
			if *ctx.isSplitView {
				updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile)
			} else {
				updateDiffViewWithoutCursor(ctx.diffView, *ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
			}
			ctx.updateFileListView()
			ctx.app.SetFocus(ctx.fileListView)
			return nil
		case tcell.KeyEnter:
			// Return to left pane
			*ctx.isSelecting = false
			*ctx.selectStart = -1
			*ctx.selectEnd = -1
			*ctx.leftPaneFocused = true
			if restoreStatusFunc != nil {
				restoreStatusFunc()
			}
			// Redraw diff view without cursor
			if *ctx.isSplitView {
				updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile)
			} else {
				updateDiffViewWithoutCursor(ctx.diffView, *ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
			}
			ctx.updateFileListView()
			ctx.app.SetFocus(ctx.fileListView)
			return nil
		case tcell.KeyCtrlE:
			// Ctrl+E: scroll down one line (cursor follows when at top)
			scrollDiffView(ctx, 1)
			return nil
		case tcell.KeyCtrlY:
			// Ctrl+Y: scroll up one line
			scrollDiffView(ctx, -1)
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case 's':
				// Toggle split view
				*ctx.isSplitView = !*ctx.isSplitView

				if *ctx.isSplitView {
					// Show split view (maintain current cursor position)
					ctx.viewUpdater = NewSplitViewUpdater(ctx.beforeView, ctx.afterView, ctx.currentFile)
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
					ctx.contentFlex.RemoveItem(ctx.unifiedViewFlex)
					ctx.contentFlex.AddItem(ctx.splitViewFlex, 0, DiffViewFlexRatio, false)
					// Move focus from diffView to splitViewFlex
					if !*ctx.leftPaneFocused {
						ctx.app.SetFocus(ctx.splitViewFlex)
					}
				} else {
					// Return to normal diff view
					ctx.viewUpdater = &UnifiedViewUpdater{
						diffView:    ctx.diffView,
						foldState:   ctx.foldState,
						filePath:    ctx.currentFile,
						repoRoot:    ctx.repoRoot,
						searchQuery: ctx.searchQuery,
					}
					ctx.contentFlex.RemoveItem(ctx.splitViewFlex)
					ctx.contentFlex.AddItem(ctx.unifiedViewFlex, 0, DiffViewFlexRatio, false)
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
					// Move focus from splitViewFlex to diffView
					if !*ctx.leftPaneFocused {
						ctx.app.SetFocus(ctx.diffView)
					}
				}
				return nil
			case 'w':
				// Toggle ignore-whitespace mode
				*ctx.ignoreWhitespace = !*ctx.ignoreWhitespace

				// Re-fetch the diff
				ctx.updateCurrentDiffText(*ctx.currentFile, *ctx.currentStatus, ctx.repoRoot, ctx.currentDiffText, *ctx.ignoreWhitespace)

				// Reset cursor position
				*ctx.cursorY = 0

				// Show message when diff is empty
				if len(strings.TrimSpace(*ctx.currentDiffText)) == 0 {
					if *ctx.isSplitView {
						ctx.beforeView.SetText("")
						ctx.afterView.SetText("No differences")
					} else {
						ctx.diffView.SetText("No differences")
					}
				} else if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}

				// Update status title
				if ctx.updateStatusTitle != nil {
					ctx.updateStatusTitle()
				}

				// Display status
				if *ctx.ignoreWhitespace {
					ctx.updateGlobalStatus("Whitespace changes hidden", "forestgreen")
				} else {
					ctx.updateGlobalStatus("Whitespace changes shown", "forestgreen")
				}
				return nil
			case 'g':
				now := time.Now()
				if *ctx.gPressed && now.Sub(*ctx.lastGTime) < 500*time.Millisecond {
					// gg -> go to top
					*ctx.cursorY = 0
					if *ctx.isSelecting {
						*ctx.selectEnd = *ctx.cursorY
					}
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
					}
					*ctx.gPressed = false
				} else {
					// First g press
					*ctx.gPressed = true
					*ctx.lastGTime = now
				}
				return nil
			case 'G': // Shift+G -> go to bottom
				maxLines := 0
				if *ctx.isSplitView {
					// For split view, get valid line count
					splitViewLines := getSplitViewLineCount(*ctx.currentDiffText)
					if splitViewLines > 0 {
						maxLines = splitViewLines - 1
					}
				} else {
					// For normal view, get display line count (including folds)
					lineCount := GetUnifiedViewLineCount(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
					if lineCount > 0 {
						maxLines = lineCount - 1
					}
				}
				*ctx.cursorY = maxLines
				if *ctx.isSelecting {
					*ctx.selectEnd = *ctx.cursorY
				}
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}
				return nil
			case 'j':
				// Move down
				maxLines := 0
				if *ctx.isSplitView {
					// For split view, get valid line count
					splitViewLines := getSplitViewLineCount(*ctx.currentDiffText)
					if splitViewLines > 0 {
						maxLines = splitViewLines - 1
					}
				} else {
					// For normal view, get display line count (including folds)
					if len(strings.TrimSpace(*ctx.currentDiffText)) > 0 {
						lineCount := GetUnifiedViewLineCount(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
						if lineCount > 0 {
							maxLines = lineCount - 1
						}
					}
				}

				if *ctx.cursorY < maxLines {
					(*ctx.cursorY)++
					if *ctx.isSelecting {
						*ctx.selectEnd = *ctx.cursorY
					}
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
					}
				}
				return nil
			case 'k':
				// Move up
				if *ctx.cursorY > 0 {
					(*ctx.cursorY)--
					if *ctx.isSelecting {
						*ctx.selectEnd = *ctx.cursorY
					}
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
					}
				}
				return nil
			case 'h':
				// Scroll left
				if *ctx.isSplitView {
					_, col := ctx.beforeView.GetScrollOffset()
					if col > 0 {
						row, _ := ctx.beforeView.GetScrollOffset()
						ctx.beforeView.ScrollTo(row, col-4)
						row2, _ := ctx.afterView.GetScrollOffset()
						ctx.afterView.ScrollTo(row2, col-4)
					}
				} else {
					row, col := ctx.diffView.GetScrollOffset()
					if col > 0 {
						ctx.diffView.ScrollTo(row, col-4)
					}
				}
				return nil
			case 'l':
				// Scroll right
				if *ctx.isSplitView {
					row, col := ctx.beforeView.GetScrollOffset()
					ctx.beforeView.ScrollTo(row, col+4)
					row2, _ := ctx.afterView.GetScrollOffset()
					ctx.afterView.ScrollTo(row2, col+4)
				} else {
					row, col := ctx.diffView.GetScrollOffset()
					ctx.diffView.ScrollTo(row, col+4)
				}
				return nil
			case 'V':
				// Shift+V to start selection mode
				if !*ctx.isSelecting {
					*ctx.isSelecting = true
					*ctx.selectStart = *ctx.cursorY
					*ctx.selectEnd = *ctx.cursorY
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
					}
				}
				return nil
			case 'y':
				start := *ctx.cursorY
				end := *ctx.cursorY
				if *ctx.isSelecting && *ctx.selectStart >= 0 && *ctx.selectEnd >= 0 {
					start = *ctx.selectStart
					end = *ctx.selectEnd
				}
				if start > end {
					start, end = end, start
				}

				// Get content directly from unified view (includes fold expanded lines)
				var sanitized []string
				if !*ctx.isSplitView {
					content := getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
					if content != nil && len(content.Lines) > 0 {
						if start < 0 {
							start = 0
						}
						if end >= len(content.Lines) {
							end = len(content.Lines) - 1
						}
						for i := start; i <= end; i++ {
							line := content.Lines[i]
							// Strip tview tags to get plain text
							plain := stripTviewTags(line.Content)
							sanitized = append(sanitized, stripDiffPrefix(plain))
						}
					}
				} else {
					lines := getSelectableDiffLines(*ctx.currentDiffText)
					if start < 0 {
						start = 0
					}
					if end >= len(lines) {
						end = len(lines) - 1
					}
					if start < len(lines) {
						for _, line := range lines[start : end+1] {
							sanitized = append(sanitized, stripDiffPrefix(line))
						}
					}
				}

				if len(sanitized) == 0 {
					ctx.updateGlobalStatus("No content to copy", "tomato")
					return nil
				}
				text := strings.Join(sanitized, "\n")
				if err := commands.CopyToClipboard(text); err != nil {
					ctx.updateGlobalStatus("Failed to copy diff lines", "tomato")
				} else {
					message := "Copied line to clipboard"
					if len(sanitized) > 1 {
						message = "Copied lines to clipboard"
					}
					ctx.updateGlobalStatus(message, "forestgreen")
				}

				if *ctx.isSelecting {
					*ctx.isSelecting = false
					*ctx.selectStart = -1
					*ctx.selectEnd = -1
				}

				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}

				return nil
			case 'Y':
				// Shift+Y: copy file path
				if *ctx.currentFile != "" {
					err := commands.CopyFilePath(*ctx.currentFile)
					if err == nil {
						ctx.updateGlobalStatus("Copied path to clipboard", "forestgreen")
					} else {
						ctx.updateGlobalStatus("Failed to copy path to clipboard", "tomato")
					}
				}
				return nil
			case 'L':
				// Shift+L: copy file/path:XX-YY if selecting, or file path if not selecting
				if *ctx.isSelecting && *ctx.currentFile != "" {
					start := *ctx.selectStart
					end := *ctx.selectEnd
					if start > end {
						start, end = end, start
					}

					var startLine, endLine int
					if _, ok := ctx.viewUpdater.(*FileViewUpdater); ok {
						// File viewer mode: line number = index + 1
						startLine = start + 1
						endLine = end + 1
					} else {
						// Diff mode: use line number mapping
						if !*ctx.isSplitView {
							displayMapping := MapUnifiedDisplayToOriginalIdx(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
							if ms, ok := displayMapping[start]; ok {
								start = ms
							}
							if me, ok := displayMapping[end]; ok {
								end = me
							}
						}

						oldLineMap, newLineMap := createLineNumberMapping(*ctx.currentDiffText)
						startLine = -1
						endLine = -1
						for i := start; i <= end; i++ {
							num := -1
							if n, ok := newLineMap[i]; ok {
								num = n
							} else if n, ok := oldLineMap[i]; ok {
								num = n
							}
							if num >= 0 {
								if startLine == -1 || num < startLine {
									startLine = num
								}
								if num > endLine {
									endLine = num
								}
							}
						}
					}

					if startLine >= 0 {
						var ref string
						if startLine == endLine {
							ref = fmt.Sprintf("%s:%d", *ctx.currentFile, startLine)
						} else {
							ref = fmt.Sprintf("%s:%d-%d", *ctx.currentFile, startLine, endLine)
						}
						if err := commands.CopyToClipboard(ref); err == nil {
							ctx.updateGlobalStatus("Copied reference to clipboard", "forestgreen")
						} else {
							ctx.updateGlobalStatus("Failed to copy reference", "tomato")
						}
					}

					// Deselect
					*ctx.isSelecting = false
					*ctx.selectStart = -1
					*ctx.selectEnd = -1
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
					}
				} else if *ctx.currentFile != "" {
					err := commands.CopyFilePath(*ctx.currentFile)
					if err == nil {
						ctx.updateGlobalStatus("Copied path to clipboard", "forestgreen")
					} else {
						ctx.updateGlobalStatus("Failed to copy path to clipboard", "tomato")
					}
				}
				return nil
			case '/':
				// Start search mode
				*ctx.isSearchMode = true
				*ctx.searchInput = ""
				*ctx.searchCursorYBeforeSearch = *ctx.cursorY
				ctx.setGlobalStatusText("[white]/[-]")
				return nil
			case 'n':
				// Move to next match
				if *ctx.searchQuery != "" && len(*ctx.searchMatches) > 0 {
					moveToNextMatch(ctx)
				}
				return nil
			case 'N':
				// Move to previous match
				if *ctx.searchQuery != "" && len(*ctx.searchMatches) > 0 {
					moveToPrevMatch(ctx)
				}
				return nil
			case 'u':
				if ctx.readOnly {
					return nil
				}
				ctx.updateGlobalStatus("undo is not implemented!", "tomato")
			case 'v':
				if ctx.readOnly {
					return nil
				}
				// Open file in $EDITOR at current line
				if *ctx.currentFile != "" {
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "vim"
					}
					lineNum := getCursorFileLineNumber(ctx)
					editorBase := filepath.Base(editor)
					ctx.app.Suspend(func() {
						var cmd *exec.Cmd
						if lineNum > 0 && (editorBase == "vim" || editorBase == "nvim") {
							cmd = exec.Command(editor, fmt.Sprintf("+%d", lineNum), *ctx.currentFile)
						} else {
							cmd = exec.Command(editor, *ctx.currentFile)
						}
						cmd.Dir = ctx.repoRoot
						cmd.Stdin = os.Stdin
						cmd.Stdout = os.Stdout
						cmd.Stderr = os.Stderr
						cmd.Run()
					})
					// Clear selection
					*ctx.isSelecting = false
					*ctx.selectStart = -1
					*ctx.selectEnd = -1
					ctx.refreshFileList()
					ctx.updateFileListView()
					ctx.updateCurrentDiffText(*ctx.currentFile, *ctx.currentStatus, ctx.repoRoot, ctx.currentDiffText, *ctx.ignoreWhitespace)
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
					}
				}
				return nil
			case 'c':
				// Open file in VSCode at current line
				if *ctx.currentFile != "" {
					lineNum := getCursorFileLineNumber(ctx)
					var arg string
					if lineNum > 0 {
						arg = fmt.Sprintf("%s:%d", *ctx.currentFile, lineNum)
					} else {
						arg = *ctx.currentFile
					}
					cmd := exec.Command("code", "-g", arg)
					cmd.Dir = ctx.repoRoot
					if err := cmd.Start(); err != nil {
						ctx.updateGlobalStatus("Failed to open VSCode", "tomato")
					}
					// Clear selection
					*ctx.isSelecting = false
					*ctx.selectStart = -1
					*ctx.selectEnd = -1
					if ctx.viewUpdater != nil {
						ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
					}
				}
				return nil
			case 'd':
				// In read-only mode (git log), 'd' exits back
				if ctx.readOnly && ctx.onEsc != nil {
					ctx.onEsc()
					return nil
				}
			case 't':
				// Open terminal command input
				if ctx.openTerminal != nil {
					ctx.openTerminal()
				}
				return nil
			case 'e':
				// Expand/collapse fold at cursor
				if !*ctx.isSplitView && ctx.foldState != nil {
					foldID := GetFoldIDAtLine(*ctx.currentDiffText, *ctx.cursorY, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
					if foldID != "" {
						wasExpanded := ctx.foldState.IsExpanded(foldID)
						ctx.foldState.ToggleExpand(foldID)
						InvalidateUnifiedContentCache()

						// If collapsing, move cursor to the fold indicator position
						if wasExpanded {
							newPos := GetFoldIndicatorPosition(*ctx.currentDiffText, foldID, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
							*ctx.cursorY = newPos
						}

						if ctx.viewUpdater != nil {
							ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
						}
					}
				}
				return nil
			case 'a':
				if ctx.readOnly {
					return nil
				}
				// Call commandA function
				// For unified view, convert to actual diff line indices excluding fold indicators
				selectStart := *ctx.selectStart
				selectEnd := *ctx.selectEnd
				if !*ctx.isSplitView {
					// Get mapping excluding fold indicators
					displayMapping := MapUnifiedDisplayToOriginalIdx(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
					// Convert selection range to indices excluding fold indicators
					if mappedStart, ok := displayMapping[selectStart]; ok {
						selectStart = mappedStart
					}
					if mappedEnd, ok := displayMapping[selectEnd]; ok {
						selectEnd = mappedEnd
					}
				}

				params := commands.CommandAParams{
					SelectStart:        selectStart,
					SelectEnd:          selectEnd,
					CurrentFile:        *ctx.currentFile,
					CurrentStatus:      *ctx.currentStatus,
					CurrentDiffText:    *ctx.currentDiffText,
					RepoRoot:           ctx.repoRoot,
					UpdateGlobalStatus: ctx.updateGlobalStatus,
					IsSplitView:        *ctx.isSplitView,
				}

				result, err := commands.CommandA(params)
				if err != nil {
					return nil
				}
				if result == nil {
					return nil
				}

				// Apply results
				*ctx.currentDiffText = result.NewDiffText

				// Deselect and update cursor position
				*ctx.isSelecting = false
				*ctx.selectStart = -1
				*ctx.selectEnd = -1

				// Cursor position boundary check
				newCursorPos := result.NewCursorPos
				if len(strings.TrimSpace(*ctx.currentDiffText)) > 0 {
					coloredDiff := ColorizeDiff(*ctx.currentDiffText)
					diffLines := util.SplitLines(coloredDiff)
					maxLines := len(diffLines) - 1
					if maxLines < 0 {
						maxLines = 0
					}
					if newCursorPos > maxLines {
						newCursorPos = maxLines
					}
					if newCursorPos < 0 {
						newCursorPos = 0
					}
				} else {
					newCursorPos = 0
				}
				*ctx.cursorY = newCursorPos

				// Redraw
				if ctx.viewUpdater != nil {
					ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
				}

				// Internally update file list
				ctx.refreshFileList()

				// If diff remains
				if !result.ShouldUpdate {
					// Save currently selected file and status
					var currentlySelectedFile string
					var currentlySelectedStatus string
					if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
						fileEntry := (*ctx.fileList)[*ctx.currentSelection]
						currentlySelectedFile = fileEntry.Path
						currentlySelectedStatus = fileEntry.StageStatus
					}

					// Redraw file list
					ctx.updateFileListView()

					// Restore selection position (search by both filename and status)
					newSelection := -1
					for i, fileEntry := range *ctx.fileList {
						if fileEntry.Path == currentlySelectedFile && fileEntry.StageStatus == currentlySelectedStatus {
							newSelection = i
							break
						}
					}
					if newSelection >= 0 {
						*ctx.currentSelection = newSelection
					} else if *ctx.currentSelection >= len(*ctx.fileList) {
						*ctx.currentSelection = len(*ctx.fileList) - 1
					}

					// Update file list again if selection position changed
					ctx.updateFileListView()
				} else {
					// If diff is gone, fully update
					if ctx.onUpdate != nil {
						ctx.onUpdate()
					}
				}
				return nil
			case 'A':
				if ctx.readOnly {
					return nil
				}
				// Stage/unstage the current file
				if *ctx.currentFile != "" {
					var cmd *exec.Cmd
					if *ctx.currentStatus == "staged" {
						cmd = exec.Command("git", "-c", "core.quotepath=false", "reset", "HEAD", *ctx.currentFile)
					} else {
						cmd = exec.Command("git", "-c", "core.quotepath=false", "add", *ctx.currentFile)
					}
					cmd.Dir = ctx.repoRoot

					err := cmd.Run()
					if err == nil {
						wasStaged := (*ctx.currentStatus == "staged")

						if *ctx.currentStatus == "staged" {
							// Show diff of now-unstaged file
							*ctx.currentStatus = "unstaged"
							newDiffText, _ := git.GetFileDiff(*ctx.currentFile, ctx.repoRoot)
							*ctx.currentDiffText = newDiffText
						} else {
							// Show diff of now-staged file
							*ctx.currentStatus = "staged"
							newDiffText, _ := git.GetStagedDiff(*ctx.currentFile, ctx.repoRoot)
							*ctx.currentDiffText = newDiffText
						}

						// Reset cursor and selection
						*ctx.isSelecting = false
						*ctx.selectStart = -1
						*ctx.selectEnd = -1
						*ctx.cursorY = 0

						// Redraw
						if ctx.viewUpdater != nil {
							ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
						}

						// Update status
						if wasStaged {
							ctx.updateGlobalStatus("File unstaged successfully!", "forestgreen")
						} else {
							ctx.updateGlobalStatus("File staged successfully!", "forestgreen")
						}

						// Call refreshFileList to get the latest state
						ctx.refreshFileList()

						// Save cursor position
						// Set to always select the first item in the unstaged section
						*ctx.preferUnstagedSection = true
						*ctx.savedTargetFile = ""

						// Update file list
						if ctx.onUpdate != nil {
							ctx.onUpdate()
						}
					} else {
						// Error case
						if *ctx.currentStatus == "staged" {
							ctx.updateGlobalStatus("Failed to unstage file", "tomato")
						} else {
							ctx.updateGlobalStatus("Failed to stage file", "tomato")
						}
					}
				}
				return nil
			case '+', '=': // '+' to widen file list
				if ctx.resizeFileList != nil {
					ctx.resizeFileList(1)
				}
				return nil
			case '-': // '-' to narrow file list
				if ctx.resizeFileList != nil {
					ctx.resizeFileList(-1)
				}
				return nil
			case 'q':
				// Quit application
				go func() {
					time.Sleep(100 * time.Millisecond)
					os.Exit(0)
				}()
				ctx.app.Stop()
				return nil
			}
		}
		return event
	}

	// Set the same key handler for both DiffView and SplitViewFlex
	ctx.diffView.SetInputCapture(keyHandler)
	ctx.splitViewFlex.SetInputCapture(keyHandler)
}

func getSelectableDiffLines(diffText string) []string {
	rawLines := util.SplitLines(diffText)
	visible := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if isUnifiedHeaderLine(line) {
			continue
		}
		visible = append(visible, line)
	}
	return visible
}

func stripDiffPrefix(line string) string {
	if len(line) == 0 {
		return line
	}
	switch line[0] {
	case '+', '-':
		return line[1:]
	default:
		return line
	}
}

// Regex to strip tview color tags
var tviewTagRegex = regexp.MustCompile(`\[("[^"]*"|[^\[\]]*)\]`)

// stripTviewTags removes tview color/region tags from text
func stripTviewTags(text string) string {
	return tviewTagRegex.ReplaceAllString(text, "")
}

// highlightSearchInTaggedText highlights occurrences of query in a tview-tagged string
func highlightSearchInTaggedText(tagged string, query string) string {
	if query == "" {
		return tagged
	}

	plain := stripTviewTags(tagged)
	plainRunes := []rune(plain)
	queryRunes := []rune(query)
	queryLen := len(queryRunes)

	if len(plainRunes) < queryLen {
		return tagged
	}

	// Mark matching visible character positions
	highlight := make([]bool, len(plainRunes))
	for i := 0; i <= len(plainRunes)-queryLen; i++ {
		match := true
		for j := 0; j < queryLen; j++ {
			if plainRunes[i+j] != queryRunes[j] {
				match = false
				break
			}
		}
		if match {
			for j := 0; j < queryLen; j++ {
				highlight[i+j] = true
			}
		}
	}

	// Return as-is if no matches
	anyMatch := false
	for _, h := range highlight {
		if h {
			anyMatch = true
			break
		}
	}
	if !anyMatch {
		return tagged
	}

	hlStart := "[:" + util.SearchHighlightBg + "]"
	const hlEnd = "[:-]"

	var result strings.Builder
	runes := []rune(tagged)
	visibleIdx := 0
	inHL := false

	for i := 0; i < len(runes); {
		// Detect tview tags
		if runes[i] == '[' {
			j := i + 1
			inQuote := false
			for j < len(runes) {
				if runes[j] == '"' {
					inQuote = !inQuote
				} else if !inQuote && runes[j] == ']' {
					break
				} else if !inQuote && runes[j] == '[' {
					break
				}
				j++
			}
			if j < len(runes) && runes[j] == ']' {
				// Valid tag: if highlighting, re-apply around the tag
				tagStr := string(runes[i : j+1])
				if inHL {
					result.WriteString(hlEnd)
					result.WriteString(tagStr)
					result.WriteString(hlStart)
				} else {
					result.WriteString(tagStr)
				}
				i = j + 1
				continue
			}
		}

		// Visible character
		shouldHL := visibleIdx < len(highlight) && highlight[visibleIdx]

		if shouldHL && !inHL {
			result.WriteString(hlStart)
			inHL = true
		} else if !shouldHL && inHL {
			result.WriteString(hlEnd)
			inHL = false
		}

		result.WriteRune(runes[i])
		visibleIdx++
		i++
	}

	if inHL {
		result.WriteString(hlEnd)
	}

	return result.String()
}

// searchInUnifiedContent searches for query in unified view content and returns matching line indices
func searchInUnifiedContent(content *UnifiedViewContent, query string) []int {
	var matches []int
	for i, line := range content.Lines {
		plain := stripTviewTags(line.LineNumber + line.Content)
		if strings.Contains(plain, query) {
			matches = append(matches, i)
		}
	}
	return matches
}

// getCursorFileLineNumber returns the file line number at the current cursor position.
// Returns 0 if the line number cannot be determined.
func getCursorFileLineNumber(ctx *DiffViewContext) int {
	if ctx.currentDiffText == nil || *ctx.currentDiffText == "" {
		return 0
	}

	// File viewer mode: line number is simply cursorY + 1
	if _, ok := ctx.viewUpdater.(*FileViewUpdater); ok {
		return *ctx.cursorY + 1
	}

	cursorIdx := *ctx.cursorY

	// For unified view, map display index to original diff index (excluding folds)
	if !*ctx.isSplitView && ctx.foldState != nil {
		displayMapping := MapUnifiedDisplayToOriginalIdx(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
		if mapped, ok := displayMapping[cursorIdx]; ok {
			cursorIdx = mapped
		}
	}

	_, newLineMap := createLineNumberMapping(*ctx.currentDiffText)
	if num, ok := newLineMap[cursorIdx]; ok {
		return num
	}
	oldLineMap, _ := createLineNumberMapping(*ctx.currentDiffText)
	if num, ok := oldLineMap[cursorIdx]; ok {
		return num
	}
	return 0
}

// performSearch executes search with current searchInput and updates matches/cursor
func performSearch(ctx *DiffViewContext) {
	query := *ctx.searchInput
	if query == "" {
		*ctx.searchQuery = ""
		*ctx.searchMatches = nil
		*ctx.searchMatchIndex = -1
		*ctx.cursorY = *ctx.searchCursorYBeforeSearch
		if ctx.viewUpdater != nil {
			ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
		}
		ctx.setGlobalStatusText("[white]/[-]")
		return
	}

	// Also update searchQuery to apply real-time search highlighting
	*ctx.searchQuery = query

	content := getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
	matches := searchInUnifiedContent(content, query)
	*ctx.searchMatches = matches

	if len(matches) > 0 {
		// Find the nearest forward match from the current cursor position
		matchIdx := 0
		for i, m := range matches {
			if m >= *ctx.searchCursorYBeforeSearch {
				matchIdx = i
				break
			}
		}
		*ctx.searchMatchIndex = matchIdx
		*ctx.cursorY = matches[matchIdx]
		if ctx.viewUpdater != nil {
			ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
		}
		ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d/%d][-]", tview.Escape(query), matchIdx+1, len(matches)))
	} else {
		*ctx.searchMatchIndex = -1
		*ctx.cursorY = *ctx.searchCursorYBeforeSearch
		if ctx.viewUpdater != nil {
			ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
		}
		ctx.setGlobalStatusText(fmt.Sprintf("[tomato]/%s [no match][-]", tview.Escape(query)))
	}
}

// moveToNextMatch moves cursor to the next search match
func moveToNextMatch(ctx *DiffViewContext) {
	matches := *ctx.searchMatches
	if len(matches) == 0 {
		return
	}
	*ctx.searchMatchIndex = (*ctx.searchMatchIndex + 1) % len(matches)
	*ctx.cursorY = matches[*ctx.searchMatchIndex]
	if ctx.viewUpdater != nil {
		ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
	}
	ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d/%d][-]", tview.Escape(*ctx.searchQuery), *ctx.searchMatchIndex+1, len(matches)))
}

// moveToPrevMatch moves cursor to the previous search match
func moveToPrevMatch(ctx *DiffViewContext) {
	matches := *ctx.searchMatches
	if len(matches) == 0 {
		return
	}
	*ctx.searchMatchIndex = (*ctx.searchMatchIndex - 1 + len(matches)) % len(matches)
	*ctx.cursorY = matches[*ctx.searchMatchIndex]
	if ctx.viewUpdater != nil {
		ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
	}
	ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d/%d][-]", tview.Escape(*ctx.searchQuery), *ctx.searchMatchIndex+1, len(matches)))
}
