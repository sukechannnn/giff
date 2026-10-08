package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

// splitPlaceholderLine is the filler rendered when one side of a split row has no content
const splitPlaceholderLine = "[dimgray] [-]"

// SplitViewContent represents the content for split view
type SplitViewContent struct {
	BeforeLines    []string
	AfterLines     []string
	BeforeLineNums []string
	AfterLineNums  []string
	Rows           []SplitViewRow // Per-row metadata, parallel to the slices above
}

// SplitViewRow describes what a split view row shows
type SplitViewRow struct {
	DiffStart int    // First diff display index shown on this row (-1 for fold rows)
	DiffEnd   int    // Last diff display index shown on this row (-1 for fold rows)
	FoldID    string // Fold identifier for fold indicator / expanded fold rows
	FoldFixed bool   // True if the fold is always expanded and cannot be toggled
	BgColor   string // Background color for the entire row (empty = default)

	IsFoldIndicator bool // True for a collapsed fold's "... lines hidden" row
}

// splitHunk holds the position of a hunk header within the diff body
type splitHunk struct {
	firstLine int // Index into diffLines of the hunk's first body line
	oldStart  int
	oldCount  int
	newStart  int
	newCount  int
}

// parseHunkHeader parses "@@ -oldStart[,oldCount] +newStart[,newCount] @@"
func parseHunkHeader(line string) (oldStart, oldCount, newStart, newCount int) {
	oldCount, newCount = 1, 1
	fields := strings.Fields(line)
	for _, f := range fields[1:] {
		if len(f) < 2 || (f[0] != '-' && f[0] != '+') {
			continue
		}
		start, count := 0, 1
		if i := strings.IndexByte(f, ','); i >= 0 {
			fmt.Sscanf(f[1:i], "%d", &start)
			fmt.Sscanf(f[i+1:], "%d", &count)
		} else {
			fmt.Sscanf(f[1:], "%d", &start)
		}
		if f[0] == '-' {
			oldStart, oldCount = start, count
		} else {
			newStart, newCount = start, count
			break
		}
	}
	return
}

// generateSplitViewContent generates content for split view from diff text
func generateSplitViewContent(diffText string, oldLineMap, newLineMap map[int]int, foldState *FoldState, filePath, repoRoot string) *SplitViewContent {
	// The diff's final newline ends its last line rather than adding an empty one
	lines := strings.Split(strings.TrimSuffix(diffText, "\n"), "\n")
	content := &SplitViewContent{
		BeforeLines:    []string{},
		AfterLines:     []string{},
		BeforeLineNums: []string{},
		AfterLineNums:  []string{},
	}

	addRow := func(before, beforeNum, after, afterNum string, row SplitViewRow) {
		content.BeforeLines = append(content.BeforeLines, before)
		content.AfterLines = append(content.AfterLines, after)
		content.BeforeLineNums = append(content.BeforeLineNums, beforeNum)
		content.AfterLineNums = append(content.AfterLineNums, afterNum)
		content.Rows = append(content.Rows, row)
	}

	var inHunk bool = false
	displayLine := 0

	// Calculate max digits for line numbers
	maxDigits := calculateMaxLineNumberDigits(oldLineMap, newLineMap)

	// A diff without any hunk (e.g. a binary file notice or a mode-only
	// change) has no line-based content to pair; show its non-header
	// lines as-is on both sides.
	if !hasHunkHeader(lines) {
		for _, line := range lines {
			if line == "" || isHeaderLine(line) {
				continue
			}
			escapedLine := tview.Escape(line)
			blank := strings.Repeat(" ", maxDigits)
			addRow(" "+escapedLine, blank, " "+escapedLine, blank, SplitViewRow{DiffStart: -1, DiffEnd: -1})
		}
		return content
	}

	// Pre-processing to pair deletion and addition lines
	type diffLine struct {
		content      string
		displayIndex int
		oldLineNum   int
		newLineNum   int
		lineType     string // "-", "+", " ", "other"
	}

	var diffLines []diffLine
	var hunks []splitHunk

	for _, line := range lines {
		// Hide header lines
		if isHeaderLine(line) {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			// Hunk header (hidden)
			inHunk = true
			oldStart, oldCount, newStart, newCount := parseHunkHeader(line)
			hunks = append(hunks, splitHunk{
				firstLine: len(diffLines),
				oldStart:  oldStart,
				oldCount:  oldCount,
				newStart:  newStart,
				newCount:  newCount,
			})
			continue
		} else if inHunk {
			lineType := "other"
			if strings.HasPrefix(line, "-") {
				lineType = "-"
			} else if strings.HasPrefix(line, "+") {
				lineType = "+"
			} else if strings.HasPrefix(line, " ") {
				lineType = " "
			}

			oldNum := -1
			newNum := -1
			if num, ok := oldLineMap[displayLine]; ok {
				oldNum = num
			}
			if num, ok := newLineMap[displayLine]; ok {
				newNum = num
			}

			diffLines = append(diffLines, diffLine{
				content:      line,
				displayIndex: displayLine,
				oldLineNum:   oldNum,
				newLineNum:   newNum,
				lineType:     lineType,
			})
			displayLine++
		}
	}

	// Collect code lines (without prefix) for rendering
	var codeLines []string
	for _, dl := range diffLines {
		if len(dl.content) > 0 && (dl.content[0] == '-' || dl.content[0] == '+' || dl.content[0] == ' ') {
			codeLines = append(codeLines, dl.content[1:])
		} else {
			codeLines = append(codeLines, dl.content)
		}
	}

	// Tokenize old/new file versions for syntax highlighting
	tokenSource := newDiffTokenSource(diffText, filePath, repoRoot, oldLineMap, newLineMap)

	// Helper: render a code line with syntax highlighting or fallback
	// mask is optional; when non-nil, inline diff highlighting is applied
	renderLine := func(idx int, prefix byte, bgColor string, fgColor string, mask []bool, maskBg string) string {
		tokens := tokenSource.lineTokens(diffLines[idx].displayIndex, prefix, codeLines[idx])
		if len(tokens) > 0 {
			var highlighted string
			if mask != nil {
				highlighted = util.RenderHighlightedLineWithMask(tokens, bgColor, mask, maskBg)
			} else {
				highlighted = util.RenderHighlightedLine(tokens, bgColor)
			}
			if bgColor != "" {
				return "[" + fgColor + ":" + bgColor + "]" + tview.Escape(string(prefix)) + "[-:-]" + highlighted
			}
			return tview.Escape(string(prefix)) + highlighted
		}
		// Fallback
		if mask != nil {
			prefixTag := ""
			if fgColor != "" {
				prefixTag = "[" + fgColor + ":" + bgColor + "]" + tview.Escape(string(prefix)) + "[-:-]"
			} else {
				prefixTag = tview.Escape(string(prefix))
			}
			return prefixTag + renderLineFallbackWithMask(codeLines[idx], mask, fgColor, bgColor, maskBg)
		}
		escaped := tview.Escape(diffLines[idx].content)
		if fgColor != "" {
			return "[" + fgColor + "]" + escaped + "[-]"
		}
		return escaped
	}

	// Folds: the unchanged regions between hunks, shared with unified view
	// through the fold IDs so expanding one shows it in both views.
	isHunkStart := make(map[int]bool)
	for _, h := range hunks {
		isHunkStart[h.firstLine] = true
	}
	var topFold, bottomFold *FoldableRange
	foldBeforeLine := make(map[int]*FoldableRange) // diffLines index -> fold shown before it
	foldOldOffset := make(map[string]int)          // fold ID -> old line number minus new line number
	if len(hunks) > 0 {
		foldableRanges := detectFoldableRanges(diffText, 3, getFileTotalLines(filePath, repoRoot))
		last := hunks[len(hunks)-1]
		for i := range foldableRanges {
			fold := &foldableRanges[i]
			switch fold.InsertAt {
			case -1:
				topFold = fold
				foldOldOffset[fold.ID] = hunks[0].oldStart - hunks[0].newStart
			case -2:
				bottomFold = fold
				foldOldOffset[fold.ID] = (last.oldStart + last.oldCount) - (last.newStart + last.newCount)
			default:
				// The hidden lines sit right before the first hunk starting
				// at or after them; the old side is in step with the new
				// side up to that hunk.
				for _, h := range hunks {
					if h.newStart >= fold.StartLine {
						foldBeforeLine[h.firstLine] = fold
						foldOldOffset[fold.ID] = h.oldStart - h.newStart
						break
					}
				}
			}
		}
	}

	appendFold := func(fold *FoldableRange) {
		if fold.Fixed || (foldState != nil && foldState.IsExpanded(fold.ID)) {
			offset := foldOldOffset[fold.ID]
			expandedLines := readFileLines(filePath, repoRoot, fold.StartLine, fold.EndLine)
			for lineIdx, expandedLine := range expandedLines {
				newNum := fold.StartLine + lineIdx
				var line string
				if tokens := tokenSource.newFileLineTokens(newNum, expandedLine); len(tokens) > 0 {
					line = " " + util.RenderHighlightedLine(tokens, util.ExpandedFoldBg)
				} else {
					line = fmt.Sprintf("[dimgray:%s] %s[-:-]", util.ExpandedFoldBg, tview.Escape(expandedLine))
				}
				oldNumStr := strings.Repeat(" ", maxDigits)
				if oldNum := newNum + offset; oldNum > 0 {
					oldNumStr = fmt.Sprintf("%*d", maxDigits, oldNum)
				}
				addRow(line, oldNumStr, line, fmt.Sprintf("%*d", maxDigits, newNum), SplitViewRow{
					DiffStart: -1,
					DiffEnd:   -1,
					FoldID:    fold.ID,
					FoldFixed: fold.Fixed,
					BgColor:   util.ExpandedFoldBg,
				})
			}
			return
		}
		indicator := fmt.Sprintf("[dimgray]... %d lines hidden (press 'e' to expand) ...[-]", fold.LineCount)
		blank := strings.Repeat(" ", maxDigits)
		addRow(indicator, blank, indicator, blank, SplitViewRow{DiffStart: -1, DiffEnd: -1, FoldID: fold.ID, IsFoldIndicator: true})
	}

	if topFold != nil {
		appendFold(topFold)
	}

	// Pairing: group consecutive - and + lines together
	i := 0
	codeIdx := 0 // tracks index into codeLines/allTokens
	for i < len(diffLines) {
		line := diffLines[i]
		if fold, ok := foldBeforeLine[i]; ok {
			appendFold(fold)
		}

		switch line.lineType {
		case "-":
			// Collect consecutive - lines
			startIdx := codeIdx
			deletions := []diffLine{line}
			j := i + 1
			codeIdx++
			for j < len(diffLines) && diffLines[j].lineType == "-" && !isHunkStart[j] {
				deletions = append(deletions, diffLines[j])
				j++
				codeIdx++
			}

			// Collect consecutive + lines
			addStartIdx := codeIdx
			additions := []diffLine{}
			for j < len(diffLines) && diffLines[j].lineType == "+" && !isHunkStart[j] {
				additions = append(additions, diffLines[j])
				j++
				codeIdx++
			}

			// Pair deletions and additions together
			maxLen := len(deletions)
			if len(additions) > maxLen {
				maxLen = len(additions)
			}

			for k := 0; k < maxLen; k++ {
				beforeLine := ""
				beforeLineNum := ""
				afterLine := ""
				afterLineNum := ""

				// Compute inline diff masks for paired lines
				var delMask, addMask []bool
				if k < len(deletions) && k < len(additions) {
					delMask, addMask = computeInlineDiffMasks(codeLines[startIdx+k], codeLines[addStartIdx+k])
				}

				if k < len(deletions) {
					beforeLine = renderLine(startIdx+k, '-', util.DeletedLineBg, util.DeletedLineFg, delMask, util.InlineDeletedBg)
					if deletions[k].oldLineNum >= 0 {
						beforeLineNum = fmt.Sprintf("%*d", maxDigits, deletions[k].oldLineNum)
					} else {
						beforeLineNum = strings.Repeat(" ", maxDigits)
					}
				} else {
					beforeLine = splitPlaceholderLine
					beforeLineNum = strings.Repeat(" ", maxDigits)
				}

				if k < len(additions) {
					afterLine = renderLine(addStartIdx+k, '+', util.AddedLineBg, util.AddedLineFg, addMask, util.InlineAddedBg)
					if additions[k].newLineNum >= 0 {
						afterLineNum = fmt.Sprintf("%*d", maxDigits, additions[k].newLineNum)
					} else {
						afterLineNum = strings.Repeat(" ", maxDigits)
					}
				} else {
					afterLine = splitPlaceholderLine
					afterLineNum = strings.Repeat(" ", maxDigits)
				}

				row := SplitViewRow{DiffStart: -1, DiffEnd: -1}
				if k < len(deletions) {
					row.DiffStart = deletions[k].displayIndex
					row.DiffEnd = deletions[k].displayIndex
				}
				if k < len(additions) {
					if row.DiffStart < 0 {
						row.DiffStart = additions[k].displayIndex
					}
					row.DiffEnd = additions[k].displayIndex
				}
				addRow(beforeLine, beforeLineNum, afterLine, afterLineNum, row)
			}

			i = j
		case "+":
			// Unpaired + line (addition without deletion)
			afterLineNum := strings.Repeat(" ", maxDigits)
			if line.newLineNum >= 0 {
				afterLineNum = fmt.Sprintf("%*d", maxDigits, line.newLineNum)
			}
			addRow(splitPlaceholderLine, strings.Repeat(" ", maxDigits),
				renderLine(codeIdx, '+', util.AddedLineBg, util.AddedLineFg, nil, ""), afterLineNum,
				SplitViewRow{DiffStart: line.displayIndex, DiffEnd: line.displayIndex})
			i++
			codeIdx++
		case " ":
			// Unchanged context line
			contextLine := renderLine(codeIdx, ' ', "", "", nil, "")
			beforeLineNum, afterLineNum := splitLineNums(line.oldLineNum, line.newLineNum, maxDigits)
			addRow(contextLine, beforeLineNum, contextLine, afterLineNum,
				SplitViewRow{DiffStart: line.displayIndex, DiffEnd: line.displayIndex})
			i++
			codeIdx++
		default:
			// Other lines
			escapedLine := tview.Escape(line.content)
			beforeLineNum, afterLineNum := splitLineNums(line.oldLineNum, line.newLineNum, maxDigits)
			addRow(" "+escapedLine, beforeLineNum, " "+escapedLine, afterLineNum,
				SplitViewRow{DiffStart: line.displayIndex, DiffEnd: line.displayIndex})
			i++
			codeIdx++
		}
	}

	if bottomFold != nil {
		appendFold(bottomFold)
	}

	return content
}

// splitLineNums formats the old/new line numbers of a row, blank when absent
func splitLineNums(oldNum, newNum, maxDigits int) (string, string) {
	beforeLineNum := strings.Repeat(" ", maxDigits)
	afterLineNum := strings.Repeat(" ", maxDigits)
	if oldNum >= 0 {
		beforeLineNum = fmt.Sprintf("%*d", maxDigits, oldNum)
	}
	if newNum >= 0 {
		afterLineNum = fmt.Sprintf("%*d", maxDigits, newNum)
	}
	return beforeLineNum, afterLineNum
}

// DiffRange returns the range of diff display indices shown on rows
// startRow..endRow, skipping fold rows. ok is false when the rows show no
// diff line at all.
func (c *SplitViewContent) DiffRange(startRow, endRow int) (start, end int, ok bool) {
	if startRow > endRow {
		startRow, endRow = endRow, startRow
	}
	start, end = -1, -1
	for r := startRow; r <= endRow; r++ {
		if r < 0 || r >= len(c.Rows) || c.Rows[r].DiffStart < 0 {
			continue
		}
		if start < 0 || c.Rows[r].DiffStart < start {
			start = c.Rows[r].DiffStart
		}
		if c.Rows[r].DiffEnd > end {
			end = c.Rows[r].DiffEnd
		}
	}
	return start, end, start >= 0
}

// RowToDiffIdx maps each row that shows diff lines to the last diff display
// index on it (the '+' side of a paired row). Fold rows are absent.
func (c *SplitViewContent) RowToDiffIdx() map[int]int {
	mapping := make(map[int]int)
	for r, row := range c.Rows {
		if row.DiffStart >= 0 {
			mapping[r] = row.DiffEnd
		}
	}
	return mapping
}

// RowForDiffIdx returns the row showing the given diff display index, or the
// first row after it when it is not shown. Returns the last row if none follows.
func (c *SplitViewContent) RowForDiffIdx(idx int) int {
	for r, row := range c.Rows {
		if row.DiffStart >= 0 && row.DiffEnd >= idx {
			return r
		}
	}
	return len(c.Rows) - 1
}

// FoldIDAtRow returns the ID of the toggleable fold shown on the row, or ""
func (c *SplitViewContent) FoldIDAtRow(row int) string {
	if row < 0 || row >= len(c.Rows) || c.Rows[row].FoldFixed {
		return ""
	}
	return c.Rows[row].FoldID
}

// FoldRow returns the first row belonging to the fold, or -1
func (c *SplitViewContent) FoldRow(foldID string) int {
	for r, row := range c.Rows {
		if row.FoldID == foldID {
			return r
		}
	}
	return -1
}

// hasHunkHeader reports whether the diff lines contain at least one hunk header
func hasHunkHeader(lines []string) bool {
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			return true
		}
	}
	return false
}

// isHeaderLine checks if the line is a header line that should be hidden
func isHeaderLine(line string) bool {
	return strings.HasPrefix(line, "diff --git") ||
		strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "--- ") ||
		strings.HasPrefix(line, "+++ ")
}
