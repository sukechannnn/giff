package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

// DiffViewUpdater interface for updating diff views
type DiffViewUpdater interface {
	UpdateWithoutCursor(diffText string)
	UpdateWithCursor(diffText string, cursorY int)
	UpdateWithSelection(diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool)
}

// UnifiedViewUpdater implements DiffViewUpdater for unified diff view
type UnifiedViewUpdater struct {
	diffView    *tview.TextView
	foldState   *FoldState
	filePath    *string
	repoRoot    string
	searchQuery *string // search query (for character-level highlighting)
}

// NewUnifiedViewUpdater creates a new UnifiedViewUpdater
func NewUnifiedViewUpdater(diffView *tview.TextView, foldState *FoldState, filePath *string, repoRoot string) *UnifiedViewUpdater {
	return &UnifiedViewUpdater{
		diffView:  diffView,
		foldState: foldState,
		filePath:  filePath,
		repoRoot:  repoRoot,
	}
}

// UpdateWithoutCursor updates unified view without cursor
func (u *UnifiedViewUpdater) UpdateWithoutCursor(diffText string) {
	filePath := ""
	if u.filePath != nil {
		filePath = *u.filePath
	}
	updateDiffViewWithoutCursor(u.diffView, diffText, u.foldState, filePath, u.repoRoot)
}

// UpdateWithCursor updates unified view with cursor
func (u *UnifiedViewUpdater) UpdateWithCursor(diffText string, cursorY int) {
	filePath := ""
	if u.filePath != nil {
		filePath = *u.filePath
	}
	var query string
	if u.searchQuery != nil {
		query = *u.searchQuery
	}
	renderUnifiedView(u.diffView, diffText, cursorY, -1, -1, false, u.foldState, filePath, u.repoRoot, query)
}

// UpdateWithSelection updates unified view with selection
func (u *UnifiedViewUpdater) UpdateWithSelection(diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool) {
	filePath := ""
	if u.filePath != nil {
		filePath = *u.filePath
	}
	var query string
	if u.searchQuery != nil {
		query = *u.searchQuery
	}
	renderUnifiedView(u.diffView, diffText, cursorY, selectStart, selectEnd, isSelecting, u.foldState, filePath, u.repoRoot, query)
}

// SplitViewUpdater implements DiffViewUpdater for split diff view
type SplitViewUpdater struct {
	beforeView *tview.TextView
	afterView  *tview.TextView
	filePath   *string
}

// NewSplitViewUpdater creates a new SplitViewUpdater
func NewSplitViewUpdater(beforeView, afterView *tview.TextView, filePath *string) *SplitViewUpdater {
	return &SplitViewUpdater{
		beforeView: beforeView,
		afterView:  afterView,
		filePath:   filePath,
	}
}

// UpdateWithoutCursor updates split view without cursor
func (s *SplitViewUpdater) UpdateWithoutCursor(diffText string) {
	filePath := ""
	if s.filePath != nil {
		filePath = *s.filePath
	}
	updateSplitViewWithoutCursor(s.beforeView, s.afterView, diffText, filePath)
}

// UpdateWithCursor updates split view with cursor
func (s *SplitViewUpdater) UpdateWithCursor(diffText string, cursorY int) {
	filePath := ""
	if s.filePath != nil {
		filePath = *s.filePath
	}
	updateSplitViewWithCursor(s.beforeView, s.afterView, diffText, cursorY, filePath)
}

// UpdateWithSelection updates split view with selection
func (s *SplitViewUpdater) UpdateWithSelection(diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool) {
	filePath := ""
	if s.filePath != nil {
		filePath = *s.filePath
	}
	updateSplitViewWithSelection(s.beforeView, s.afterView, diffText, cursorY, selectStart, selectEnd, isSelecting, filePath)
}

// ----------↓↓↓ unified_view_functions ↓↓↓----------

// unifiedContentCache caches the generated unified view content to avoid regeneration on cursor moves
var unifiedContentCache struct {
	diffText string
	filePath string
	content  *UnifiedViewContent
}

func getCachedUnifiedContent(diffText string, foldState *FoldState, filePath, repoRoot string) *UnifiedViewContent {
	if unifiedContentCache.diffText == diffText && unifiedContentCache.filePath == filePath && unifiedContentCache.content != nil {
		return unifiedContentCache.content
	}
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, foldState, filePath, repoRoot)
	unifiedContentCache.diffText = diffText
	unifiedContentCache.filePath = filePath
	unifiedContentCache.content = content
	return content
}

// InvalidateUnifiedContentCache clears the unified content cache (call when fold state changes)
func InvalidateUnifiedContentCache() {
	unifiedContentCache.content = nil
}

func updateDiffViewWithoutCursor(diffView *tview.TextView, diffText string, foldState *FoldState, filePath, repoRoot string) {
	renderUnifiedView(diffView, diffText, -1, -1, -1, false, foldState, filePath, repoRoot, "")
}

func updateDiffViewWithCursor(diffView *tview.TextView, diffText string, cursorY int, foldState *FoldState, filePath, repoRoot string) {
	renderUnifiedView(diffView, diffText, cursorY, -1, -1, false, foldState, filePath, repoRoot, "")
}

func updateDiffViewWithSelection(diffView *tview.TextView, diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool, foldState *FoldState, filePath, repoRoot string) {
	renderUnifiedView(diffView, diffText, cursorY, selectStart, selectEnd, isSelecting, foldState, filePath, repoRoot, "")
}

func renderUnifiedView(diffView *tview.TextView, diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool, foldState *FoldState, filePath, repoRoot string, searchQuery string) {
	diffView.Clear()

	content := getCachedUnifiedContent(diffText, foldState, filePath, repoRoot)

	for i, line := range content.Lines {
		var bg string
		var lineNumFg string
		if isSelecting && isLineSelected(i, selectStart, selectEnd) {
			bg = "dimgrey"
			lineNumFg = "white"
		} else if i == cursorY {
			bg = "blue"
			lineNumFg = "white"
		} else if line.BgColor != "" {
			bg = line.BgColor
			lineNumFg = "dimgray"
		}

		// Apply character-level highlighting if search query exists
		lineContent := line.Content
		if searchQuery != "" {
			lineContent = highlightSearchInTaggedText(lineContent, searchQuery)
		}

		if bg != "" {
			lineNum := util.ReplaceBackground(line.LineNumber, bg)
			var highlighted string
			if searchQuery != "" {
				highlighted = util.ReplaceBackgroundPreserving(lineContent, bg, []string{util.SearchHighlightBg})
			} else {
				highlighted = util.ReplaceBackground(lineContent, bg)
			}
			diffView.Write([]byte("[" + lineNumFg + ":" + bg + "]" + lineNum + highlighted + strings.Repeat(" ", 500) + "[-:-]\n"))
		} else {
			diffView.Write([]byte("[dimgray]" + line.LineNumber + "[-]" + lineContent + "\n"))
		}
	}

	// Adjust scroll position (keep cursor visible)
	_, _, _, height := diffView.GetInnerRect()
	currentRow, _ := diffView.GetScrollOffset()

	// If cursor is below the screen
	if cursorY >= currentRow+height-1 {
		diffView.ScrollTo(cursorY-height+2, 0)
	}
	// If cursor is above the screen
	if cursorY < currentRow {
		diffView.ScrollTo(cursorY, 0)
	}
}

// getUnifiedViewLineCount gets valid line count for unified view
func getUnifiedViewLineCount(diffText string) int {
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, nil, "", "")
	return len(content.Lines)
}

// ----------↑↑↑ unified_view_functions ↑↑↑----------

// ----------↓↓↓ split_view_functions ↓↓↓----------

// splitContentCache caches the generated split view content to avoid regeneration on cursor moves
var splitContentCache struct {
	diffText string
	filePath string
	content  *SplitViewContent
}

func getCachedSplitContent(diffText string, filePath string) *SplitViewContent {
	if splitContentCache.diffText == diffText && splitContentCache.filePath == filePath && splitContentCache.content != nil {
		return splitContentCache.content
	}
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateSplitViewContent(diffText, oldLineMap, newLineMap, filePath)
	splitContentCache.diffText = diffText
	splitContentCache.filePath = filePath
	splitContentCache.content = content
	return content
}

// getSplitViewLineCount gets valid line count for split view
func getSplitViewLineCount(diffText string) int {
	content := getCachedSplitContent(diffText, "")
	return len(content.BeforeLines)
}

func updateSplitViewWithoutCursor(beforeView, afterView *tview.TextView, diffText string, filePath string) {
	renderSplitView(beforeView, afterView, diffText, -1, -1, -1, false, filePath)
}

// updateSplitViewWithCursor updates split view with cursor
func updateSplitViewWithCursor(beforeView, afterView *tview.TextView, diffText string, cursorY int, filePath string) {
	renderSplitView(beforeView, afterView, diffText, cursorY, -1, -1, false, filePath)
}

func updateSplitViewWithSelection(beforeView, afterView *tview.TextView, diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool, filePath string) {
	renderSplitView(beforeView, afterView, diffText, cursorY, selectStart, selectEnd, isSelecting, filePath)
}

func renderSplitView(beforeView, afterView *tview.TextView, diffText string, cursorY int, selectStart int, selectEnd int, isSelecting bool, filePath string) {
	beforeView.Clear()
	afterView.Clear()

	content := getCachedSplitContent(diffText, filePath)
	beforeLines := content.BeforeLines
	afterLines := content.AfterLines
	beforeLineNums := content.BeforeLineNums
	afterLineNums := content.AfterLineNums

	// Get actual index of cursor line (simplified)
	// cursorY is treated as a display line index
	cursorIndex := -1
	if cursorY >= 0 && cursorY < len(beforeLines) {
		cursorIndex = cursorY
	}

	// Update display
	for i, line := range beforeLines {
		// Add line number
		lineNum := beforeLineNums[i] + " │ "

		if isSelecting && isLineSelected(i, selectStart, selectEnd) {
			// Selected line: replace background with dimgrey
			highlighted := util.ReplaceBackground(line, "dimgrey")
			beforeView.Write([]byte("[white:dimgrey]" + lineNum + "[-:-]" + highlighted + "[-:-]\n"))
		} else if cursorIndex >= 0 && i == cursorIndex {
			// Cursor line: replace background with blue
			highlighted := util.ReplaceBackground(line, "blue")
			beforeView.Write([]byte("[white:blue]" + lineNum + "[-:-]" + highlighted + "[-:-]\n"))
		} else {
			beforeView.Write([]byte("[dimgray]" + lineNum + "[-]" + line + "\n"))
		}
	}

	for i, line := range afterLines {
		// Add line number
		lineNum := afterLineNums[i] + " │ "

		if isSelecting && isLineSelected(i, selectStart, selectEnd) {
			// Selected line: replace background with dimgrey
			highlighted := util.ReplaceBackground(line, "dimgrey")
			afterView.Write([]byte("[white:dimgrey]" + lineNum + "[-:-]" + highlighted + "[-:-]\n"))
		} else if cursorIndex >= 0 && i == cursorIndex {
			// Cursor line: replace background with blue
			highlighted := util.ReplaceBackground(line, "blue")
			afterView.Write([]byte("[white:blue]" + lineNum + "[-:-]" + highlighted + "[-:-]\n"))
		} else {
			afterView.Write([]byte("[dimgray]" + lineNum + "[-]" + line + "\n"))
		}
	}

	// Synchronize scroll position
	if cursorIndex >= 0 {
		_, _, _, height := beforeView.GetInnerRect()
		currentRow, _ := beforeView.GetScrollOffset()

		// If cursor is below the screen
		if cursorIndex >= currentRow+height-1 {
			scrollPos := cursorIndex - height + 2
			beforeView.ScrollTo(scrollPos, 0)
			afterView.ScrollTo(scrollPos, 0)
		}
		// If cursor is above the screen
		if cursorIndex < currentRow {
			beforeView.ScrollTo(cursorIndex, 0)
			afterView.ScrollTo(cursorIndex, 0)
		}
	} else {
		// Without cursor, scroll to top
		beforeView.ScrollTo(0, 0)
		afterView.ScrollTo(0, 0)
	}
}

// ----------↑↑↑ split_view_functions ↑↑↑----------

// isLineSelected checks if line is in selection range
func isLineSelected(index, start, end int) bool {
	if start == -1 || end == -1 {
		return false
	}
	min := start
	max := end
	if min > max {
		min, max = max, min
	}
	return index >= min && index <= max
}

// createLineNumberMapping creates line number mapping from diff text
func createLineNumberMapping(diffText string) (map[int]int, map[int]int) {
	oldLineMap := make(map[int]int)
	newLineMap := make(map[int]int)

	lines := strings.Split(diffText, "\n")
	displayLine := 0
	var oldLineNum, newLineNum int
	inHunk := false

	for _, line := range lines {
		// Skip header lines (same logic as ColorizeDiff)
		if strings.HasPrefix(line, "diff --git") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "@@") {
			// Get line numbers from hunk header
			if strings.HasPrefix(line, "@@") {
				// @@ -oldStart,oldCount +newStart,newCount @@
				var oldStart, newStart int
				fmt.Sscanf(line, "@@ -%d", &oldStart)
				parts := strings.Split(line, " +")
				if len(parts) >= 2 {
					fmt.Sscanf(parts[1], "%d", &newStart)
				}
				oldLineNum = oldStart
				newLineNum = newStart
				inHunk = true
			}
			continue
		}

		if !inHunk {
			continue
		}

		// Actual diff lines (only count lines displayed by ColorizeDiff)
		if strings.HasPrefix(line, "-") {
			oldLineMap[displayLine] = oldLineNum
			oldLineNum++
		} else if strings.HasPrefix(line, "+") {
			newLineMap[displayLine] = newLineNum
			newLineNum++
		} else {
			// Lines starting with space or other lines (context lines)
			oldLineMap[displayLine] = oldLineNum
			newLineMap[displayLine] = newLineNum
			oldLineNum++
			newLineNum++
		}

		displayLine++
	}

	return oldLineMap, newLineMap
}

// FileViewUpdater implements DiffViewUpdater for file content viewing
type FileViewUpdater struct {
	diffView    *tview.TextView
	filePath    *string
	searchQuery *string
}

func (f *FileViewUpdater) UpdateWithoutCursor(content string) {
	renderFileView(f.diffView, content, -1, -1, -1, false, *f.filePath, f.getSearchQuery())
}

func (f *FileViewUpdater) UpdateWithCursor(content string, cursorY int) {
	renderFileView(f.diffView, content, cursorY, -1, -1, false, *f.filePath, f.getSearchQuery())
}

func (f *FileViewUpdater) UpdateWithSelection(content string, cursorY int, selectStart int, selectEnd int, isSelecting bool) {
	renderFileView(f.diffView, content, cursorY, selectStart, selectEnd, isSelecting, *f.filePath, f.getSearchQuery())
}

func (f *FileViewUpdater) getSearchQuery() string {
	if f.searchQuery != nil {
		return *f.searchQuery
	}
	return ""
}

// renderFileView renders file content with syntax highlighting and line numbers
func renderFileView(diffView *tview.TextView, content string, cursorY int, selectStart int, selectEnd int, isSelecting bool, filePath string, searchQuery string) {
	diffView.Clear()

	if content == "" {
		diffView.SetText("[dimgray]Empty file[-]")
		return
	}

	lines := strings.Split(content, "\n")
	// Remove trailing empty line from Split
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	// Tokenize for syntax highlighting
	var allTokens [][]chroma.Token
	if filePath != "" {
		allTokens = util.TokenizeCode(filePath, lines)
	}

	// Calculate line number width
	maxDigits := len(fmt.Sprintf("%d", len(lines)))

	for i, line := range lines {
		lineNum := fmt.Sprintf("[dimgray]%*d │ [-]", maxDigits, i+1)

		var lineContent string
		if allTokens != nil && i < len(allTokens) && len(allTokens[i]) > 0 {
			lineContent = util.RenderHighlightedLine(allTokens[i], "")
		} else {
			lineContent = tview.Escape(line)
		}

		// Apply search highlighting
		if searchQuery != "" {
			lineContent = highlightSearchInTaggedText(lineContent, searchQuery)
		}

		// Apply cursor/selection background
		bg := ""
		lineNumFg := "dimgray"
		if isSelecting && isLineSelected(i, selectStart, selectEnd) {
			bg = "dimgrey"
			lineNumFg = "white"
		} else if cursorY >= 0 && i == cursorY {
			bg = "blue"
			lineNumFg = "white"
		}

		if bg != "" {
			highlighted := util.ReplaceBackground(lineContent, bg)
			if searchQuery != "" {
				highlighted = util.ReplaceBackgroundPreserving(lineContent, bg, []string{util.SearchHighlightBg})
			}
			diffView.Write([]byte("[" + lineNumFg + ":" + bg + "]" + fmt.Sprintf("%*d │ ", maxDigits, i+1) + highlighted + strings.Repeat(" ", 500) + "[-:-]\n"))
		} else {
			diffView.Write([]byte(lineNum + lineContent + "\n"))
		}
	}

	// Scroll to keep cursor visible
	if cursorY >= 0 {
		_, _, _, height := diffView.GetInnerRect()
		currentRow, _ := diffView.GetScrollOffset()
		if cursorY >= currentRow+height-1 {
			diffView.ScrollTo(cursorY-height+2, 0)
		} else if cursorY < currentRow {
			diffView.ScrollTo(cursorY, 0)
		}
	}
}
