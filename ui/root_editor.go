package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// Saved cursor information
var savedTargetFile string = ""
var preferUnstagedSection bool = false

// globalStatusView defined globally
var globalStatusView *tview.TextView
var fileListKeyMessage = "a:stage  A:stage file  d:discard  C-a:stage all  u:undo  C-r:redo  C-k:commit  C-j:amend  Tab:file  gg/G:top/end  H/L:dir  s:split  w:ws  /:grep  !:hide  C-f:filter  v:editor  c:code  C-l:log  pr:PR diff  t:terminal  Y:copy  C-e/C-y:scroll  Enter:switch  q:quit"
var diffViewKeyMessage = "a:stage lines  d:discard lines  A:stage file  u:undo  C-r:redo  V:select  g/G:top/end  /:search  e:fold  s:split  w:ws  y:yank  Y:copy path  pr:PR diff  C-e/C-y:scroll  Esc:back  q:quit"

// restoreStatusFunc is called to restore the default status message (set by SetupRootEditor)
var restoreStatusFunc func()

// restoreStatus puts the default status message back, using override when a
// view has a status bar of its own
func restoreStatus(override func()) {
	if override != nil {
		override()
	} else if restoreStatusFunc != nil {
		restoreStatusFunc()
	}
}

func updateGlobalStatus(message string, color string) {
	if globalStatusView != nil {
		globalStatusView.SetText(fmt.Sprintf("[%s]%s[-]", color, message))
		go func() {
			time.Sleep(5 * time.Second)
			if restoreStatusFunc != nil {
				restoreStatusFunc()
			}
		}()
	}
}

// Get diff for a file
func updateCurrentDiffText(filePath string, status string, repoRoot string, currentDiffText *string, ignoreWhitespace bool) {
	var diffText string
	var err error

	// Check if it's a directory
	fullPath := filepath.Join(repoRoot, filePath)
	fileInfo, statErr := os.Stat(fullPath)
	if statErr == nil && fileInfo.IsDir() {
		return
	}

	switch status {
	case "staged":
		diffText, err = git.GetStagedDiffWithOptions(filePath, repoRoot, ignoreWhitespace)
	case "untracked":
		isBinary, binErr := util.IsBinaryFile(filePath, repoRoot)
		if binErr != nil {
			err = binErr
		} else if isBinary {
			diffText = util.FormatBinaryNotice(filePath)
		} else {
			content, readErr := util.ReadFileContent(filePath, repoRoot)
			if readErr != nil {
				err = readErr
			} else {
				diffText = util.FormatAsAddedLines(content, filePath)
			}
		}
	default:
		diffText, err = git.GetFileDiffWithOptions(filePath, repoRoot, ignoreWhitespace)
	}

	// Show only a notice for binary files instead of git's raw binary diff output
	if err == nil && util.IsBinaryDiff(diffText) {
		diffText = util.FormatBinaryNotice(filePath)
	}

	if err != nil {
		diffText = fmt.Sprintf("Error getting diff for %s: %v\n\nThis might be a deleted file or there might be an issue with git.", filePath, err)
	}

	if currentDiffText != nil {
		*currentDiffText = diffText
	}
}

func RootEditor(app *tview.Application, stagedFiles, modifiedFiles, untrackedFiles []git.FileInfo, repoRoot string, patchFilePath string, onUpdate func(), enableAutoRefresh bool, initialFile string, externalFile string) tview.Primitive {
	// Keep references for updating file lists
	stagedFilesPtr := &stagedFiles
	modifiedFilesPtr := &modifiedFiles
	untrackedFilesPtr := &untrackedFiles

	// Commit-related state
	var isCommitMode bool = false
	var isAmendMode bool = false
	var commitMessage string = ""
	var focusBeforeCommit tview.Primitive = nil // focus position before commit mode

	// File browser mode state
	var isFileBrowserMode bool = false
	var browserFiles []git.FileInfo
	var browserCollapseState = NewDirCollapseState()
	var browserFilterQuery string
	var browserFilterMode bool
	// browserRoot is the directory used to resolve browser-mode file paths.
	// Defaults to repoRoot but is replaced with an external file's parent
	// directory when the user passes a path outside the repo on startup.
	browserRoot := repoRoot
	// Keep current file info
	var currentFile string
	var currentStatus string
	var currentDiffText string
	var cursorY int = 0
	var selectStart int = -1
	var selectEnd int = -1
	var isSelecting bool = false
	var currentSelection int = 0
	var leftPaneFocused bool = true
	var gPressed bool = false
	var lastGTime time.Time
	var isSplitView bool = false      // split view mode flag
	var ignoreWhitespace bool = false // ignore whitespace mode flag

	// File list filter state
	var fileFilterQuery string
	var fileFilterMode bool

	// Path exclusion state ('!' in the file list), for hiding generated code
	// and other noise. Empty means nothing is excluded.
	var fileExcludeQuery string

	// Diff content grep state ('/' in the file list). diffGrepHits is nil while
	// no grep is active and holds per-file hit counts otherwise.
	var diffGrepQuery string
	var diffGrepHits map[string]int
	// narrowingStatusText renders whatever is narrowing the list for the status
	// bar. It is assigned further down, once the state it reads exists.
	var narrowingStatusText func() string

	// Search state
	var searchQuery string
	var searchMatches []int
	var searchMatchIndex int = -1
	var isSearchMode bool = false
	var searchInput string
	var searchCursorYBeforeSearch int

	// Fold state for managing expandable ranges
	foldState := NewFoldState()

	// Function to update the status bar title
	updateStatusTitle := func() {
		var titleParts []string
		if enableAutoRefresh {
			titleParts = append(titleParts, "Watch: on")
		}
		if ignoreWhitespace {
			titleParts = append(titleParts, "Hide whitespace: on")
		}
		if len(titleParts) > 0 {
			globalStatusView.SetTitle(" " + strings.Join(titleParts, " | ") + " ")
			globalStatusView.SetTitleAlign(tview.AlignLeft)
		} else {
			globalStatusView.SetTitle("")
		}
	}
	restoreStatusFunc = func() {
		// Anything narrowing the list owns the status bar until it is cleared,
		// so a running grep, filter or exclusion is never forgotten.
		if narrowingStatusText != nil {
			if text := narrowingStatusText(); text != "" {
				globalStatusView.SetText(text)
				return
			}
		}
		if leftPaneFocused {
			globalStatusView.SetText(fileListKeyMessage)
		} else {
			globalStatusView.SetText(diffViewKeyMessage)
		}
	}

	// Create listStatusView
	globalStatusView = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetWrap(false)
	borderColor := util.CommitAreaBorderColor.ToTcellColor()

	globalStatusView.SetBorder(true)
	globalStatusView.SetBorderColor(borderColor)
	globalStatusView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Create flex layout (vertical split, then horizontal split below)
	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Create text view for left pane (file list)
	fileListView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	fileListView.SetBorder(true).SetTitle("j/k: navigate, Enter: switch to diff")
	fileListView.SetBorderColor(borderColor)
	fileListView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Create text view for right pane (diff display)
	diffView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	diffView.SetBorder(true).SetTitle("Enter: back to list")
	diffView.SetBorderColor(borderColor)
	diffView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Flex container for unified view
	unifiedViewFlex := tview.NewFlex().
		AddItem(diffView, 0, 1, false)
	unifiedViewFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Create text views for split view
	beforeView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	beforeView.SetBorder(true).SetTitle("Before")
	beforeView.SetBorderColor(borderColor)
	beforeView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	afterView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	afterView.SetBorder(true).SetTitle("After")
	afterView.SetBorderColor(borderColor)
	afterView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Flex container for split view
	splitViewFlex := tview.NewFlex().
		AddItem(beforeView, 0, 1, false).
		AddItem(afterView, 0, 1, false)
	splitViewFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Save cursor restore flags (will be restored after fileList is built)
	// An explicit initialFile argument overrides any carried-over state.
	if initialFile != "" {
		savedTargetFile = initialFile
		preferUnstagedSection = false
	}
	needsCursorRestore := preferUnstagedSection || savedTargetFile != ""
	savedPreferUnstaged := preferUnstagedSection
	savedTarget := savedTargetFile
	preferUnstagedSection = false
	savedTargetFile = ""

	// Variables for building the file list
	var fileList []FileEntry
	var lineNumberMap = make(map[int]int)
	dirCollapseState := NewDirCollapseState()

	// findFirstFileIndex finds the nearest file (non-directory) entry.
	// Searches backwards from 'from', then forward from the beginning.
	findFirstFileIndex := func(from int) int {
		if from > 0 {
			for i := from - 1; i >= 0; i-- {
				if i < len(fileList) && !fileList[i].IsDirectory {
					return i
				}
			}
		}
		for i, entry := range fileList {
			if !entry.IsDirectory {
				return i
			}
		}
		return 0
	}

	// findIndexByPathStatus returns the index of the file entry matching the
	// given path and stage status in the current fileList, or -1 if absent.
	findIndexByPathStatus := func(path, status string) int {
		for i, fe := range fileList {
			if !fe.IsDirectory && fe.Path == path && fe.StageStatus == status {
				return i
			}
		}
		return -1
	}

	// nearestSurvivingSelection picks a new selection after the previously
	// selected file disappeared from the (already rebuilt) fileList. It walks
	// upward through the previous ordering to the closest file that still
	// exists, then downward, falling back to the first file. This keeps the
	// cursor visible (one file up) instead of vanishing on --watch refresh.
	nearestSurvivingSelection := func(oldList []FileEntry, oldSel int) int {
		for i := oldSel - 1; i >= 0; i-- {
			if i < len(oldList) && !oldList[i].IsDirectory {
				if idx := findIndexByPathStatus(oldList[i].Path, oldList[i].StageStatus); idx >= 0 {
					return idx
				}
			}
		}
		for i := oldSel + 1; i < len(oldList); i++ {
			if !oldList[i].IsDirectory {
				if idx := findIndexByPathStatus(oldList[i].Path, oldList[i].StageStatus); idx >= 0 {
					return idx
				}
			}
		}
		return findFirstFileIndex(0)
	}

	// Commit message input area
	commitTextArea := tview.NewTextArea().
		SetPlaceholder("Enter commit message (Option+Enter to commit, Ctrl+O to return, Ctrl+L to file list, Esc to cancel)")

	// TextArea style settings
	// Text style (for input characters)
	commitTextArea.SetTextStyle(tcell.StyleDefault.
		Foreground(util.MainTextColor.ToTcellColor()).
		Background(util.BackgroundColor.ToTcellColor()))

	// Placeholder style
	commitTextArea.SetPlaceholderStyle(tcell.StyleDefault.
		Foreground(util.PlaceholderColor.ToTcellColor()).
		Background(util.BackgroundColor.ToTcellColor()))

	// Background color and border settings
	commitTextArea.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	commitTextArea.SetBorder(true)
	commitTextArea.SetBorderColor(util.CommitAreaBorderColor.ToTcellColor())
	commitTextArea.SetTitle("Commit Message")
	commitTextArea.SetTitleAlign(tview.AlignLeft)
	commitTextArea.SetTitleColor(tcell.ColorWhite)

	terminal := newTerminalPane(app, repoRoot)

	// Left-right split flex
	contentFlex := tview.NewFlex()
	contentFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Dynamic layout ratio
	fileListRatio := FileListFlexRatio
	diffViewRatio := DiffViewFlexRatio

	rebuildContentFlex := func() {
		contentFlex.Clear()
		rightPane := unifiedViewFlex
		if isSplitView {
			rightPane = splitViewFlex
		}
		contentFlex.
			AddItem(fileListView, 0, fileListRatio, leftPaneFocused).
			AddItem(rightPane, 0, diffViewRatio, !leftPaneFocused)
	}

	// Layout settings
	contentFlex.
		AddItem(fileListView, 0, FileListFlexRatio, true).
		AddItem(unifiedViewFlex, 0, DiffViewFlexRatio, false)

	// Build file list content (colorized)
	buildFileListContent := func(focusedPane bool) string {
		return BuildFileListContent(
			*stagedFilesPtr,
			*modifiedFilesPtr,
			*untrackedFilesPtr,
			currentSelection,
			focusedPane,
			&fileList,
			lineNumberMap,
			dirCollapseState,
			fileFilterQuery,
			fileExcludeQuery,
			diffGrepHits,
		)
	}

	// Variable to preserve scroll position
	var preserveScrollRow int = -1

	// Build browser file list content
	buildBrowserContent := func(focusedPane bool) string {
		return BuildFileListContentForBrowser(
			browserFiles,
			currentSelection,
			focusedPane,
			&fileList,
			lineNumberMap,
			browserCollapseState,
			browserFilterQuery,
		)
	}

	// scrollToSelection scrolls the file list just enough to show the selected
	// line, from the scroll offset the list had before it was redrawn
	scrollToSelection := func(currentRow, currentCol int) {
		actualLine, exists := lineNumberMap[currentSelection]
		if !exists {
			return
		}
		_, _, _, height := fileListView.GetInnerRect()

		// If selected line is below the screen
		if actualLine >= currentRow+height-1 {
			fileListView.ScrollTo(actualLine-height+2, currentCol)
		} else if actualLine < currentRow {
			// If selected line is above the screen
			// For the first file (currentSelection == 0), scroll to top to show header
			if currentSelection == 0 {
				fileListView.ScrollTo(0, currentCol)
			} else {
				fileListView.ScrollTo(actualLine, currentCol)
			}
		} else {
			// If selected line is within visible range, maintain current scroll position
			fileListView.ScrollTo(currentRow, currentCol)
		}
	}

	// Update initial display
	updateFileListView := func() {
		// File browser mode: use browser content
		if isFileBrowserMode {
			currentRow, currentCol := fileListView.GetScrollOffset()
			fileListView.Clear()
			fileListView.SetText(buildBrowserContent(leftPaneFocused && !browserFilterMode))
			scrollToSelection(currentRow, currentCol)
			return
		}

		// Save current scroll position
		currentRow, currentCol := fileListView.GetScrollOffset()

		// Use preserveScrollRow if set (when returning from viewer)
		shouldPreserveScroll := preserveScrollRow >= 0
		if shouldPreserveScroll {
			currentRow = preserveScrollRow
			preserveScrollRow = -1 // reset
		}

		fileListView.Clear()
		fileListView.SetText(buildFileListContent(leftPaneFocused && !fileFilterMode))

		// Scroll position handling
		if shouldPreserveScroll {
			// Restore scroll position when returning from viewer
			fileListView.ScrollTo(currentRow, currentCol)
		} else {
			// During normal operation, scroll to keep selected line visible
			scrollToSelection(currentRow, currentCol)
		}
	}

	// narrowingStatusText reports every narrowing that is running — the diff
	// grep, the path exclusion and the file name filter — so what is hidden
	// stays on screen until it is cleared. Returns "" when nothing narrows the
	// list. Counts come from the rendered list rather than the raw results,
	// because the three narrowings stack on top of each other.
	narrowingStatusText = func() string {
		shownFiles := 0
		for _, entry := range fileList {
			if !entry.IsDirectory {
				shownFiles++
			}
		}

		var parts []string

		if diffGrepQuery != "" {
			files, hits := 0, 0
			for _, entry := range fileList {
				if entry.IsDirectory {
					continue
				}
				if n, ok := diffGrepHits[git.DiffGrepKey(entry.StageStatus, entry.Path)]; ok {
					files++
					hits += n
				}
			}
			if files > 0 {
				parts = append(parts, fmt.Sprintf("[white]%s%s [%d files, %d hits][-]",
					diffGrepPrompt, tview.Escape(diffGrepQuery), files, hits))
			} else {
				parts = append(parts, fmt.Sprintf("[tomato]%s%s [no match][-]",
					diffGrepPrompt, tview.Escape(diffGrepQuery)))
			}
		}

		if fileExcludeQuery != "" {
			hidden := 0
			for _, list := range [][]git.FileInfo{*stagedFilesPtr, *modifiedFilesPtr, *untrackedFilesPtr} {
				for _, fileInfo := range list {
					if matchesFilter(FileEntry{Path: fileInfo.Path}, fileExcludeQuery) {
						hidden++
					}
				}
			}
			parts = append(parts, fmt.Sprintf("[white]%s%s [%d hidden][-]",
				fileExcludePrompt, tview.Escape(fileExcludeQuery), hidden))
		}

		if fileFilterQuery != "" {
			if shownFiles > 0 {
				parts = append(parts, fmt.Sprintf("[white]%s%s [%d matched][-]",
					fileFilterPrompt, tview.Escape(fileFilterQuery), shownFiles))
			} else {
				parts = append(parts, fmt.Sprintf("[tomato]%s%s [no match][-]",
					fileFilterPrompt, tview.Escape(fileFilterQuery)))
			}
		}

		if len(parts) == 0 {
			return ""
		}
		if diffGrepQuery != "" {
			parts = append(parts, "[white]n/N:hit[-]")
		}
		if leftPaneFocused {
			// Esc clears the narrowings only from the file list.
			parts = append(parts, "[white]Esc:clear[-]")
		}
		return strings.Join(parts, "  ")
	}

	// runDiffGrep greps the changed lines of every pending diff and remembers
	// the hits, so the file list can be narrowed to the matching files.
	runDiffGrep := func(query string) error {
		hits, err := git.GrepDiff(repoRoot, query, *untrackedFilesPtr, ignoreWhitespace)
		if err != nil {
			return err
		}
		diffGrepQuery = query
		diffGrepHits = hits
		return nil
	}

	// refreshDiffGrep re-runs the active grep against the current working tree
	// so files that started or stopped matching enter or leave the narrowed
	// list, for instance after staging or discarding a change.
	refreshDiffGrep := func() {
		if diffGrepQuery == "" {
			return
		}
		if hits, err := git.GrepDiff(repoRoot, diffGrepQuery, *untrackedFilesPtr, ignoreWhitespace); err == nil {
			diffGrepHits = hits
		}
	}

	// Function to internally update the file list
	refreshFileList := func() {
		// Get new file list
		newStaged, newModified, newUntracked, err := git.GetChangedFiles(repoRoot)
		if err == nil {
			*stagedFilesPtr = newStaged
			*modifiedFilesPtr = newModified
			*untrackedFilesPtr = newUntracked
		}
		refreshDiffGrep()
	}

	// applyGrepSearchToDiff installs the active diff grep as the diff view's
	// search query, so its matches stay highlighted when a file is opened and
	// n/N can walk them. It leaves a manual '/' search untouched.
	applyGrepSearchToDiff := func() {
		if diffGrepQuery == "" {
			return
		}
		searchQuery = diffGrepQuery
		searchMatches = nil
		searchMatchIndex = -1
		if currentDiffText == "" {
			return
		}
		searchMatches = findSearchMatches(currentDiffText, isSplitView, foldState, currentFile, repoRoot, searchQuery)
		if len(searchMatches) > 0 {
			searchMatchIndex = 0
			cursorY = searchMatches[0]
			searchCursorYBeforeSearch = cursorY
		}
	}

	// Function to update the diff of the selected file
	updateSelectedFileDiff := func() {
		// File browser mode: show file content
		if isFileBrowserMode {
			if len(fileList) == 0 {
				diffView.SetText("No files")
				return
			}
			if currentSelection < 0 {
				currentSelection = 0
			} else if currentSelection >= len(fileList) {
				currentSelection = len(fileList) - 1
			}
			entry := fileList[currentSelection]
			if entry.IsDirectory {
				diffView.SetText("dir: " + entry.Path + "/")
				currentFile = ""
				currentDiffText = ""
				return
			}
			currentFile = entry.Path
			var content string
			isBinary, err := util.IsBinaryFile(entry.Path, browserRoot)
			if err == nil {
				if isBinary {
					content = util.FormatBinaryNotice(entry.Path)
				} else {
					content, err = util.ReadFileContent(entry.Path, browserRoot)
				}
			}
			if err != nil {
				diffView.SetText("[red]Error reading file: " + err.Error() + "[-]")
				currentDiffText = ""
				return
			}
			currentDiffText = content
			cursorY = 0
			renderFileView(diffView, content, -1, -1, -1, false, currentFile, "")
			return
		}

		// Adjust selection range
		if len(fileList) == 0 {
			// If file list is empty
			currentDiffText = "No file content ✨"
			currentFile = ""
			currentStatus = ""
			if isSplitView {
				beforeView.SetText("")
				afterView.SetText("No file content ✨")
			} else {
				diffView.SetText("No file content ✨")
			}
			return
		}

		// Adjust if currentSelection is out of range
		if currentSelection < 0 {
			currentSelection = 0
		} else if currentSelection >= len(fileList) {
			currentSelection = len(fileList) - 1
		}

		fileEntry := fileList[currentSelection]

		// Show path when directory is selected
		if fileEntry.IsDirectory {
			currentDiffText = ""
			currentFile = ""
			currentStatus = ""
			dirText := "dir: " + fileEntry.Path + "/"
			if isSplitView {
				beforeView.SetText("")
				afterView.SetText(dirText)
			} else {
				diffView.SetText(dirText)
			}
			return
		}

		file := fileEntry.Path
		status := fileEntry.StageStatus

		// Update current file info
		currentFile = file
		currentStatus = status

		// Reset cursor and selection
		cursorY = 0
		isSelecting = false
		selectStart = -1
		selectEnd = -1

		updateCurrentDiffText(file, status, repoRoot, &currentDiffText, ignoreWhitespace)
		applyGrepSearchToDiff()

		if isSplitView && diffGrepQuery != "" {
			// Show where the grep matched, scrolling to the first match.
			updateSplitViewHighlighted(beforeView, afterView, currentDiffText, cursorY, foldState, currentFile, repoRoot, searchQuery)
		} else if isSplitView {
			updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, foldState, currentFile, repoRoot)
		} else if diffGrepQuery != "" {
			// Show where the grep matched, scrolling to the first match.
			updateDiffViewHighlighted(diffView, currentDiffText, cursorY, foldState, currentFile, repoRoot, searchQuery)
		} else {
			updateDiffViewWithoutCursor(diffView, currentDiffText, foldState, currentFile, repoRoot)
		}

	}

	updateFileListView()

	// Restore cursor position (executed after fileList is built)
	if needsCursorRestore {
		for i, entry := range fileList {
			if entry.IsDirectory {
				continue
			}
			if !savedPreferUnstaged && entry.Path == savedTarget {
				currentSelection = i
				break
			}
			if savedPreferUnstaged && entry.StageStatus == "unstaged" {
				currentSelection = i
				break
			}
		}
		updateFileListView()
	}

	// If initial selection is a directory, select the first file
	if currentSelection < len(fileList) && fileList[currentSelection].IsDirectory {
		for i, entry := range fileList {
			if !entry.IsDirectory {
				currentSelection = i
				updateFileListView()
				break
			}
		}
	}

	// Show diff of the first file on initial display
	updateSelectedFileDiff()

	// Set initial message
	globalStatusView.SetText(fileListKeyMessage)
	updateStatusTitle()

	// Terminal open callback (shared between diff view and file list)
	// Set below; the terminal refreshes the diff view through it
	var diffViewContext *DiffViewContext

	openTerminalFunc := func() {
		var focusBefore tview.Primitive = fileListView
		if !leftPaneFocused {
			if isSplitView {
				focusBefore = splitViewFlex
			} else {
				focusBefore = diffView
			}
		}
		// Refresh after each command in case it changed the git state
		terminal.Open(mainFlex, focusBefore, func() {
			refreshFileList()
			updateFileListView()
			if leftPaneFocused {
				updateSelectedFileDiff()
			} else if currentFile != "" {
				// The terminal was opened from the diff view: keep its cursor
				updateCurrentDiffText(currentFile, currentStatus, repoRoot, &currentDiffText, ignoreWhitespace)
				if diffViewContext.viewUpdater != nil {
					diffViewContext.viewUpdater.UpdateWithCursor(currentDiffText, cursorY)
				}
			}
		})
	}

	// Pull request diff callback ('pr', shared between diff view and file list):
	// shows the branch's changes as GitHub does, for when they are already
	// committed and pushed and no longer show up as local changes
	openPRDiffFunc := func() {
		globalStatusView.SetText("[dimgray]Loading the pull request diff...[-]")
		go func() {
			base, err := git.PRBaseRef(repoRoot)
			var output string
			if err == nil {
				output, err = git.GetPRChangedFiles(base, repoRoot)
			}
			app.QueueUpdateDraw(func() {
				if err != nil {
					updateGlobalStatus(err.Error(), "tomato")
					return
				}
				files := parseNameStatus(output)
				if len(files) == 0 {
					updateGlobalStatus("No changes against "+base, "tomato")
					return
				}
				restoreStatusFunc()

				focusBefore := app.GetFocus()
				prView := tview.NewFlex()
				showReadOnlyDiff(app, prView, repoRoot, "PR: "+base+"...HEAD", files, func(opts []string, paths ...string) []string {
					extra := append(append([]string{}, opts...), "--")
					return git.PRDiffArgs(base, append(extra, paths...)...)
				}, func() ([]FileEntry, error) {
					output, err := git.GetPRChangedFiles(base, repoRoot)
					if err != nil {
						return nil, err
					}
					return parseNameStatus(output), nil
				}, func() {
					app.SetRoot(mainFlex, true)
					app.SetFocus(focusBefore)
				})
				app.SetRoot(prView, true)
			})
		}()
	}

	undoStack := NewUndoStack(repoRoot)

	// Set up key input handling for right pane (same behavior as file_view.go)
	// Set up diff view key bindings
	diffViewContext = &DiffViewContext{
		// UI Components
		diffView:        diffView,
		fileListView:    fileListView,
		beforeView:      beforeView,
		afterView:       afterView,
		splitViewFlex:   splitViewFlex,
		unifiedViewFlex: unifiedViewFlex,
		contentFlex:     contentFlex,
		app:             app,
		mainView:        mainFlex,

		// State
		cursorY:               &cursorY,
		selectStart:           &selectStart,
		selectEnd:             &selectEnd,
		isSelecting:           &isSelecting,
		isSplitView:           &isSplitView,
		leftPaneFocused:       &leftPaneFocused,
		currentDiffText:       &currentDiffText,
		currentFile:           &currentFile,
		currentStatus:         &currentStatus,
		savedTargetFile:       &savedTargetFile,
		preferUnstagedSection: &preferUnstagedSection,
		currentSelection:      &currentSelection,
		fileList:              &fileList,
		preserveScrollRow:     &preserveScrollRow,
		ignoreWhitespace:      &ignoreWhitespace,

		// Paths
		repoRoot:  repoRoot,
		patchPath: patchFilePath,

		// Key handling state
		gPressed:  &gPressed,
		lastGTime: &lastGTime,

		// View updater
		viewUpdater: &UnifiedViewUpdater{
			diffView:    diffView,
			foldState:   foldState,
			filePath:    &currentFile,
			repoRoot:    repoRoot,
			searchQuery: &searchQuery,
		},

		// Fold state
		foldState: foldState,

		// Diff content grep state
		diffGrepQuery: &diffGrepQuery,

		// Search state
		searchQuery:               &searchQuery,
		searchMatches:             &searchMatches,
		searchMatchIndex:          &searchMatchIndex,
		isSearchMode:              &isSearchMode,
		searchInput:               &searchInput,
		searchCursorYBeforeSearch: &searchCursorYBeforeSearch,

		// Undo
		undoStack: undoStack,

		// Callbacks
		updateFileListView: updateFileListView,
		updateGlobalStatus: updateGlobalStatus,
		setGlobalStatusText: func(text string) {
			if globalStatusView != nil {
				globalStatusView.SetText(text)
			}
		},
		refreshFileList:       refreshFileList,
		onUpdate:              onUpdate,
		updateCurrentDiffText: updateCurrentDiffText,
		updateStatusTitle:     updateStatusTitle,
		openTerminal:          openTerminalFunc,
		openPRDiff:            openPRDiffFunc,
		resizeFileList: func(delta int) {
			newFileRatio := fileListRatio + delta
			newDiffRatio := diffViewRatio - delta
			if newFileRatio < 1 || newDiffRatio < 1 {
				return
			}
			fileListRatio = newFileRatio
			diffViewRatio = newDiffRatio
			rebuildContentFlex()
		},
	}
	SetupDiffViewKeyBindings(diffViewContext)

	// Set up file list key bindings
	fileListKeyContext := &FileListKeyContext{
		// UI Components
		fileListView:    fileListView,
		diffView:        diffView,
		beforeView:      beforeView,
		afterView:       afterView,
		splitViewFlex:   splitViewFlex,
		unifiedViewFlex: unifiedViewFlex,
		contentFlex:     contentFlex,
		app:             app,
		mainView:        mainFlex, // add reference to main view

		// State
		currentSelection:  &currentSelection,
		cursorY:           &cursorY,
		isSelecting:       &isSelecting,
		selectStart:       &selectStart,
		selectEnd:         &selectEnd,
		isSplitView:       &isSplitView,
		leftPaneFocused:   &leftPaneFocused,
		currentFile:       &currentFile,
		currentStatus:     &currentStatus,
		currentDiffText:   &currentDiffText,
		preserveScrollRow: &preserveScrollRow,
		ignoreWhitespace:  &ignoreWhitespace,
		isFilterMode:      &fileFilterMode,
		filterQuery:       &fileFilterQuery,
		excludeQuery:      &fileExcludeQuery,
		diffGrepQuery:     &diffGrepQuery,
		diffGrepHits:      &diffGrepHits,
		runDiffGrep:       runDiffGrep,

		// Key handling state (shared with diff view so `gg` times out together)
		gPressed:  &gPressed,
		lastGTime: &lastGTime,

		// Collections
		fileList: &fileList,

		// Directory collapse state
		dirCollapseState: dirCollapseState,

		// Paths
		repoRoot: repoRoot,

		// Diff view context
		diffViewContext: diffViewContext,

		// Undo
		undoStack: undoStack,

		// Callbacks
		updateFileListView:     updateFileListView,
		updateSelectedFileDiff: updateSelectedFileDiff,
		refreshFileList:        refreshFileList,
		updateCurrentDiffText:  updateCurrentDiffText,
		updateGlobalStatus:     updateGlobalStatus,
		updateStatusTitle:      updateStatusTitle,
		setGlobalStatusText: func(text string) {
			if globalStatusView != nil {
				globalStatusView.SetText(text)
			}
		},
		openTerminal:      openTerminalFunc,
		openPRDiff:        openPRDiffFunc,
		isFileBrowserMode: &isFileBrowserMode,
		resizeFileList: func(delta int) {
			newFileRatio := fileListRatio + delta
			newDiffRatio := diffViewRatio - delta
			if newFileRatio < 1 || newDiffRatio < 1 {
				return
			}
			fileListRatio = newFileRatio
			diffViewRatio = newDiffRatio
			rebuildContentFlex()
		},
	}
	SetupFileListKeyBindings(fileListKeyContext)

	// advanceGrepFile opens the next (direction > 0) or previous file of the
	// grep-narrowed list and puts the cursor on its first or last match. It
	// returns false, leaving the selection untouched, when no further file has
	// a match to jump to.
	diffViewContext.advanceGrepFile = func(direction int) bool {
		if diffGrepQuery == "" || direction == 0 {
			return false
		}
		startSelection := currentSelection
		for i := currentSelection + direction; i >= 0 && i < len(fileList); i += direction {
			if fileList[i].IsDirectory {
				continue
			}
			currentSelection = i
			updateFileListView()
			updateSelectedFileDiff() // reinstalls the grep search for this file
			if len(searchMatches) == 0 {
				continue
			}
			if direction > 0 {
				searchMatchIndex = 0
			} else {
				searchMatchIndex = len(searchMatches) - 1
			}
			cursorY = searchMatches[searchMatchIndex]
			if diffViewContext.viewUpdater != nil {
				diffViewContext.viewUpdater.UpdateWithCursor(currentDiffText, cursorY)
			}
			return true
		}
		if currentSelection != startSelection {
			currentSelection = startSelection
			updateFileListView()
			updateSelectedFileDiff()
			if diffViewContext.viewUpdater != nil {
				diffViewContext.viewUpdater.UpdateWithCursor(currentDiffText, cursorY)
			}
		}
		return false
	}

	// What the diff view showed before the file browser opened, so that
	// leaving the browser lands on the same file and line again
	var selectionBeforeBrowser FileEntry
	var cursorBeforeBrowser int
	var splitBeforeBrowser bool

	exitFileBrowser := func() {
		isFileBrowserMode = false
		browserRoot = repoRoot
		fileListKeyContext.dirCollapseState = dirCollapseState
		fileListKeyContext.filterQuery = &fileFilterQuery
		fileListKeyContext.isFilterMode = &fileFilterMode
		fileListView.SetTitle("j/k: navigate, Enter: switch to diff")
		// Restore diff view updater
		if splitBeforeBrowser {
			isSplitView = true
			diffViewContext.viewUpdater = NewSplitViewUpdater(beforeView, afterView, foldState, &currentFile, repoRoot, &searchQuery)
			rebuildContentFlex()
		} else {
			diffViewContext.viewUpdater = &UnifiedViewUpdater{
				diffView:    diffView,
				foldState:   foldState,
				filePath:    &currentFile,
				repoRoot:    repoRoot,
				searchQuery: &searchQuery,
			}
		}
		updateFileListView()
		for i, entry := range fileList {
			if entry.Path == selectionBeforeBrowser.Path && entry.StageStatus == selectionBeforeBrowser.StageStatus && entry.IsDirectory == selectionBeforeBrowser.IsDirectory {
				currentSelection = i
				break
			}
		}
		updateFileListView()
		updateSelectedFileDiff()
		if !leftPaneFocused {
			cursorY = cursorBeforeBrowser
			if diffViewContext.viewUpdater != nil {
				diffViewContext.viewUpdater.UpdateWithCursor(currentDiffText, cursorY)
			}
			if isSplitView {
				app.SetFocus(splitViewFlex)
			} else {
				app.SetFocus(diffView)
			}
		}
		if restoreStatusFunc != nil {
			restoreStatusFunc()
		}
	}

	// enterFileBrowser lists every tracked file with the directories collapsed.
	// With a target path, the directories leading to it are opened, it is
	// selected, and its content is shown with the cursor on line (1-based).
	enterFileBrowser := func(target string, line int) {
		browserRoot = repoRoot
		allFiles, err := git.GetAllTrackedFiles(repoRoot)
		if err != nil {
			updateGlobalStatus("Failed to get file list: "+err.Error(), "tomato")
			return
		}
		if target != "" {
			if _, err := os.Stat(filepath.Join(repoRoot, target)); err != nil {
				updateGlobalStatus("Cannot open "+target+": the file does not exist", "tomato")
				return
			}
			// An untracked file is not in ls-files.
			tracked := false
			for _, f := range allFiles {
				if f.Path == target {
					tracked = true
					break
				}
			}
			if !tracked {
				allFiles = append(allFiles, git.FileInfo{Path: target})
			}
		}

		selectionBeforeBrowser = FileEntry{}
		if currentSelection >= 0 && currentSelection < len(fileList) {
			selectionBeforeBrowser = fileList[currentSelection]
		}
		cursorBeforeBrowser = cursorY
		// The file viewer draws in the unified pane.
		splitBeforeBrowser = isSplitView
		if isSplitView {
			isSplitView = false
			rebuildContentFlex()
		}

		browserFiles = allFiles
		isFileBrowserMode = true
		currentSelection = 0
		browserFilterQuery = ""
		browserFilterMode = false
		fileFilterQuery = ""
		fileFilterMode = false
		// The browser lists plain files, which no diff grep applies to.
		diffGrepQuery = ""
		diffGrepHits = nil
		fileExcludeQuery = ""
		searchQuery = ""
		searchMatches = nil
		searchMatchIndex = -1
		// Collapse all directories by default
		browserCollapseState = NewDirCollapseState()
		// Build initial list to find directories
		BuildFileListContentForBrowser(browserFiles, 0, true, &fileList, lineNumberMap, browserCollapseState, "")
		for _, entry := range fileList {
			if entry.IsDirectory {
				browserCollapseState.SetCollapsed("browser", entry.Path, true)
			}
		}
		if target != "" {
			for dir := filepath.Dir(target); dir != "." && dir != "/"; dir = filepath.Dir(dir) {
				browserCollapseState.SetCollapsed("browser", filepath.ToSlash(dir), false)
			}
			BuildFileListContentForBrowser(browserFiles, 0, true, &fileList, lineNumberMap, browserCollapseState, "")
			for i, entry := range fileList {
				if !entry.IsDirectory && entry.Path == target {
					currentSelection = i
					break
				}
			}
		}
		fileListKeyContext.dirCollapseState = browserCollapseState
		fileListKeyContext.filterQuery = &browserFilterQuery
		fileListKeyContext.isFilterMode = &browserFilterMode
		fileListView.SetTitle("File Browser (f: back to diff)")
		// Switch diff view to file viewer updater
		diffViewContext.viewUpdater = &FileViewUpdater{
			diffView:    diffView,
			filePath:    &currentFile,
			searchQuery: &searchQuery,
		}
		updateFileListView()
		updateSelectedFileDiff()
		if target != "" && !leftPaneFocused {
			if lineCount := len(util.SplitLines(currentDiffText)); line > 0 && lineCount > 0 {
				cursorY = min(line-1, lineCount-1)
			}
			diffViewContext.viewUpdater.UpdateWithCursor(currentDiffText, cursorY)
			app.SetFocus(diffView)
		}
		if restoreStatusFunc != nil {
			restoreStatusFunc()
		}
	}

	// Set toggleFileBrowser after fileListKeyContext is created (needs reference)
	fileListKeyContext.toggleFileBrowser = func() {
		if isFileBrowserMode {
			exitFileBrowser()
		} else {
			enterFileBrowser("", 0)
		}
	}

	// 'f' in the diff pane shows the whole file under the cursor, and back.
	diffViewContext.toggleFileView = func() {
		if isFileBrowserMode {
			exitFileBrowser()
		} else if currentFile != "" {
			enterFileBrowser(currentFile, getCursorFileLineNumber(diffViewContext))
		}
	}

	// If an external file path was supplied (outside the repo), bootstrap the
	// file browser with just that file so the user lands directly in file view.
	if externalFile != "" {
		browserRoot = filepath.Dir(externalFile)
		browserFiles = []git.FileInfo{{Path: filepath.Base(externalFile)}}
		isFileBrowserMode = true
		currentSelection = 0
		browserFilterQuery = ""
		browserFilterMode = false
		browserCollapseState = NewDirCollapseState()
		fileListKeyContext.dirCollapseState = browserCollapseState
		fileListKeyContext.filterQuery = &browserFilterQuery
		fileListKeyContext.isFilterMode = &browserFilterMode
		fileListView.SetTitle("File Browser (f: back to diff)")
		diffViewContext.viewUpdater = &FileViewUpdater{
			diffView:    diffView,
			filePath:    &currentFile,
			searchQuery: &searchQuery,
		}
		updateFileListView()
		updateSelectedFileDiff()
	}

	// Start goroutine only when auto-refresh is enabled
	if enableAutoRefresh {
		stopRefresh := make(chan bool)

		// Variables to cache previous file list
		var lastStagedFiles, lastModifiedFiles, lastUntrackedFiles []git.FileInfo

		// Function to determine if file list has changed
		hasFileListChanged := func(newStaged, newModified, newUntracked []git.FileInfo) bool {
			if len(lastStagedFiles) != len(newStaged) ||
				len(lastModifiedFiles) != len(newModified) ||
				len(lastUntrackedFiles) != len(newUntracked) {
				return true
			}

			// Compare contents of each file list
			for i := range newStaged {
				if lastStagedFiles[i].Path != newStaged[i].Path ||
					lastStagedFiles[i].ChangeStatus != newStaged[i].ChangeStatus {
					return true
				}
			}

			for i := range newModified {
				if lastModifiedFiles[i].Path != newModified[i].Path ||
					lastModifiedFiles[i].ChangeStatus != newModified[i].ChangeStatus {
					return true
				}
			}

			for i := range newUntracked {
				if lastUntrackedFiles[i].Path != newUntracked[i].Path ||
					lastUntrackedFiles[i].ChangeStatus != newUntracked[i].ChangeStatus {
					return true
				}
			}

			return false
		}

		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Skip auto-refresh in file browser mode
					if isFileBrowserMode {
						continue
					}
					// Get new file list
					newStaged, newModified, newUntracked, err := git.GetChangedFiles(repoRoot)
					if err != nil {
						continue // do nothing if error occurs
					}

					// Also check for diff changes in the currently selected file
					var currentFileDiffChanged bool = false
					var newDiffText string
					if currentFile != "" {
						// Use the same retrieval logic as the main path so
						// transformations (e.g. binary file notice) match
						updateCurrentDiffText(currentFile, currentStatus, repoRoot, &newDiffText, ignoreWhitespace)
						currentFileDiffChanged = (newDiffText != currentDiffText)
					}

					// Check if file list has changed
					fileListChanged := hasFileListChanged(newStaged, newModified, newUntracked)

					// Do nothing if neither file list nor current file diff has changed
					if !fileListChanged && !currentFileDiffChanged {
						continue
					}

					app.QueueUpdateDraw(func() {

						// Save currently selected file and status
						var currentlySelectedFile string
						var currentlySelectedStatus string
						if currentSelection >= 0 && currentSelection < len(fileList) {
							fileEntry := fileList[currentSelection]
							currentlySelectedFile = fileEntry.Path
							currentlySelectedStatus = fileEntry.StageStatus
						}

						// Update cache and list only when file list has changed
						if fileListChanged {
							// Update cache
							lastStagedFiles = newStaged
							lastModifiedFiles = newModified
							lastUntrackedFiles = newUntracked

							// Update file list
							*stagedFilesPtr = newStaged
							*modifiedFilesPtr = newModified
							*untrackedFilesPtr = newUntracked

							// Re-grep so files that started or stopped matching
							// enter or leave the narrowed list.
							refreshDiffGrep()

							// Snapshot the previous ordering before rebuilding so
							// we can fall back to a neighboring file if the
							// selected one disappears.
							oldList := make([]FileEntry, len(fileList))
							copy(oldList, fileList)
							oldSelection := currentSelection

							// Rebuild fileList against the new git state first so
							// selection is resolved against the up-to-date list.
							updateFileListView()

							// Restore selection position (search by both filename and status)
							newSelection := -1
							for i, fileEntry := range fileList {
								if fileEntry.Path == currentlySelectedFile && fileEntry.StageStatus == currentlySelectedStatus {
									newSelection = i
									break
								}
							}
							if newSelection >= 0 {
								currentSelection = newSelection
							} else {
								// Selected file's diff disappeared (e.g. edited away
								// by an external tool under --watch). Move the cursor
								// to the nearest preceding file that still exists so
								// the highlight stays visible.
								currentSelection = nearestSurvivingSelection(oldList, oldSelection)
								// If focus is on diff view, return to file list
								if !leftPaneFocused {
									leftPaneFocused = true
									restoreStatusFunc()
									app.SetFocus(fileListView)
								}
							}

							// Re-render with the resolved selection.
							updateFileListView()
						}

						// Update right pane diff
						if leftPaneFocused {
							// If left pane is focused
							if fileListChanged {
								updateSelectedFileDiff()
							} else if currentFileDiffChanged {
								// File list hasn't changed but diff content has changed
								currentDiffText = newDiffText
								if isSplitView {
									updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, foldState, currentFile, repoRoot)
								} else {
									updateDiffViewWithoutCursor(diffView, currentDiffText, foldState, currentFile, repoRoot)
								}
							}
						} else if currentFile != "" {
							// If right pane is focused, update while preserving cursor position
							// Only update if diff has changed
							if newDiffText != currentDiffText {
								currentDiffText = newDiffText

								// Handle empty diff
								if len(currentDiffText) == 0 {
									if isSplitView {
										beforeView.SetText("")
										afterView.SetText("No differences")
									} else {
										diffView.SetText("No differences")
									}
									return
								}

								coloredDiff := ColorizeDiff(currentDiffText)
								diffLines := util.SplitLines(coloredDiff)

								// Adjust cursor position (if diff line count decreased)
								if cursorY >= len(diffLines) {
									cursorY = len(diffLines) - 1
									if cursorY < 0 {
										cursorY = 0
									}
								}

								// Also adjust selection range
								if isSelecting {
									if selectStart >= len(diffLines) {
										selectStart = len(diffLines) - 1
									}
									if selectEnd >= len(diffLines) {
										selectEnd = len(diffLines) - 1
									}
								}

								// Update split view if in split mode, otherwise normal update
								if isSplitView {
									updateSplitViewWithCursor(beforeView, afterView, currentDiffText, cursorY, foldState, currentFile, repoRoot)
								} else {
									updateDiffViewWithCursor(diffView, currentDiffText, cursorY, foldState, currentFile, repoRoot)
								}
							}
						}
					})
				case <-stopRefresh:
					return
				}
			}
		}()
	}

	exitCommitMode := func() {
		isCommitMode = false
		isAmendMode = false
		leftPaneFocused = true
		restoreStatusFunc()
		commitTextArea.SetText("", false)
		commitTextArea.SetTitle("Commit Message")
		mainFlex.RemoveItem(commitTextArea)
		app.SetFocus(fileListView)
	}

	commitTextArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEnter:
			// Option+Enter (Alt+Enter) to execute commit
			if event.Modifiers()&tcell.ModAlt != 0 {
				commitMessage = commitTextArea.GetText()
				if commitMessage == "" {
					updateGlobalStatus("Commit message cannot be empty", "tomato")
					return nil
				}

				var err error
				if isAmendMode {
					err = git.CommitAmend(commitMessage, repoRoot)
				} else {
					err = git.Commit(commitMessage, repoRoot)
				}
				if err != nil {
					if isAmendMode {
						updateGlobalStatus("Failed to amend commit: "+err.Error(), "tomato")
					} else {
						updateGlobalStatus("Failed to commit: "+err.Error(), "tomato")
					}
					// On error, don't exit commit mode so user can retry with message preserved
					return nil
				}

				// Update file list after commit
				refreshFileList()

				// Move cursor to the first file after commit
				currentSelection = findFirstFileIndex(0)

				// Update view again to reflect correct selection position
				updateFileListView()
				updateSelectedFileDiff()

				exitCommitMode()

				if isAmendMode {
					updateGlobalStatus("Successfully amended commit", "forestgreen")
				} else {
					updateGlobalStatus("Successfully committed", "forestgreen")
				}
				return nil
			}
			// Normal Enter is handled as newline
			return event
		case tcell.KeyCtrlL:
			app.SetFocus(fileListView)
			return nil
		case tcell.KeyCtrlO:
			// Return to previous focus position before commit mode
			if focusBeforeCommit != nil {
				app.SetFocus(focusBeforeCommit)
			}
			return nil
		case tcell.KeyEsc:
			exitCommitMode()
			return nil
		}
		return event
	})

	// Add status view and content to mainFlex
	mainFlex.AddItem(globalStatusView, 3, 0, false).
		AddItem(contentFlex, 0, 1, true)

	mainFlex.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Once the commit text area is open, let it handle Ctrl+K (kill to
		// end of line) and Ctrl+J itself instead of swallowing them here.
		if isCommitMode {
			// Ctrl+O temporarily moves focus out of the commit area while
			// keeping it visible; let Ctrl+K bring focus back to it.
			if event.Key() == tcell.KeyCtrlK && app.GetFocus() != commitTextArea {
				app.SetFocus(commitTextArea)
				return nil
			}
			return event
		}
		if event.Key() == tcell.KeyCtrlK {
			if len(*stagedFilesPtr) == 0 {
				updateGlobalStatus("No changes are staged for commit", "tomato")
				return nil
			}

			if leftPaneFocused {
				focusBeforeCommit = fileListView
			} else if isSplitView {
				focusBeforeCommit = splitViewFlex
			} else {
				focusBeforeCommit = diffView
			}
			isCommitMode = true
			isAmendMode = false
			mainFlex.AddItem(commitTextArea, 7, 0, true) // Height set to 7 to support multi-line input
			app.SetFocus(commitTextArea)
			return nil
		}
		if event.Key() == tcell.KeyCtrlJ {
			if len(*stagedFilesPtr) == 0 {
				updateGlobalStatus("No changes are staged for commit", "tomato")
				return nil
			}

			cmd := exec.Command("git", "log", "-1", "--pretty=%B")
			cmd.Dir = repoRoot
			output, err := cmd.Output()
			var lastCommitMsg string
			if err == nil {
				lastCommitMsg = strings.TrimSpace(string(output))
			}

			if leftPaneFocused {
				focusBeforeCommit = fileListView
			} else if isSplitView {
				focusBeforeCommit = splitViewFlex
			} else {
				focusBeforeCommit = diffView
			}
			isCommitMode = true
			isAmendMode = true
			commitTextArea.SetTitle("Commit Message (Amend)")
			commitTextArea.SetText(lastCommitMsg, false)
			mainFlex.AddItem(commitTextArea, 7, 0, true)
			app.SetFocus(commitTextArea)
			return nil
		}
		return event
	})

	return mainFlex
}
