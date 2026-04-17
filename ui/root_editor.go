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
var fileListKeyMessage = "a:stage  A:stage file  d:discard  C-a:stage all  C-k:commit  C-j:amend  H/L:dir  s:split  w:ws  /:filter  v:editor  c:code  C-l:log  t:terminal  Y:copy  C-e/C-y:scroll  Enter:switch  q:quit"
var diffViewKeyMessage = "a:stage lines  A:stage file  V:select  g/G:top/end  /:search  e:fold  s:split  w:ws  y:yank  Y:copy path  C-e/C-y:scroll  Esc:back  q:quit"

// restoreStatusFunc is called to restore the default status message (set by SetupRootEditor)
var restoreStatusFunc func()

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
		content, readErr := util.ReadFileContent(filePath, repoRoot)
		if readErr != nil {
			err = readErr
		} else {
			diffText = util.FormatAsAddedLines(content, filePath)
		}
	default:
		diffText, err = git.GetFileDiffWithOptions(filePath, repoRoot, ignoreWhitespace)
	}

	if err != nil {
		diffText = fmt.Sprintf("Error getting diff for %s: %v\n\nThis might be a deleted file or there might be an issue with git.", filePath, err)
	}

	if currentDiffText != nil {
		*currentDiffText = diffText
	}
}

func RootEditor(app *tview.Application, stagedFiles, modifiedFiles, untrackedFiles []git.FileInfo, repoRoot string, patchFilePath string, onUpdate func(), enableAutoRefresh bool) tview.Primitive {
	// Keep references for updating file lists
	stagedFilesPtr := &stagedFiles
	modifiedFilesPtr := &modifiedFiles
	untrackedFilesPtr := &untrackedFiles

	// Commit-related state
	var isCommitMode bool = false
	var isAmendMode bool = false
	var commitMessage string = ""
	var focusBeforeCommit tview.Primitive = nil // focus position before commit mode

	// Terminal command mode state
	var isTerminalMode bool = false
	var focusBeforeTerminal tview.Primitive = nil
	shellHistory := util.LoadShellHistory(1000)
	var historyIndex int = -1    // -1 = not browsing history
	var historyStash string = "" // stash current input when browsing

	// File browser mode state
	var isFileBrowserMode bool = false
	var browserFiles []git.FileInfo
	var browserCollapseState = NewDirCollapseState()
	var browserFilterQuery string
	var browserFilterMode bool
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

	// Terminal command input
	terminalInput := tview.NewInputField().
		SetLabel("(giff) $ ").
		SetFieldBackgroundColor(util.BackgroundColor.ToTcellColor()).
		SetLabelStyle(tcell.StyleDefault.
			Foreground(tcell.ColorAqua).
			Background(util.BackgroundColor.ToTcellColor()))
	terminalInput.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	terminalInput.SetBorder(false)

	// Terminal output display
	terminalOutput := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true)
	terminalOutput.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	terminalOutput.SetBorder(false)

	// Terminal flex container (input + output)
	terminalFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(terminalInput, 1, 0, true).
		AddItem(terminalOutput, 0, 1, false)
	terminalFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	terminalFlex.SetBorder(true)
	terminalFlex.SetBorderColor(util.CommitAreaBorderColor.ToTcellColor())
	terminalFlex.SetTitle(" Terminal (Esc to close) ")
	terminalFlex.SetTitleAlign(tview.AlignLeft)
	terminalFlex.SetTitleColor(tcell.ColorWhite)

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

	// Update initial display
	updateFileListView := func() {
		// File browser mode: use browser content
		if isFileBrowserMode {
			fileListView.Clear()
			fileListView.SetText(buildBrowserContent(leftPaneFocused && !browserFilterMode))
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
			if actualLine, exists := lineNumberMap[currentSelection]; exists {
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
			content, err := util.ReadFileContent(entry.Path, repoRoot)
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

		if isSplitView {
			updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, currentFile)
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
	openTerminalFunc := func() {
		if isTerminalMode {
			app.SetFocus(terminalInput)
			return
		}
		isTerminalMode = true
		if leftPaneFocused {
			focusBeforeTerminal = fileListView
		} else if isSplitView {
			focusBeforeTerminal = splitViewFlex
		} else {
			focusBeforeTerminal = diffView
		}
		terminalOutput.Clear()
		terminalInput.SetText("")
		mainFlex.AddItem(terminalFlex, 12, 0, true)
		app.SetFocus(terminalInput)
	}

	// Set up key input handling for right pane (same behavior as file_view.go)
	// Set up diff view key bindings
	diffViewContext := &DiffViewContext{
		// UI Components
		diffView:        diffView,
		fileListView:    fileListView,
		beforeView:      beforeView,
		afterView:       afterView,
		splitViewFlex:   splitViewFlex,
		unifiedViewFlex: unifiedViewFlex,
		contentFlex:     contentFlex,
		app:             app,

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

		// Search state
		searchQuery:               &searchQuery,
		searchMatches:             &searchMatches,
		searchMatchIndex:          &searchMatchIndex,
		isSearchMode:              &isSearchMode,
		searchInput:               &searchInput,
		searchCursorYBeforeSearch: &searchCursorYBeforeSearch,

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

		// Collections
		fileList: &fileList,

		// Directory collapse state
		dirCollapseState: dirCollapseState,

		// Paths
		repoRoot: repoRoot,

		// Diff view context
		diffViewContext: diffViewContext,

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

	// Set toggleFileBrowser after fileListKeyContext is created (needs reference)
	fileListKeyContext.toggleFileBrowser = func() {
		if isFileBrowserMode {
			// Exit file browser mode
			isFileBrowserMode = false
			fileListKeyContext.dirCollapseState = dirCollapseState
			fileListKeyContext.filterQuery = &fileFilterQuery
			fileListKeyContext.isFilterMode = &fileFilterMode
			fileListView.SetTitle("j/k: navigate, Enter: switch to diff")
			// Restore diff view updater
			diffViewContext.viewUpdater = &UnifiedViewUpdater{
				diffView:    diffView,
				foldState:   foldState,
				filePath:    &currentFile,
				repoRoot:    repoRoot,
				searchQuery: &searchQuery,
			}
			updateFileListView()
			updateSelectedFileDiff()
			if restoreStatusFunc != nil {
				restoreStatusFunc()
			}
		} else {
			// Enter file browser mode
			allFiles, err := git.GetAllTrackedFiles(repoRoot)
			if err != nil {
				updateGlobalStatus("Failed to get file list: "+err.Error(), "tomato")
				return
			}
			browserFiles = allFiles
			isFileBrowserMode = true
			currentSelection = 0
			browserFilterQuery = ""
			browserFilterMode = false
			fileFilterQuery = ""
			fileFilterMode = false
			// Collapse all directories by default
			browserCollapseState = NewDirCollapseState()
			// Build initial list to find directories
			BuildFileListContentForBrowser(browserFiles, 0, true, &fileList, lineNumberMap, browserCollapseState, "")
			for _, entry := range fileList {
				if entry.IsDirectory {
					browserCollapseState.SetCollapsed("browser", entry.Path, true)
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
			if restoreStatusFunc != nil {
				restoreStatusFunc()
			}
		}
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
						if currentStatus == "staged" {
							newDiffText, _ = git.GetStagedDiffWithOptions(currentFile, repoRoot, ignoreWhitespace)
						} else if currentStatus == "untracked" {
							content, readErr := util.ReadFileContent(currentFile, repoRoot)
							if readErr == nil {
								newDiffText = util.FormatAsAddedLines(content, currentFile)
							}
						} else {
							newDiffText, _ = git.GetFileDiffWithOptions(currentFile, repoRoot, ignoreWhitespace)
						}
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
								// If selected file disappeared, find nearest file
								currentSelection = findFirstFileIndex(currentSelection)
								// If focus is on diff view, return to file list
								if !leftPaneFocused {
									leftPaneFocused = true
									restoreStatusFunc()
									app.SetFocus(fileListView)
								}
							}

							// Update display
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
									updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, currentFile)
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
									updateSplitViewWithCursor(beforeView, afterView, currentDiffText, cursorY, currentFile)
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

	exitTerminalMode := func() {
		isTerminalMode = false
		mainFlex.RemoveItem(terminalFlex)
		if focusBeforeTerminal != nil {
			app.SetFocus(focusBeforeTerminal)
		} else {
			app.SetFocus(fileListView)
		}
	}

	// Handle up/down arrow for shell history browsing
	terminalInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyUp:
			if len(shellHistory) == 0 {
				return nil
			}
			if historyIndex == -1 {
				// Start browsing: stash current input as prefix filter
				historyStash = terminalInput.GetText()
			}
			// Find next matching entry
			prefix := strings.ToLower(historyStash)
			for next := historyIndex + 1; next < len(shellHistory); next++ {
				if prefix == "" || strings.HasPrefix(strings.ToLower(shellHistory[next]), prefix) {
					historyIndex = next
					terminalInput.SetText(shellHistory[next])
					return nil
				}
			}
			return nil
		case tcell.KeyDown:
			if historyIndex < 0 {
				return nil
			}
			// Find previous matching entry
			prefix := strings.ToLower(historyStash)
			for next := historyIndex - 1; next >= 0; next-- {
				if prefix == "" || strings.HasPrefix(strings.ToLower(shellHistory[next]), prefix) {
					historyIndex = next
					terminalInput.SetText(shellHistory[next])
					return nil
				}
			}
			// No more matches: back to stashed input
			historyIndex = -1
			terminalInput.SetText(historyStash)
			return nil
		}
		return event
	})

	terminalInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			cmdText := terminalInput.GetText()
			if cmdText == "" {
				return
			}
			terminalInput.SetText("")
			historyIndex = -1
			historyStash = ""

			// Show running state
			terminalOutput.Clear()
			terminalOutput.SetText("[dimgray]running...[-]")

			// Execute command asynchronously to avoid UI freeze
			go func() {
				shell := os.Getenv("SHELL")
				if shell == "" {
					shell = "sh"
				}
				var cmdScript string
				if snapPath := util.GetSnapshotPath(); snapPath != "" {
					cmdScript = fmt.Sprintf("source %q 2>/dev/null; eval %q", snapPath, cmdText)
				} else {
					cmdScript = cmdText
				}
				cmd := exec.Command(shell, "-c", cmdScript)
				cmd.Dir = repoRoot
				output, err := cmd.CombinedOutput()

				app.QueueUpdateDraw(func() {
					// Clear and show result
					terminalOutput.Clear()
					var result strings.Builder
					result.WriteString("[#50E88A]$ " + tview.Escape(cmdText) + "[-]\n")
					if err != nil {
						if len(output) > 0 {
							result.WriteString("[red]" + tview.Escape(string(output)) + "[-]")
							if output[len(output)-1] != '\n' {
								result.WriteString("\n")
							}
						}
						result.WriteString("[red]exit: " + err.Error() + "[-]")
					} else if len(output) > 0 {
						result.WriteString(tview.Escape(string(output)))
						if output[len(output)-1] != '\n' {
							result.WriteString("\n")
						}
					}
					terminalOutput.SetText(result.String())

					// Refresh file list in case git state changed
					refreshFileList()
					updateFileListView()
					updateSelectedFileDiff()
				})
			}()
		case tcell.KeyEscape:
			exitTerminalMode()
		}
	})

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

				if isAmendMode {
					updateGlobalStatus("Successfully amended commit", "forestgreen")
				} else {
					updateGlobalStatus("Successfully committed", "forestgreen")
				}
				// Update file list after commit
				refreshFileList()

				// Adjust selection position after file list is updated
				// Call updateFileListView to rebuild fileList
				updateFileListView()

				// Move cursor to the first file after commit
				currentSelection = findFirstFileIndex(0)

				// Update view again to reflect correct selection position
				updateFileListView()
				updateSelectedFileDiff()

				exitCommitMode()
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
		if event.Key() == tcell.KeyCtrlK {
			// Check if there are staged changes
			if len(*stagedFilesPtr) == 0 {
				updateGlobalStatus("No changes are staged for commit", "tomato")
				return nil
			}

			if !isCommitMode {
				// Save current focus before entering commit mode
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
			} else {
				app.SetFocus(commitTextArea)
			}
			return nil
		}
		if event.Key() == tcell.KeyCtrlJ {
			// Get the latest commit message
			cmd := exec.Command("git", "log", "-1", "--pretty=%B")
			cmd.Dir = repoRoot
			output, err := cmd.Output()
			var lastCommitMsg string
			if err == nil {
				lastCommitMsg = strings.TrimSpace(string(output))
			}

			if !isCommitMode {
				// Save current focus before entering commit mode
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
			} else {
				app.SetFocus(commitTextArea)
			}
			return nil
		}
		return event
	})

	return mainFlex
}
