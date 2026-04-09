package ui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

// GitLogEntry represents a single git log entry
type GitLogEntry struct {
	Hash     string
	Message  string
	Author   string
	Date     string
	FullLine string // original display line
}

// GitLogView manages the git log display and navigation
type GitLogView struct {
	app          *tview.Application
	logView      *tview.TextView
	flex         *tview.Flex
	repoRoot     string
	logEntries   []GitLogEntry
	currentLine  int
	showingCommit bool
	onExit       func()
	scrollOffset int
	commitFiles  []FileEntry
	// State for gg command
	gPressed  *bool
	lastGTime *time.Time
}

// NewGitLogView creates a new git log viewer
func NewGitLogView(app *tview.Application, repoRoot string, onExit func()) *GitLogView {
	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(true)
	logView.SetBorder(true).SetTitle("Git Log (j/k: navigate, Enter: show commit, Esc: exit)")
	logView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	flex := tview.NewFlex().
		AddItem(logView, 0, 1, true)

	glv := &GitLogView{
		app:          app,
		logView:      logView,
		flex:         flex,
		repoRoot:     repoRoot,
		logEntries:   []GitLogEntry{},
		currentLine:  0,
		showingCommit: false,
		onExit:       onExit,
		scrollOffset: 0,
		commitFiles:  []FileEntry{},
		gPressed:     new(bool),
		lastGTime:    new(time.Time),
	}

	glv.setupLogKeyBindings()
	glv.loadGitLog()

	go func() {
		glv.app.QueueUpdateDraw(func() {
			glv.updateSelection()
		})
	}()

	return glv
}

// GetView returns the main flex view
func (glv *GitLogView) GetView() tview.Primitive {
	return glv.flex
}

// convertANSIToTviewColors converts ANSI escape codes to tview color tags
func convertANSIToTviewColors(text string) string {
	patterns := []struct {
		pattern     string
		replacement string
	}{
		{`\x1b\[1;31m`, "[red]"},
		{`\x1b\[31m`, "[red]"},
		{`\x1b\[1;32m`, "[green]"},
		{`\x1b\[32m`, "[green]"},
		{`\x1b\[1;33m`, "[yellow]"},
		{`\x1b\[33m`, "[yellow]"},
		{`\x1b\[1;34m`, "[blue]"},
		{`\x1b\[34m`, "[blue]"},
		{`\x1b\[1;35m`, "[purple]"},
		{`\x1b\[35m`, "[purple]"},
		{`\x1b\[1;36m`, "[cyan]"},
		{`\x1b\[36m`, "[cyan]"},
		{`\x1b\[1;37m`, "[white]"},
		{`\x1b\[37m`, "[white]"},
		{`\x1b\[1m`, ""},
		{`\x1b\[m`, "[-]"},
		{`\x1b\[0m`, "[-]"},
		{`\x1b\[(0;)?39m`, "[-]"},
	}

	result := text
	for _, p := range patterns {
		re := regexp.MustCompile(p.pattern)
		result = re.ReplaceAllString(result, p.replacement)
	}

	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	result = re.ReplaceAllString(result, "")

	return result
}

// loadGitLog executes git log and parses the output
func (glv *GitLogView) loadGitLog() {
	cmd := exec.Command("git", "log", "--graph", "--pretty=format:%C(yellow)%h%Creset - %s %Cgreen(%cr) %C(bold blue)<%an>%Creset", "--color=always", "-n", "200")
	cmd.Dir = glv.repoRoot

	output, err := cmd.Output()
	if err != nil {
		glv.logView.SetText("[red]Failed to load git log: " + err.Error())
		return
	}

	lines := strings.Split(string(output), "\n")
	glv.logEntries = []GitLogEntry{}

	hashRegex := regexp.MustCompile(`([a-f0-9]{7,})`)

	for _, line := range lines {
		if line == "" {
			continue
		}

		cleanLine := convertANSIToTviewColors(line)

		var hash string
		if matches := hashRegex.FindStringSubmatch(line); len(matches) > 0 {
			hash = matches[0]
		}

		entry := GitLogEntry{
			Hash:     hash,
			FullLine: cleanLine,
		}
		glv.logEntries = append(glv.logEntries, entry)
	}

	glv.updateSelection()
}

// updateSelection updates the visual selection in the log view
func (glv *GitLogView) updateSelection() {
	if len(glv.logEntries) == 0 {
		return
	}

	_, _, _, height := glv.logView.GetInnerRect()
	visibleLines := height
	if visibleLines <= 0 {
		visibleLines = 25
	}

	relativePos := glv.currentLine - glv.scrollOffset

	if relativePos < 3 && glv.scrollOffset > 0 {
		glv.scrollOffset = glv.currentLine - 3
		if glv.scrollOffset < 0 {
			glv.scrollOffset = 0
		}
	} else if relativePos >= visibleLines-3 {
		glv.scrollOffset = glv.currentLine - visibleLines + 4
		if glv.scrollOffset < 0 {
			glv.scrollOffset = 0
		}
		if glv.scrollOffset >= len(glv.logEntries) {
			glv.scrollOffset = len(glv.logEntries) - 1
		}
	}

	endLine := glv.scrollOffset + visibleLines
	if endLine > len(glv.logEntries) {
		endLine = len(glv.logEntries)
	}

	if endLine-glv.scrollOffset < 10 && len(glv.logEntries) > 10 {
		endLine = glv.scrollOffset + min(len(glv.logEntries)-glv.scrollOffset, 30)
	}

	var content strings.Builder
	for i := glv.scrollOffset; i < endLine; i++ {
		if i >= len(glv.logEntries) {
			break
		}
		entry := glv.logEntries[i]
		if i == glv.currentLine {
			content.WriteString("[white:blue]" + entry.FullLine + "[-:-]\n")
		} else {
			content.WriteString(entry.FullLine + "\n")
		}
	}

	glv.logView.SetText(content.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// loadCommitFiles loads the list of files changed in a commit
func (glv *GitLogView) loadCommitFiles(commitHash string) {
	cmd := exec.Command(
		"git",
		"-c", "core.quotepath=false",
		"show",
		"--name-status",
		"--format=",
		commitHash,
	)
	cmd.Dir = glv.repoRoot

	output, err := cmd.Output()
	if err != nil {
		glv.commitFiles = []FileEntry{}
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	glv.commitFiles = []FileEntry{}

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		rawStatus := strings.TrimSpace(parts[0])
		fileName := strings.TrimSpace(parts[len(parts)-1])
		if rawStatus == "" || fileName == "" {
			continue
		}

		fileInfo := FileEntry{
			Path:         fileName,
			StageStatus:  "commit",
			ChangeStatus: normalizeCommitStatus(rawStatus),
		}
		glv.commitFiles = append(glv.commitFiles, fileInfo)
	}
}

// normalizeCommitStatus converts git show status codes to descriptive labels
func normalizeCommitStatus(rawStatus string) string {
	if rawStatus == "" {
		return ""
	}

	switch rawStatus[0] {
	case 'A':
		return "added"
	case 'M':
		return "modified"
	case 'D':
		return "deleted"
	case 'R':
		return "renamed"
	case 'C':
		return "copied"
	case 'T':
		return "type-changed"
	default:
		return strings.ToLower(rawStatus)
	}
}

// showCommitDetails shows the details of the selected commit using shared components
func (glv *GitLogView) showCommitDetails() {
	if glv.currentLine >= len(glv.logEntries) {
		return
	}

	entry := glv.logEntries[glv.currentLine]
	if entry.Hash == "" {
		return
	}

	commitHash := entry.Hash
	glv.loadCommitFiles(commitHash)

	// Create UI components (same pattern as root_editor.go)
	fileListView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	fileListView.SetBorder(true).SetTitle("Changed Files")
	fileListView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	diffView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	diffView.SetBorder(true).SetTitle("File Diff")
	diffView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	diffView.SetBorderStyle(tcell.StyleDefault)

	unifiedViewFlex := tview.NewFlex().
		AddItem(diffView, 0, 1, false)
	unifiedViewFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	beforeView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	beforeView.SetBorder(true).SetTitle("Before")
	beforeView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	afterView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	afterView.SetBorder(true).SetTitle("After")
	afterView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	splitViewFlex := tview.NewFlex().
		AddItem(beforeView, 0, 1, false).
		AddItem(afterView, 0, 1, false)
	splitViewFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// Status bar
	statusView := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetWrap(false)
	statusView.SetBorder(true)
	statusView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	commitStatusMessage := "j/k:move  /:search  e:fold  s:split  V:select  C-y:copy  Esc:back  q:quit"
	statusView.SetText(commitStatusMessage)

	contentFlex := tview.NewFlex()
	contentFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	contentFlex.
		AddItem(fileListView, 0, FileListFlexRatio, true).
		AddItem(unifiedViewFlex, 0, DiffViewFlexRatio, false)

	// Vertical layout with status bar + content
	commitMainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(statusView, 3, 0, false).
		AddItem(contentFlex, 0, 1, true)
	commitMainFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	// State variables
	var fileList []FileEntry
	lineNumberMap := make(map[int]int)
	dirCollapseState := NewDirCollapseState()
	var currentSelection int
	var cursorY int
	var currentFile string
	var currentStatus string
	var currentDiffText string
	var isSplitView bool
	var leftPaneFocused = true
	var isSelecting bool
	var selectStart = -1
	var selectEnd = -1
	var preserveScrollRow = -1
	var ignoreWhitespace bool
	var fileFilterQuery string
	var fileFilterMode bool
	var gPressed bool
	var lastGTime time.Time
	var searchQuery string
	var searchMatches []int
	var searchMatchIndex = -1
	var isSearchMode bool
	var searchInput string
	var searchCursorYBeforeSearch int
	foldState := NewFoldState()

	// Diff retrieval callback (git show)
	updateDiffText := func(filePath, status, repo string, out *string, _ bool) {
		cmd := exec.Command("git", "show", "--format=", commitHash, "--", filePath)
		cmd.Dir = glv.repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			*out = ""
			return
		}
		*out = strings.TrimLeft(string(output), "\n")
	}

	// Build file list
	buildFileListContent := func(focusedPane bool) string {
		return BuildFileListContentForCommit(
			glv.commitFiles,
			currentSelection,
			focusedPane,
			&fileList,
			lineNumberMap,
			dirCollapseState,
		)
	}

	updateFileListView := func() {
		currentRow, currentCol := fileListView.GetScrollOffset()
		shouldPreserveScroll := preserveScrollRow >= 0
		if shouldPreserveScroll {
			currentRow = preserveScrollRow
			preserveScrollRow = -1
		}

		fileListView.Clear()
		fileListView.SetText(buildFileListContent(leftPaneFocused))

		if shouldPreserveScroll {
			fileListView.ScrollTo(currentRow, currentCol)
		} else {
			if actualLine, exists := lineNumberMap[currentSelection]; exists {
				_, _, _, height := fileListView.GetInnerRect()
				if actualLine >= currentRow+height-1 {
					fileListView.ScrollTo(actualLine-height+2, currentCol)
				} else if actualLine < currentRow {
					if currentSelection == 0 {
						fileListView.ScrollTo(0, currentCol)
					} else {
						fileListView.ScrollTo(actualLine, currentCol)
					}
				} else {
					fileListView.ScrollTo(currentRow, currentCol)
				}
			}
		}
	}

	updateSelectedFileDiff := func() {
		if len(fileList) == 0 {
			currentDiffText = ""
			currentFile = ""
			currentStatus = ""
			if isSplitView {
				beforeView.SetText("")
				afterView.SetText("No files changed")
			} else {
				diffView.SetText("No files changed")
			}
			return
		}

		if currentSelection < 0 {
			currentSelection = 0
		} else if currentSelection >= len(fileList) {
			currentSelection = len(fileList) - 1
		}

		fileEntry := fileList[currentSelection]

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

		currentFile = fileEntry.Path
		currentStatus = fileEntry.StageStatus

		cursorY = 0
		isSelecting = false
		selectStart = -1
		selectEnd = -1

		updateDiffText(currentFile, currentStatus, glv.repoRoot, &currentDiffText, ignoreWhitespace)

		if isSplitView {
			updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, currentFile)
		} else {
			updateDiffViewWithoutCursor(diffView, currentDiffText, foldState, currentFile, glv.repoRoot)
		}
	}

	updateFileListView()

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

	updateSelectedFileDiff()

	// Build DiffViewContext
	diffViewContext := &DiffViewContext{
		diffView:        diffView,
		fileListView:    fileListView,
		beforeView:      beforeView,
		afterView:       afterView,
		splitViewFlex:   splitViewFlex,
		unifiedViewFlex: unifiedViewFlex,
		contentFlex:     contentFlex,
		app:             glv.app,

		cursorY:               &cursorY,
		selectStart:           &selectStart,
		selectEnd:             &selectEnd,
		isSelecting:           &isSelecting,
		isSplitView:           &isSplitView,
		leftPaneFocused:       &leftPaneFocused,
		currentDiffText:       &currentDiffText,
		currentFile:           &currentFile,
		currentStatus:         &currentStatus,
		savedTargetFile:       new(string),
		preferUnstagedSection: new(bool),
		currentSelection:      &currentSelection,
		fileList:              &fileList,
		preserveScrollRow:     &preserveScrollRow,
		ignoreWhitespace:      &ignoreWhitespace,

		repoRoot:  glv.repoRoot,
		patchPath: "",

		gPressed:  &gPressed,
		lastGTime: &lastGTime,

		viewUpdater: &UnifiedViewUpdater{
			diffView:    diffView,
			foldState:   foldState,
			filePath:    &currentFile,
			repoRoot:    glv.repoRoot,
			searchQuery: &searchQuery,
		},

		foldState: foldState,

		searchQuery:               &searchQuery,
		searchMatches:             &searchMatches,
		searchMatchIndex:          &searchMatchIndex,
		isSearchMode:              &isSearchMode,
		searchInput:               &searchInput,
		searchCursorYBeforeSearch: &searchCursorYBeforeSearch,

		readOnly: true,

		updateFileListView: updateFileListView,
		updateGlobalStatus: func(message string, color string) {
			statusView.SetText(fmt.Sprintf("[%s]%s[-]", color, message))
			go func() {
				time.Sleep(5 * time.Second)
				statusView.SetText(commitStatusMessage)
			}()
		},
		setGlobalStatusText: func(text string) {
			statusView.SetText(text)
		},
		refreshFileList:       func() {},
		onUpdate:              nil,
		updateCurrentDiffText: updateDiffText,
		updateStatusTitle:     func() {},
		onEsc:                 glv.backToLog,
	}
	SetupDiffViewKeyBindings(diffViewContext)

	// Build FileListKeyContext
	fileListKeyContext := &FileListKeyContext{
		fileListView:    fileListView,
		diffView:        diffView,
		beforeView:      beforeView,
		afterView:       afterView,
		splitViewFlex:   splitViewFlex,
		unifiedViewFlex: unifiedViewFlex,
		contentFlex:     contentFlex,
		app:             glv.app,
		mainView:        glv.flex,

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

		fileList:         &fileList,
		dirCollapseState: dirCollapseState,
		repoRoot:         glv.repoRoot,
		diffViewContext:  diffViewContext,

		readOnly: true,

		updateFileListView:     updateFileListView,
		updateSelectedFileDiff: updateSelectedFileDiff,
		refreshFileList:        func() {},
		updateCurrentDiffText:  updateDiffText,
		updateGlobalStatus: func(message string, color string) {
			statusView.SetText(fmt.Sprintf("[%s]%s[-]", color, message))
			go func() {
				time.Sleep(5 * time.Second)
				statusView.SetText(commitStatusMessage)
			}()
		},
		updateStatusTitle: func() {},
		setGlobalStatusText: func(text string) {
			statusView.SetText(text)
		},
		onEsc: glv.backToLog,
	}
	SetupFileListKeyBindings(fileListKeyContext)

	// Switch display
	glv.flex.Clear()
	glv.flex.AddItem(commitMainFlex, 0, 1, true)
	glv.showingCommit = true
	glv.app.SetFocus(fileListView)
}

// backToLog returns to the log view from commit details
func (glv *GitLogView) backToLog() {
	glv.flex.Clear()
	glv.flex.AddItem(glv.logView, 0, 1, true)
	glv.showingCommit = false
	glv.app.SetFocus(glv.logView)
}

// setupLogKeyBindings configures keyboard navigation for the log list
func (glv *GitLogView) setupLogKeyBindings() {
	glv.logView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			if glv.onExit != nil {
				glv.onExit()
			}
			return nil
		case tcell.KeyEnter:
			glv.showCommitDetails()
			return nil
		}

		switch event.Rune() {
		case 'q':
			glv.quitApplication()
			return nil
		case 'j':
			if glv.currentLine < len(glv.logEntries)-1 {
				glv.currentLine++
				glv.updateSelection()
			}
			return nil
		case 'k':
			if glv.currentLine > 0 {
				glv.currentLine--
				glv.updateSelection()
			}
			return nil
		case 'g':
			now := time.Now()
			if *glv.gPressed && now.Sub(*glv.lastGTime) < 500*time.Millisecond {
				glv.currentLine = 0
				glv.scrollOffset = 0
				glv.updateSelection()
				*glv.gPressed = false
			} else {
				*glv.gPressed = true
				*glv.lastGTime = now
			}
			return nil
		case 'G':
			if len(glv.logEntries) > 0 {
				glv.currentLine = len(glv.logEntries) - 1
				glv.updateSelection()
			}
			return nil
		}

		return event
	})
}

func (glv *GitLogView) quitApplication() {
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(0)
	}()
	glv.app.Stop()
}
