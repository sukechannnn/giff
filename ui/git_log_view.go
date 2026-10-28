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
	"github.com/sukechannnn/giff/git"
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
	logView.SetBorderColor(util.CommitAreaBorderColor.ToTcellColor())
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
			content.WriteString("[" + string(util.MainTextColor) + ":" + string(util.CursorLineColor) + "]" + entry.FullLine + "[-:-]\n")
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
	glv.commitFiles = parseNameStatus(string(output))
}

// parseNameStatus turns `git show/diff --name-status` output into file entries
func parseNameStatus(output string) []FileEntry {
	files := []FileEntry{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
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
		files = append(files, fileInfo)
	}
	return files
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
	showReadOnlyDiff(glv.app, glv.flex, glv.repoRoot, "Changed Files", glv.commitFiles, func(opts []string, paths ...string) []string {
		args := append([]string{"show", "--format="}, opts...)
		args = append(args, commitHash, "--")
		return append(args, paths...)
	}, nil, glv.backToLog)
	glv.showingCommit = true
}

// showReadOnlyDiff fills host with a file list and diff view for browsing
// files without staging. diffArgs returns the git arguments that print the
// diff of paths, or of every file when there are none, with opts such as "-w"
// added as diff options. reload re-reads the file list of a diff that can
// change, such as the pull request diff when commits land, and is called by
// 'r'. It is nil for a fixed diff. onEsc leaves the view.
func showReadOnlyDiff(app *tview.Application, host *tview.Flex, repoRoot string, title string, files []FileEntry, diffArgs func(opts []string, paths ...string) []string, reload func() ([]FileEntry, error), onEsc func()) {
	// Create UI components (same pattern as root_editor.go)
	fileListView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	borderColor := util.CommitAreaBorderColor.ToTcellColor()

	fileListView.SetBorder(true).SetTitle(title)
	fileListView.SetBorderColor(borderColor)
	fileListView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	diffView := tview.NewTextView().
		SetDynamicColors(true).
		SetRegions(true).
		SetWrap(false)
	diffView.SetBorder(true).SetTitle("File Diff")
	diffView.SetBorderColor(borderColor)
	diffView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	unifiedViewFlex := tview.NewFlex().
		AddItem(diffView, 0, 1, false)
	unifiedViewFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

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
	statusView.SetBorderColor(borderColor)
	statusView.SetBackgroundColor(util.BackgroundColor.ToTcellColor())

	commitStatusMessage := "j/k:move  /:grep  !:exclude  e:fold  s:split  w:ws  V:select  c/v:editor  C-y:copy  t:terminal  Esc:back  q:quit"
	if reload != nil {
		commitStatusMessage = strings.Replace(commitStatusMessage, "  Esc:back", "  r:reload  Esc:back", 1)
	}
	statusView.SetText(commitStatusMessage)

	contentFlex := tview.NewFlex()
	contentFlex.SetBackgroundColor(util.BackgroundColor.ToTcellColor())
	contentFlex.
		AddItem(fileListView, 0, FileListFlexRatio, true).
		AddItem(unifiedViewFlex, 0, DiffViewFlexRatio, false)

	// '+' and '-' move the border between the file list and the diff
	fileListRatio := FileListFlexRatio
	diffViewRatio := DiffViewFlexRatio

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
	var fileExcludeQuery string
	var fileFilterMode bool
	var diffGrepQuery string
	var diffGrepHits map[string]int // per-path hit counts of the '/' grep
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
	updateDiffText := func(filePath, status, repo string, out *string, ignoreWhitespace bool) {
		var opts []string
		if ignoreWhitespace {
			opts = append(opts, "-w")
		}
		cmd := exec.Command("git", diffArgs(opts, filePath)...)
		cmd.Dir = repoRoot
		output, err := cmd.CombinedOutput()
		if err != nil {
			*out = ""
			return
		}
		*out = strings.TrimLeft(string(output), "\n")
		if util.IsBinaryDiff(*out) {
			*out = util.FormatBinaryNotice(filePath)
		}
	}

	// shownFiles narrows files by the Ctrl+F filter, the '!' exclusion and the
	// '/' grep
	shownFiles := func() []FileEntry {
		if fileFilterQuery == "" && fileExcludeQuery == "" && diffGrepQuery == "" {
			return files
		}
		var shown []FileEntry
		for _, f := range files {
			if diffGrepQuery != "" && diffGrepHits[f.Path] == 0 {
				continue
			}
			if fileExcludeQuery != "" && matchesFilter(f, fileExcludeQuery) {
				continue
			}
			if fileFilterQuery != "" && !matchesFilter(f, fileFilterQuery) {
				continue
			}
			shown = append(shown, f)
		}
		return shown
	}

	// restoreStatus shows what is narrowing the list, or the key help
	restoreStatus := func() {
		var parts []string
		if diffGrepQuery != "" {
			shown := shownFiles()
			hits := 0
			for _, f := range shown {
				hits += diffGrepHits[f.Path]
			}
			if len(shown) > 0 {
				parts = append(parts, fmt.Sprintf("[white]%s%s [%d files, %d hits][-]",
					diffGrepPrompt, tview.Escape(diffGrepQuery), len(shown), hits))
			} else {
				parts = append(parts, fmt.Sprintf("[tomato]%s%s [no match][-]",
					diffGrepPrompt, tview.Escape(diffGrepQuery)))
			}
		}
		if fileExcludeQuery != "" {
			hidden := 0
			for _, f := range files {
				if matchesFilter(f, fileExcludeQuery) {
					hidden++
				}
			}
			parts = append(parts, fmt.Sprintf("[white]%s%s [%d hidden][-]",
				fileExcludePrompt, tview.Escape(fileExcludeQuery), hidden))
		}
		if fileFilterQuery != "" {
			if matched := len(shownFiles()); matched > 0 {
				parts = append(parts, fmt.Sprintf("[white]%s%s [%d matched][-]",
					fileFilterPrompt, tview.Escape(fileFilterQuery), matched))
			} else {
				parts = append(parts, fmt.Sprintf("[tomato]%s%s [no match][-]",
					fileFilterPrompt, tview.Escape(fileFilterQuery)))
			}
		}
		if len(parts) == 0 {
			statusView.SetText(commitStatusMessage)
			return
		}
		if diffGrepQuery != "" {
			parts = append(parts, "[white]n/N:hit[-]")
		}
		if leftPaneFocused {
			// Esc clears the narrowings only from the file list.
			parts = append(parts, "[white]Esc:clear[-]")
		}
		statusView.SetText(strings.Join(parts, "  "))
	}

	updateStatusTitle := func() {
		if ignoreWhitespace {
			statusView.SetTitle(" Hide whitespace: on ")
			statusView.SetTitleAlign(tview.AlignLeft)
		} else {
			statusView.SetTitle("")
		}
	}

	// runDiffGrep greps the changed lines of every file at once and remembers
	// the hits, so the file list can be narrowed to the matching files.
	runDiffGrep := func(query string) error {
		opts := []string{"--no-ext-diff", "-U0"}
		if ignoreWhitespace {
			opts = append(opts, "-w")
		}
		cmd := exec.Command("git", append(git.GrepDiffConfig(), diffArgs(opts)...)...)
		cmd.Dir = repoRoot
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		diffGrepQuery = query
		diffGrepHits = git.GrepDiffText(string(output), query)
		return nil
	}

	// applyGrepSearchToDiff installs the active grep as the diff view's search
	// query, so its matches stay highlighted and n/N can walk them.
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

	resizeFileList := func(delta int) {
		newFileRatio := fileListRatio + delta
		newDiffRatio := diffViewRatio - delta
		if newFileRatio < 1 || newDiffRatio < 1 {
			return
		}
		fileListRatio = newFileRatio
		diffViewRatio = newDiffRatio
		rebuildContentFlex()
	}

	updateGlobalStatus := func(message string, color string) {
		statusView.SetText(fmt.Sprintf("[%s]%s[-]", color, message))
		go func() {
			time.Sleep(5 * time.Second)
			app.QueueUpdateDraw(restoreStatus)
		}()
	}

	// The terminal ('t') is made on first use. What it shows is fixed, so
	// there is nothing to refresh after a command.
	var terminal *terminalPane
	openTerminal := func() {
		if terminal == nil {
			terminal = newTerminalPane(app, repoRoot)
		}
		var focusBefore tview.Primitive = fileListView
		if !leftPaneFocused {
			if isSplitView {
				focusBefore = splitViewFlex
			} else {
				focusBefore = diffView
			}
		}
		terminal.Open(commitMainFlex, focusBefore, nil)
	}

	// Build file list
	buildFileListContent := func(focusedPane bool) string {
		return BuildFileListContentForCommit(
			shownFiles(),
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

		updateDiffText(currentFile, currentStatus, repoRoot, &currentDiffText, ignoreWhitespace)
		applyGrepSearchToDiff()

		if isSplitView && diffGrepQuery != "" {
			// Show where the grep matched, scrolling to the first match.
			updateSplitViewHighlighted(beforeView, afterView, currentDiffText, cursorY, foldState, currentFile, repoRoot, searchQuery)
		} else if isSplitView {
			updateSplitViewWithoutCursor(beforeView, afterView, currentDiffText, foldState, currentFile, repoRoot)
		} else if diffGrepQuery != "" {
			updateDiffViewHighlighted(diffView, currentDiffText, cursorY, foldState, currentFile, repoRoot, searchQuery)
		} else {
			updateDiffViewWithoutCursor(diffView, currentDiffText, foldState, currentFile, repoRoot)
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
		app:             app,

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

		repoRoot:  repoRoot,
		patchPath: "",

		gPressed:  &gPressed,
		lastGTime: &lastGTime,

		viewUpdater: &UnifiedViewUpdater{
			diffView:    diffView,
			foldState:   foldState,
			filePath:    &currentFile,
			repoRoot:    repoRoot,
			searchQuery: &searchQuery,
		},

		foldState: foldState,

		diffGrepQuery: &diffGrepQuery,

		searchQuery:               &searchQuery,
		searchMatches:             &searchMatches,
		searchMatchIndex:          &searchMatchIndex,
		isSearchMode:              &isSearchMode,
		searchInput:               &searchInput,
		searchCursorYBeforeSearch: &searchCursorYBeforeSearch,

		readOnly: true,

		updateFileListView: updateFileListView,
		updateGlobalStatus: updateGlobalStatus,
		setGlobalStatusText: func(text string) {
			statusView.SetText(text)
		},
		refreshFileList:       func() {},
		onUpdate:              nil,
		updateCurrentDiffText: updateDiffText,
		updateStatusTitle:     updateStatusTitle,
		onEsc:                 onEsc,
		openTerminal:          openTerminal,
		restoreStatus:         restoreStatus,
		resizeFileList:        resizeFileList,
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
		app:             app,
		mainView:        host,

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

		fileList:         &fileList,
		dirCollapseState: dirCollapseState,
		repoRoot:         repoRoot,
		diffViewContext:  diffViewContext,

		readOnly: true,

		updateFileListView:     updateFileListView,
		updateSelectedFileDiff: updateSelectedFileDiff,
		refreshFileList:        func() {},
		updateCurrentDiffText:  updateDiffText,
		updateGlobalStatus:     updateGlobalStatus,
		updateStatusTitle:      updateStatusTitle,
		setGlobalStatusText: func(text string) {
			statusView.SetText(text)
		},
		onEsc:          onEsc,
		openTerminal:   openTerminal,
		restoreStatus:  restoreStatus,
		resizeFileList: resizeFileList,
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

	if reload != nil {
		setupReadOnlyReload(readOnlyReloadState{
			app:          app,
			reload:       reload,
			files:        &files,
			fileList:     &fileList,
			selection:    &currentSelection,
			currentFile:  &currentFile,
			diffText:     &currentDiffText,
			cursorY:      &cursorY,
			isSelecting:  &isSelecting,
			selectStart:  &selectStart,
			selectEnd:    &selectEnd,
			leftFocused:  &leftPaneFocused,
			fileListView: fileListView,
			diffContext:  diffViewContext,
			fetchDiff: func(filePath string) string {
				var text string
				updateDiffText(filePath, currentStatus, repoRoot, &text, ignoreWhitespace)
				return text
			},
			regrep: func() {
				if diffGrepQuery != "" && runDiffGrep(diffGrepQuery) != nil {
					diffGrepHits = nil
				}
			},
			updateFileListView:     updateFileListView,
			updateSelectedFileDiff: updateSelectedFileDiff,
			updateGlobalStatus:     updateGlobalStatus,
			restoreStatus:          restoreStatus,
			fileListContext:        fileListKeyContext,
		})
	}

	// Switch display
	host.Clear()
	host.AddItem(commitMainFlex, 0, 1, true)
	app.SetFocus(fileListView)
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
		case 'd':
			// Return to diff mode
			if glv.onExit != nil {
				glv.onExit()
			}
			return nil
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
