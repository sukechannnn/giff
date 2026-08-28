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
	"github.com/sukechannnn/giff/ui/commands"
)

// moveFileListSelection moves the file list selection linearly through all visible entries
// (both files and directories). moveFileListToFile moves only between files.
// matchesFilter checks if a file entry matches the current filter query
// expandBraces expands a glob pattern with {a,b,c} into multiple patterns.
// e.g. "*.{ts,tsx}" -> ["*.ts", "*.tsx"]
func expandBraces(pattern string) []string {
	start := strings.Index(pattern, "{")
	if start < 0 {
		return []string{pattern}
	}
	end := strings.Index(pattern[start:], "}")
	if end < 0 {
		return []string{pattern}
	}
	end += start

	prefix := pattern[:start]
	suffix := pattern[end+1:]
	alternatives := strings.Split(pattern[start+1:end], ",")

	var results []string
	for _, alt := range alternatives {
		expanded := expandBraces(prefix + strings.TrimSpace(alt) + suffix)
		results = append(results, expanded...)
	}
	return results
}

// matchGlob matches a file path against a glob pattern supporting ** for any path segments.
func matchGlob(pattern, path string) bool {
	pattern = strings.ToLower(pattern)
	path = strings.ToLower(path)

	if strings.Contains(pattern, "**") {
		// Split on ** and match each part
		parts := strings.SplitN(pattern, "**", 2)
		before := parts[0]
		after := strings.TrimPrefix(parts[1], "/")

		// Before must match the prefix (or be empty)
		if before != "" {
			before = strings.TrimSuffix(before, "/")
			if !strings.HasPrefix(path, before+"/") && path != before {
				return false
			}
		}

		if after == "" {
			return true
		}

		// Try matching "after" pattern against every possible suffix
		segments := strings.Split(path, "/")
		for i := range segments {
			candidate := strings.Join(segments[i:], "/")
			if matched, _ := filepath.Match(after, candidate); matched {
				return true
			}
			// Also try matching just the filename
			if i == len(segments)-1 {
				if matched, _ := filepath.Match(after, segments[i]); matched {
					return true
				}
			}
		}
		return false
	}

	// No **, try direct match or substring
	if matched, _ := filepath.Match(pattern, path); matched {
		return true
	}
	// Also try matching against just the filename
	base := filepath.Base(path)
	if matched, _ := filepath.Match(pattern, base); matched {
		return true
	}
	return false
}

func matchesFilter(entry FileEntry, filterQuery string) bool {
	if filterQuery == "" {
		return true
	}
	if entry.IsDirectory {
		return false
	}

	// Check if it looks like a glob pattern
	if strings.ContainsAny(filterQuery, "*?{[") {
		patterns := expandBraces(filterQuery)
		for _, p := range patterns {
			if matchGlob(p, entry.Path) {
				return true
			}
		}
		return false
	}

	// Plain substring match
	return strings.Contains(strings.ToLower(entry.Path), strings.ToLower(filterQuery))
}

func moveFileListSelection(ctx *FileListKeyContext, direction int) {
	if *ctx.currentSelection < 0 || *ctx.currentSelection >= len(*ctx.fileList) {
		return
	}

	next := *ctx.currentSelection + direction
	for next >= 0 && next < len(*ctx.fileList) {
		entry := (*ctx.fileList)[next]
		// Skip files that don't match the filter. Directories are already filtered at build time.
		if *ctx.filterQuery != "" && !entry.IsDirectory && !matchesFilter(entry, *ctx.filterQuery) {
			next += direction
			continue
		}
		*ctx.currentSelection = next
		ctx.updateFileListView()
		if !entry.IsDirectory {
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

	if direction < 0 {
		ctx.fileListView.ScrollTo(0, 0)
	}
}

// jumpFileListSelection moves the selection to the first file (toTop=true)
// or the last file (toTop=false), skipping directories and entries that do
// not match the active filter.
func jumpFileListSelection(ctx *FileListKeyContext, toTop bool) {
	if len(*ctx.fileList) == 0 {
		return
	}
	step := 1
	start := 0
	if !toTop {
		step = -1
		start = len(*ctx.fileList) - 1
	}
	for i := start; i >= 0 && i < len(*ctx.fileList); i += step {
		entry := (*ctx.fileList)[i]
		if entry.IsDirectory {
			continue
		}
		if *ctx.filterQuery != "" && !matchesFilter(entry, *ctx.filterQuery) {
			continue
		}
		*ctx.currentSelection = i
		ctx.updateFileListView()
		ctx.updateSelectedFileDiff()
		return
	}
}

// moveFileListToFile moves the selection to the next/previous file, skipping directories.
// If the current entry is a directory, this jumps to the nearest file in the given direction
// (which is the directory's first child when it is expanded).
func moveFileListToFile(ctx *FileListKeyContext, direction int) {
	if *ctx.currentSelection < 0 || *ctx.currentSelection >= len(*ctx.fileList) {
		return
	}

	next := *ctx.currentSelection + direction
	for next >= 0 && next < len(*ctx.fileList) {
		entry := (*ctx.fileList)[next]
		if entry.IsDirectory {
			next += direction
			continue
		}
		if *ctx.filterQuery != "" && !matchesFilter(entry, *ctx.filterQuery) {
			next += direction
			continue
		}
		*ctx.currentSelection = next
		ctx.updateFileListView()
		if ctx.diffDebounceTimer != nil {
			ctx.diffDebounceTimer.Stop()
		}
		ctx.diffDebounceTimer = time.AfterFunc(80*time.Millisecond, func() {
			ctx.app.QueueUpdateDraw(func() {
				ctx.updateSelectedFileDiff()
			})
		})
		return
	}
}

// Prompts marking which of the two file list input modes is running.
const (
	diffGrepPrompt   = "/"
	fileFilterPrompt = "filter:"
)

// filterInputStatusText renders the filter input shown in the status bar with
// a block cursor at the given rune position so users can see where editing
// applies. If the cursor is at the end, a trailing reverse-video space is
// appended; otherwise the character under the cursor is highlighted.
func filterInputStatusText(prompt, input string, cursor int) string {
	runes := []rune(input)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	if cursor == len(runes) {
		return fmt.Sprintf("[white]%s%s[::r] [::-]", prompt, tview.Escape(input))
	}
	before := tview.Escape(string(runes[:cursor]))
	onCursor := tview.Escape(string(runes[cursor : cursor+1]))
	after := tview.Escape(string(runes[cursor+1:]))
	return fmt.Sprintf("[white]%s%s[::r]%s[::-]%s", prompt, before, onCursor, after)
}

// applyUndoRedoInFileList applies the given undo/redo action and refreshes the
// file list and selected diff to reflect the restored index state.
func applyUndoRedoInFileList(ctx *FileListKeyContext, action func() (string, error), verb string) {
	if ctx.undoStack == nil {
		return
	}
	desc, err := action()
	if err != nil {
		if ctx.updateGlobalStatus != nil {
			ctx.updateGlobalStatus(fmt.Sprintf("Nothing to %s", verb), "yellow")
		}
		return
	}

	// Remember current selection by path so we can restore it after refresh
	var selectedPath, selectedStatus string
	selectedIsDir := false
	if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
		entry := (*ctx.fileList)[*ctx.currentSelection]
		selectedPath = entry.Path
		selectedStatus = entry.StageStatus
		selectedIsDir = entry.IsDirectory
	}

	ctx.refreshFileList()
	ctx.updateFileListView()

	if selectedPath != "" {
		for i, fe := range *ctx.fileList {
			if fe.Path == selectedPath && fe.IsDirectory == selectedIsDir && fe.StageStatus == selectedStatus {
				*ctx.currentSelection = i
				break
			}
		}
		if *ctx.currentSelection >= len(*ctx.fileList) {
			*ctx.currentSelection = len(*ctx.fileList) - 1
		}
		if *ctx.currentSelection < 0 {
			*ctx.currentSelection = 0
		}
		ctx.updateFileListView()
	}
	ctx.updateSelectedFileDiff()

	if ctx.updateGlobalStatus != nil {
		label := "Undone"
		if verb == "redo" {
			label = "Redone"
		}
		ctx.updateGlobalStatus(fmt.Sprintf("%s: %s", label, desc), "forestgreen")
	}
}

// stageStatusGroup maps a StageStatus to its display section group so that
// untracked entries (which now share a section with unstaged entries) match
// against unstaged directory entries for parent/collapse lookups.
func stageStatusGroup(stage string) string {
	if stage == "untracked" {
		return "unstaged"
	}
	return stage
}

// findParentDirectory finds the parent directory entry for the given entry
func findParentDirectory(fileList *[]FileEntry, currentIdx int) int {
	entry := (*fileList)[currentIdx]
	entryGroup := stageStatusGroup(entry.StageStatus)
	for i := currentIdx - 1; i >= 0; i-- {
		candidate := (*fileList)[i]
		if candidate.IsDirectory && stageStatusGroup(candidate.StageStatus) == entryGroup &&
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
	hitCounts map[string]int,
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
		nil,
		hitCounts,
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
	grepHits map[string]int,
) string {
	// Rebuild fileList
	// Clear slice contents (keep the reference)
	*fileList = (*fileList)[:0]
	for k := range lineNumberMap {
		delete(lineNumberMap, k)
	}

	// Narrow by file name (glob patterns supported) and, when a diff grep is
	// active (grepHits is non-nil), by whether the file has any matching line.
	filterFn := func(files []git.FileInfo, stageStatus string) []git.FileInfo {
		if filterQuery == "" && grepHits == nil {
			return files
		}
		var filtered []git.FileInfo
		for _, f := range files {
			if filterQuery != "" && !matchesFilter(FileEntry{Path: f.Path}, filterQuery) {
				continue
			}
			if grepHits != nil {
				if _, ok := grepHits[git.DiffGrepKey(stageStatus, f.Path)]; !ok {
					continue
				}
			}
			filtered = append(filtered, f)
		}
		return filtered
	}
	filteredStaged := filterFn(stagedFiles, git.GrepStageStaged)
	filteredModified := filterFn(modifiedFiles, git.GrepStageUnstaged)
	filteredUntracked := filterFn(untrackedFiles, git.GrepStageUntracked)

	var coloredContent strings.Builder
	regionIndex := 0
	currentLine := 0

	// Staged files
	if len(filteredStaged) > 0 {
		coloredContent.WriteString("[green]Changes to be committed:[white]\n")
		currentLine++
		tree := buildFileTree(filteredStaged)
		renderFileTree(tree, 1, &coloredContent, fileList,
			"staged", &regionIndex, currentSelection, focusedPane, lineNumberMap, &currentLine, filteredStaged, collapseState, grepHits)
		coloredContent.WriteString("\n")
		currentLine++
	}

	// Unstaged files (modified + untracked rendered as a single tree)
	if len(filteredModified) > 0 || len(filteredUntracked) > 0 {
		coloredContent.WriteString("[yellow]Changes not staged for commit:[white]\n")
		currentLine++

		combined := make([]git.FileInfo, 0, len(filteredModified)+len(filteredUntracked))
		combined = append(combined, filteredModified...)
		combined = append(combined, filteredUntracked...)

		statusMap := make(map[string]string, len(combined))
		for _, fi := range combined {
			statusMap[fi.Path] = fi.ChangeStatus
		}

		// Override per-file StageStatus so untracked files keep "untracked"
		// semantics for behavior (diff rendering, discard action) while sharing
		// the same section/tree as unstaged files.
		stageOverride := make(map[string]string, len(filteredUntracked))
		for _, fi := range filteredUntracked {
			stageOverride[fi.Path] = "untracked"
		}

		tree := buildFileTree(combined)
		renderFileTreeForGitFiles(tree, 1, "", &coloredContent, fileList,
			"unstaged", &regionIndex, currentSelection, focusedPane,
			lineNumberMap, &currentLine, combined, collapseState, statusMap, stageOverride, grepHits)
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

// BuildFileListContentForBrowser builds file list content for file browser mode.
// Uses git.FileInfo list (all tracked files) and renders with tree structure.
func BuildFileListContentForBrowser(
	allFiles []git.FileInfo,
	currentSelection int,
	focusedPane bool,
	fileList *[]FileEntry,
	lineNumberMap map[int]int,
	collapseState *DirCollapseState,
	filterQuery string,
) string {
	*fileList = (*fileList)[:0]
	for k := range lineNumberMap {
		delete(lineNumberMap, k)
	}

	// Apply filter (supports glob patterns)
	var filtered []git.FileInfo
	if filterQuery != "" {
		for _, f := range allFiles {
			entry := FileEntry{Path: f.Path}
			if matchesFilter(entry, filterQuery) {
				filtered = append(filtered, f)
			}
		}
	} else {
		filtered = allFiles
	}

	tree := buildFileTree(filtered)
	statusMap := make(map[string]string, len(filtered))
	for _, fi := range filtered {
		statusMap[fi.Path] = fi.ChangeStatus
	}

	var content strings.Builder
	regionIndex := 0
	currentLine := 0

	renderFileTreeForGitFiles(
		tree, 0, "", &content, fileList,
		"browser", &regionIndex, currentSelection, focusedPane,
		lineNumberMap, &currentLine, filtered, collapseState, statusMap, nil, nil,
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

	// Undo stack for staging operations (nil in read-only mode)
	undoStack *UndoStack

	// Debounce timer for diff updates
	diffDebounceTimer *time.Timer

	// Mode
	readOnly bool // if true, disable staging/discard operations

	// File filter state
	isFilterMode *bool
	filterInput  string
	filterCursor int     // rune-index cursor position inside filterInput
	filterQuery  *string // active file name filter (empty = no filter)

	// Diff content grep state ('/' in the file list). runDiffGrep is nil in
	// views without pending diffs (git log), where '/' filters by file name.
	isGrepInput   bool            // the input line in progress greps diffs
	diffGrepQuery *string         // active grep query (empty = no grep)
	diffGrepHits  *map[string]int // per-file hit counts, nil while inactive
	runDiffGrep   func(query string) error
	// diffGrepStatusText renders the running grep for the status bar.
	diffGrepStatusText func() string

	// Key handling state for gg chord
	gPressed  *bool
	lastGTime *time.Time

	// Callbacks
	updateFileListView     func()
	updateSelectedFileDiff func()
	refreshFileList        func()
	updateCurrentDiffText  func(string, string, string, *string, bool)
	updateGlobalStatus     func(string, string)
	updateStatusTitle      func()
	setGlobalStatusText    func(string)
	onEsc                  func()          // if non-nil, called on Esc key
	openTerminal           func()          // if non-nil, opens terminal command input
	toggleFileBrowser      func()          // if non-nil, toggles file browser mode
	isFileBrowserMode      *bool           // pointer to file browser mode flag
	resizeFileList         func(delta int) // resize file list width
}

// filterPrompt returns the status bar prefix of the input mode in progress.
func filterPrompt(ctx *FileListKeyContext) string {
	if ctx.isGrepInput {
		return diffGrepPrompt
	}
	return fileFilterPrompt
}

// startFilterInput opens the status bar input line. grep=true searches the
// contents of the pending diffs, grep=false filters by file name.
func startFilterInput(ctx *FileListKeyContext, grep bool) {
	*ctx.isFilterMode = true
	ctx.isGrepInput = grep
	ctx.filterInput = ""
	ctx.filterCursor = 0
	ctx.updateFileListView() // Redraw without cursor highlight
	if ctx.setGlobalStatusText != nil {
		ctx.setGlobalStatusText(filterInputStatusText(filterPrompt(ctx), ctx.filterInput, ctx.filterCursor))
	}
}

// grepCursorLineNumber returns the file line number the diff pane's cursor sits
// on while a grep narrows the list, so an editor can be opened at the hit
// instead of at the top of the file. It returns 0 when no grep is running, in
// which case the diff pane has no meaningful cursor of its own yet.
func grepCursorLineNumber(ctx *FileListKeyContext) int {
	if ctx.diffViewContext == nil || !diffGrepActive(ctx) {
		return 0
	}
	return getCursorFileLineNumber(ctx.diffViewContext)
}

// diffGrepActive reports whether a diff content grep is narrowing the list.
func diffGrepActive(ctx *FileListKeyContext) bool {
	return ctx.diffGrepQuery != nil && *ctx.diffGrepQuery != ""
}

// clearDiffGrep drops the active grep along with the match highlighting it
// installed in the diff view.
func clearDiffGrep(ctx *FileListKeyContext) {
	if ctx.diffGrepQuery == nil {
		return
	}
	*ctx.diffGrepQuery = ""
	if ctx.diffGrepHits != nil {
		*ctx.diffGrepHits = nil
	}
	if ctx.diffViewContext != nil {
		*ctx.diffViewContext.searchQuery = ""
		*ctx.diffViewContext.searchMatches = nil
		*ctx.diffViewContext.searchMatchIndex = -1
	}
}

// applyDiffGrep greps the changed lines of every pending diff and narrows the
// file list to the files that match, annotating each with its hit count.
func applyDiffGrep(ctx *FileListKeyContext) {
	if ctx.runDiffGrep == nil {
		return
	}

	if ctx.filterInput == "" {
		clearDiffGrep(ctx)
		ctx.updateFileListView()
		ctx.updateSelectedFileDiff()
		if ctx.setGlobalStatusText != nil {
			ctx.setGlobalStatusText(fileListKeyMessage)
		}
		return
	}

	if err := ctx.runDiffGrep(ctx.filterInput); err != nil {
		clearDiffGrep(ctx)
		ctx.updateFileListView()
		if ctx.updateGlobalStatus != nil {
			ctx.updateGlobalStatus("Failed to grep diffs: "+err.Error(), "tomato")
		}
		return
	}

	ctx.updateFileListView()

	// Expand every directory so all surviving files are visible.
	if ctx.dirCollapseState != nil {
		for _, entry := range *ctx.fileList {
			if entry.IsDirectory {
				ctx.dirCollapseState.SetCollapsed(entry.StageStatus, entry.Path, false)
			}
		}
		ctx.updateFileListView()
	}

	hasFile := false
	for _, entry := range *ctx.fileList {
		if !entry.IsDirectory {
			hasFile = true
			break
		}
	}
	if hasFile {
		// Selects the first surviving file and shows its diff, which installs
		// the grep query as the diff view's search query.
		jumpFileListSelection(ctx, true)
	} else {
		// Nothing matched: no file to select, but the diff pane still needs to
		// stop showing whichever file was there before.
		*ctx.currentSelection = 0
		ctx.updateSelectedFileDiff()
	}

	// The query stays in the status bar from here until Esc clears the grep.
	if ctx.setGlobalStatusText != nil && ctx.diffGrepStatusText != nil {
		ctx.setGlobalStatusText(ctx.diffGrepStatusText())
	}
}

// applyFileFilter updates the file list selection to match the filter query
func applyFileFilter(ctx *FileListKeyContext) {
	if ctx.filterInput == "" {
		// Clear filter: reset to show all and select first file
		*ctx.filterQuery = ""
		ctx.updateFileListView()
		if diffGrepActive(ctx) && restoreStatusFunc != nil {
			restoreStatusFunc()
		} else if ctx.setGlobalStatusText != nil {
			ctx.setGlobalStatusText("[white]" + fileFilterPrompt + "[-]")
		}
		return
	}
	*ctx.filterQuery = ctx.filterInput

	// Rebuild file list with filter applied
	ctx.updateFileListView()

	// Expand all directories in the filtered result
	if ctx.dirCollapseState != nil {
		for _, entry := range *ctx.fileList {
			if entry.IsDirectory {
				ctx.dirCollapseState.SetCollapsed(entry.StageStatus, entry.Path, false)
			}
		}
		// Rebuild again with directories expanded
		ctx.updateFileListView()
	}

	// Find first matching file and count matches
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
			ctx.setGlobalStatusText(fmt.Sprintf("[white]%s%s [%d matched][-]", fileFilterPrompt, tview.Escape(ctx.filterInput), matched))
		} else {
			ctx.setGlobalStatusText(fmt.Sprintf("[tomato]%s%s [no match][-]", fileFilterPrompt, tview.Escape(ctx.filterInput)))
		}
	}
}

// SetupFileListKeyBindings sets up key bindings for file list view
func SetupFileListKeyBindings(ctx *FileListKeyContext) {
	ctx.fileListView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Filter mode input handling
		if *ctx.isFilterMode {
			runes := []rune(ctx.filterInput)
			if ctx.filterCursor < 0 {
				ctx.filterCursor = 0
			}
			if ctx.filterCursor > len(runes) {
				ctx.filterCursor = len(runes)
			}

			redrawStatus := func() {
				if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(filterInputStatusText(filterPrompt(ctx), ctx.filterInput, ctx.filterCursor))
				}
			}

			switch event.Key() {
			case tcell.KeyEnter:
				*ctx.isFilterMode = false
				if ctx.isGrepInput {
					applyDiffGrep(ctx)
				} else {
					applyFileFilter(ctx)
				}
			case tcell.KeyEsc:
				*ctx.isFilterMode = false
				ctx.filterInput = ""
				ctx.filterCursor = 0
				if ctx.isGrepInput {
					clearDiffGrep(ctx)
				} else {
					*ctx.filterQuery = ""
				}
				ctx.updateFileListView()
				ctx.updateSelectedFileDiff()
				// restoreStatusFunc keeps a still-running grep on screen.
				if restoreStatusFunc != nil {
					restoreStatusFunc()
				} else if ctx.setGlobalStatusText != nil {
					ctx.setGlobalStatusText(fileListKeyMessage)
				}
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if ctx.filterCursor > 0 {
					ctx.filterInput = string(runes[:ctx.filterCursor-1]) + string(runes[ctx.filterCursor:])
					ctx.filterCursor--
				}
				redrawStatus()
			case tcell.KeyDelete:
				if ctx.filterCursor < len(runes) {
					ctx.filterInput = string(runes[:ctx.filterCursor]) + string(runes[ctx.filterCursor+1:])
				}
				redrawStatus()
			case tcell.KeyLeft, tcell.KeyCtrlB:
				if ctx.filterCursor > 0 {
					ctx.filterCursor--
				}
				redrawStatus()
			case tcell.KeyRight, tcell.KeyCtrlF:
				if ctx.filterCursor < len(runes) {
					ctx.filterCursor++
				}
				redrawStatus()
			case tcell.KeyHome, tcell.KeyCtrlA:
				ctx.filterCursor = 0
				redrawStatus()
			case tcell.KeyEnd, tcell.KeyCtrlE:
				ctx.filterCursor = len(runes)
				redrawStatus()
			case tcell.KeyCtrlU:
				// Clear to beginning of line
				ctx.filterInput = string(runes[ctx.filterCursor:])
				ctx.filterCursor = 0
				redrawStatus()
			case tcell.KeyCtrlK:
				// Clear to end of line
				ctx.filterInput = string(runes[:ctx.filterCursor])
				redrawStatus()
			case tcell.KeyRune:
				r := event.Rune()
				ctx.filterInput = string(runes[:ctx.filterCursor]) + string(r) + string(runes[ctx.filterCursor:])
				ctx.filterCursor++
				redrawStatus()
			}
			return nil
		}

		switch event.Key() {
		case tcell.KeyEsc:
			// If a filter or a diff grep is active, clear it first but keep the
			// cursor on the currently selected entry so users don't lose their
			// place.
			if *ctx.filterQuery != "" || diffGrepActive(ctx) {
				var selectedPath, selectedStatus string
				selectedIsDir := false
				if *ctx.currentSelection >= 0 && *ctx.currentSelection < len(*ctx.fileList) {
					entry := (*ctx.fileList)[*ctx.currentSelection]
					selectedPath = entry.Path
					selectedStatus = entry.StageStatus
					selectedIsDir = entry.IsDirectory
				}

				*ctx.filterQuery = ""
				ctx.filterInput = ""
				ctx.filterCursor = 0
				clearDiffGrep(ctx)
				ctx.updateFileListView()

				newSel := -1
				if selectedPath != "" {
					for i, fe := range *ctx.fileList {
						if fe.Path == selectedPath && fe.IsDirectory == selectedIsDir && fe.StageStatus == selectedStatus {
							newSel = i
							break
						}
					}
				}
				if newSel < 0 {
					newSel = 0
				}
				*ctx.currentSelection = newSel
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
		case tcell.KeyTab:
			moveFileListToFile(ctx, 1)
			return nil
		case tcell.KeyBacktab:
			moveFileListToFile(ctx, -1)
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
				}

				// Use updateSelectedFileDiff which handles both diff and browser modes
				ctx.updateSelectedFileDiff()

				// Redraw with cursor for the right pane
				if ctx.diffViewContext != nil && ctx.diffViewContext.viewUpdater != nil {
					ctx.diffViewContext.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, *ctx.cursorY)
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
		case tcell.KeyCtrlF:
			// Ctrl+F filters by file name; '/' greps the diff contents.
			startFilterInput(ctx, false)
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
		case tcell.KeyCtrlR:
			if ctx.readOnly {
				return nil
			}
			applyUndoRedoInFileList(ctx, ctx.undoStack.Redo, "redo")
			return nil
		case tcell.KeyCtrlA:
			if ctx.readOnly {
				return nil
			}
			snapshot, _ := ctx.undoStack.CaptureSnapshot()
			cmd := exec.Command("git", "-c", "core.quotepath=false", "add", "--all")
			cmd.Dir = ctx.repoRoot
			if err := cmd.Run(); err != nil {
				if ctx.updateGlobalStatus != nil {
					ctx.updateGlobalStatus("Failed to stage all files", "tomato")
				}
				return nil
			}
			ctx.undoStack.Push(snapshot, "stage all")

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
			case 'u':
				if ctx.readOnly {
					return nil
				}
				applyUndoRedoInFileList(ctx, ctx.undoStack.Undo, "undo")
				return nil
			case 'g':
				now := time.Now()
				if ctx.gPressed != nil && *ctx.gPressed && ctx.lastGTime != nil && now.Sub(*ctx.lastGTime) < 500*time.Millisecond {
					jumpFileListSelection(ctx, true)
					if ctx.gPressed != nil {
						*ctx.gPressed = false
					}
				} else {
					if ctx.gPressed != nil {
						*ctx.gPressed = true
					}
					if ctx.lastGTime != nil {
						*ctx.lastGTime = now
					}
				}
				return nil
			case 'G':
				jumpFileListSelection(ctx, false)
				return nil
			case 'H':
				handleFileListLeft(ctx)
				return nil
			case 'L':
				handleFileListRight(ctx)
				return nil
			case 'h': // Scroll file list left
				row, col := ctx.fileListView.GetScrollOffset()
				if col > 0 {
					ctx.fileListView.ScrollTo(row, col-4)
				}
				return nil
			case 'l': // Scroll file list right
				row, col := ctx.fileListView.GetScrollOffset()
				ctx.fileListView.ScrollTo(row, col+4)
				return nil
			case 's':
				// Toggle split view
				*ctx.isSplitView = !*ctx.isSplitView

				if *ctx.isSplitView {
					// Show split view
					updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile, ctx.repoRoot)
					ctx.contentFlex.RemoveItem(ctx.unifiedViewFlex)
					ctx.contentFlex.AddItem(ctx.splitViewFlex, 0, DiffViewFlexRatio, false)
					// Update viewUpdater for split view
					if ctx.diffViewContext != nil {
						ctx.diffViewContext.viewUpdater = NewSplitViewUpdater(ctx.beforeView, ctx.afterView, ctx.currentFile, ctx.repoRoot)
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
				// Grep the pending diffs. The file browser shows plain files and
				// the git log view has no working tree changes, so '/' keeps
				// filtering by file name there.
				inBrowser := ctx.isFileBrowserMode != nil && *ctx.isFileBrowserMode
				startFilterInput(ctx, ctx.runDiffGrep != nil && !inBrowser)
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
					updateSplitViewWithoutCursor(ctx.beforeView, ctx.afterView, *ctx.currentDiffText, *ctx.currentFile, ctx.repoRoot)
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

					snapshot, _ := ctx.undoStack.CaptureSnapshot()

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

					desc := "stage " + file
					if status == "staged" {
						desc = "unstage " + file
					}
					ctx.undoStack.Push(snapshot, desc)

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
			case 'd': // 'd' to discard changes, or back to diff/log
				if ctx.isFileBrowserMode != nil && *ctx.isFileBrowserMode {
					if ctx.toggleFileBrowser != nil {
						ctx.toggleFileBrowser()
					}
					return nil
				}
				if ctx.readOnly {
					if ctx.onEsc != nil {
						ctx.onEsc()
					}
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
					lineNum := grepCursorLineNumber(ctx)

					// Suspend application and launch $EDITOR
					editor := os.Getenv("EDITOR")
					if editor == "" {
						editor = "vim"
					}
					editorBase := filepath.Base(editor)
					ctx.app.Suspend(func() {
						var cmd *exec.Cmd
						if lineNum > 0 && (editorBase == "vim" || editorBase == "nvim") {
							cmd = exec.Command(editor, fmt.Sprintf("+%d", lineNum), filePath)
						} else {
							cmd = exec.Command(editor, filePath)
						}
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
			case 'n':
				// Walk the grep hits without leaving the file list. Stepping off
				// the end of a file continues into the next matching one, so the
				// selection follows along.
				if ctx.diffViewContext != nil && diffGrepActive(ctx) {
					moveToNextMatch(ctx.diffViewContext)
				}
				return nil
			case 'N':
				if ctx.diffViewContext != nil && diffGrepActive(ctx) {
					moveToPrevMatch(ctx.diffViewContext)
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
					arg := fileEntry.Path
					if lineNum := grepCursorLineNumber(ctx); lineNum > 0 {
						arg = fmt.Sprintf("%s:%d", fileEntry.Path, lineNum)
					}
					cmd := exec.Command("code", "-g", arg)
					cmd.Dir = ctx.repoRoot
					if err := cmd.Start(); err != nil {
						if ctx.updateGlobalStatus != nil {
							ctx.updateGlobalStatus("Failed to open VSCode", "tomato")
						}
					}
				}
				return nil
			case 'f': // 'f' to toggle file browser mode
				if ctx.toggleFileBrowser != nil {
					ctx.toggleFileBrowser()
				}
				return nil
			case 't': // 't' to open terminal command input
				if ctx.openTerminal != nil {
					ctx.openTerminal()
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
