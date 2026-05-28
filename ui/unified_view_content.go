package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

// ColorizedLine represents a colorized diff line with its type
type ColorizedLine struct {
	Content  string // Color-tagged content
	LineType byte   // '+', '-', ' ', 'o' (other/fold)
}

// UnifiedViewLine represents a single line in unified view
type UnifiedViewLine struct {
	Content         string
	LineNumber      string
	LineType        byte   // '+', '-', ' ', 'o' (other/fold)
	IsFoldIndicator bool   // True if this is a fold indicator line (not a real diff line)
	FoldID          string // Fold identifier (empty if not a fold indicator)
	BgColor         string // Background color for the entire line (empty = default)
}

// UnifiedViewContent represents the content for unified view
type UnifiedViewContent struct {
	Lines []UnifiedViewLine
}

// FoldableRange represents a range of lines that can be folded
type FoldableRange struct {
	StartLine int    // File line number where fold starts
	EndLine   int    // File line number where fold ends
	InsertAt  int    // Display index where fold indicator should be inserted
	LineCount int    // Number of lines in the fold
	ID        string // Unique identifier for this fold
}

// GetUnifiedViewLineCount returns the actual line count including fold indicators
func GetUnifiedViewLineCount(diffText string, foldState *FoldState, filePath, repoRoot string) int {
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, foldState, filePath, repoRoot)
	return len(content.Lines)
}

// MapUnifiedDisplayToOriginalIdx maps unified view display indices (including fold indicators)
// to original diff line indices (excluding headers)
func MapUnifiedDisplayToOriginalIdx(diffText string, foldState *FoldState, filePath, repoRoot string) map[int]int {
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, foldState, filePath, repoRoot)

	mapping := make(map[int]int)
	originalIdx := 0

	for displayIdx, line := range content.Lines {
		// Skip fold indicator lines and expanded fold content (both have FoldID set)
		if line.FoldID != "" {
			continue
		}
		// This is a real diff line, map it to the original index
		mapping[displayIdx] = originalIdx
		originalIdx++
	}

	return mapping
}

// detectFoldableRanges detects ranges that can be folded based on new-file line numbers.
// Fold content is read from the current (new) file, so gap detection must use new-file
// line numbers consistently. Mixing old and new line numbers (e.g., for `-` lines that
// only have old-file numbers) produces spurious folds when many deletions or additions
// cause the two numbering systems to diverge within a hunk.
// totalLines is the total number of lines in the new file (0 if unknown, which disables bottom folds).
func detectFoldableRanges(diffText string, minGap int, totalLines int) []FoldableRange {
	lines := strings.Split(diffText, "\n")

	// Parse diff, collecting display indices of lines that correspond to actual new-file
	// lines (context and `+` lines). Track the new-file position before the first shown
	// line and after the last one for top/bottom fold detection.
	type shownEntry struct {
		displayIdx int
		newLine    int
	}
	var shown []shownEntry

	displayLine := 0
	var newLineNum int
	inHunk := false
	firstNewPosition := -1

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			var newStart int
			parts := strings.Split(line, " +")
			if len(parts) >= 2 {
				fmt.Sscanf(parts[1], "%d", &newStart)
			}
			newLineNum = newStart
			inHunk = true
			continue
		}
		if strings.HasPrefix(line, "diff --git") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "--- ") ||
			strings.HasPrefix(line, "+++ ") {
			continue
		}
		if !inHunk {
			continue
		}
		// Real diff body lines always start with a space, '+', or '-'. Anything
		// else (empty trailing line from strings.Split, "\ No newline" markers)
		// must not be treated as content or it shifts new-file line tracking.
		if line == "" || strings.HasPrefix(line, "\\") {
			continue
		}

		if firstNewPosition < 0 {
			firstNewPosition = newLineNum
		}

		if strings.HasPrefix(line, "-") {
			// `-` lines don't correspond to a new-file line; new position doesn't advance.
		} else if strings.HasPrefix(line, "+") {
			shown = append(shown, shownEntry{displayIdx: displayLine, newLine: newLineNum})
			newLineNum++
		} else {
			shown = append(shown, shownEntry{displayIdx: displayLine, newLine: newLineNum})
			newLineNum++
		}
		displayLine++
	}

	if displayLine == 0 {
		return nil
	}

	lastNewPositionAfter := newLineNum

	var ranges []FoldableRange

	// Top fold: hidden lines before the first displayed line.
	if firstNewPosition > 1 {
		topGap := firstNewPosition - 1
		if topGap >= minGap {
			ranges = append(ranges, FoldableRange{
				StartLine: 1,
				EndLine:   firstNewPosition - 1,
				InsertAt:  -1, // Special value: insert at the beginning
				LineCount: topGap,
				ID:        fmt.Sprintf("fold-top-1-%d", firstNewPosition-1),
			})
		}
	}

	// Mid folds: gaps between consecutive lines that actually show new-file content.
	// Insert the fold indicator just before the next shown line so any intervening
	// `-` lines render with the preceding change block, not after the fold.
	for i := 0; i < len(shown)-1; i++ {
		current := shown[i]
		next := shown[i+1]
		gap := next.newLine - current.newLine - 1
		if gap >= minGap {
			ranges = append(ranges, FoldableRange{
				StartLine: current.newLine + 1,
				EndLine:   next.newLine - 1,
				InsertAt:  next.displayIdx - 1,
				LineCount: gap,
				ID:        fmt.Sprintf("fold-%d-%d", current.newLine+1, next.newLine-1),
			})
		}
	}

	// Bottom fold: hidden lines after the last displayed line.
	if totalLines > 0 && lastNewPositionAfter <= totalLines {
		bottomGap := totalLines - lastNewPositionAfter + 1
		if bottomGap >= minGap {
			ranges = append(ranges, FoldableRange{
				StartLine: lastNewPositionAfter,
				EndLine:   totalLines,
				InsertAt:  -2, // Special value: insert at the end
				LineCount: bottomGap,
				ID:        fmt.Sprintf("fold-bottom-%d-%d", lastNewPositionAfter, totalLines),
			})
		}
	}

	return ranges
}

// generateUnifiedViewContent generates content for unified view from diff text
func generateUnifiedViewContent(diffText string, oldLineMap, newLineMap map[int]int, foldState *FoldState, filePath, repoRoot string) *UnifiedViewContent {
	// First colorize the diff
	coloredLines := colorizeDiff(diffText, filePath)

	// Calculate max digits for line numbers
	maxDigits := calculateMaxLineNumberDigits(oldLineMap, newLineMap)

	// Get total lines in file for top/bottom fold detection
	totalLines := getFileTotalLines(filePath, repoRoot)

	// Detect foldable ranges (minimum 3 lines gap)
	foldableRanges := detectFoldableRanges(diffText, 3, totalLines)

	// Create maps for quick lookup
	foldMap := make(map[int]*FoldableRange)
	var topFold, bottomFold *FoldableRange
	for i := range foldableRanges {
		fold := &foldableRanges[i]
		if fold.InsertAt == -1 {
			topFold = fold
		} else if fold.InsertAt == -2 {
			bottomFold = fold
		} else {
			foldMap[fold.InsertAt] = fold
		}
	}

	// Build content with line numbers
	content := &UnifiedViewContent{
		Lines: []UnifiedViewLine{},
	}

	// Insert top fold indicator/content at the beginning
	if topFold != nil {
		appendFoldContent(content, topFold, foldState, filePath, repoRoot, maxDigits)
	}

	for i, cl := range coloredLines {
		lineNum := generateLineNumber(cl.LineType, i, maxDigits, oldLineMap, newLineMap)
		content.Lines = append(content.Lines, UnifiedViewLine{
			Content:         cl.Content,
			LineNumber:      lineNum,
			LineType:        cl.LineType,
			IsFoldIndicator: false,
		})

		// Check if we should insert a fold indicator after this line
		if fold, exists := foldMap[i]; exists {
			appendFoldContent(content, fold, foldState, filePath, repoRoot, maxDigits)
		}
	}

	// Insert bottom fold indicator/content at the end
	if bottomFold != nil {
		appendFoldContent(content, bottomFold, foldState, filePath, repoRoot, maxDigits)
	}

	return content
}

// appendFoldContent appends fold indicator or expanded content to the unified view
func appendFoldContent(content *UnifiedViewContent, fold *FoldableRange, foldState *FoldState, filePath, repoRoot string, maxDigits int) {
	if foldState != nil && foldState.IsExpanded(fold.ID) {
		// Expanded: show actual file content
		expandedLines := readFileLines(filePath, repoRoot, fold.StartLine, fold.EndLine)

		// Try to syntax highlight expanded lines
		var allTokens [][]chroma.Token
		if filePath != "" {
			allTokens = util.TokenizeCode(filePath, expandedLines)
		}

		for lineIdx, expandedLine := range expandedLines {
			actualLineNum := fold.StartLine + lineIdx
			lineNumStr := fmt.Sprintf("[dimgray:%s]%*d │ [-:-]", util.ExpandedFoldBg, maxDigits, actualLineNum)

			var lineContent string
			if allTokens != nil && len(allTokens[lineIdx]) > 0 {
				lineContent = " " + util.RenderHighlightedLine(allTokens[lineIdx], util.ExpandedFoldBg)
			} else {
				lineContent = fmt.Sprintf("[dimgray:%s] %s[-:-]", util.ExpandedFoldBg, tview.Escape(expandedLine))
			}

			content.Lines = append(content.Lines, UnifiedViewLine{
				Content:         lineContent,
				LineNumber:      lineNumStr,
				LineType:        'o',
				IsFoldIndicator: false,
				FoldID:          fold.ID,
				BgColor:         util.ExpandedFoldBg,
			})
		}
	} else {
		// Collapsed: show fold indicator
		foldIndicator := fmt.Sprintf("[dimgray]... %d lines hidden (press 'e' to expand) ...[-]", fold.LineCount)
		content.Lines = append(content.Lines, UnifiedViewLine{
			Content:         foldIndicator,
			LineNumber:      strings.Repeat(" ", maxDigits) + " │ ",
			LineType:        'o',
			IsFoldIndicator: true,
			FoldID:          fold.ID,
		})
	}
}

// getFileTotalLines returns the total number of lines in a file
func getFileTotalLines(filePath, repoRoot string) int {
	if filePath == "" || repoRoot == "" {
		return 0
	}
	fullPath := filepath.Join(repoRoot, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(content), "\n")
	return len(lines)
}

// GetFoldIDAtLine returns the fold ID at the given display line index, or empty string if none
func GetFoldIDAtLine(diffText string, lineIndex int, foldState *FoldState, filePath, repoRoot string) string {
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, foldState, filePath, repoRoot)

	if lineIndex >= 0 && lineIndex < len(content.Lines) {
		return content.Lines[lineIndex].FoldID
	}
	return ""
}

// GetFoldIndicatorPosition returns the display line index of a fold indicator after toggling
// This is used to move cursor to the fold indicator when collapsing
func GetFoldIndicatorPosition(diffText string, foldID string, foldState *FoldState, filePath, repoRoot string) int {
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateUnifiedViewContent(diffText, oldLineMap, newLineMap, foldState, filePath, repoRoot)

	for i, line := range content.Lines {
		if line.FoldID == foldID {
			return i
		}
	}
	return 0
}

// readFileLines reads lines from a file within the specified range (1-indexed, inclusive)
func readFileLines(filePath, repoRoot string, startLine, endLine int) []string {
	fullPath := filepath.Join(repoRoot, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return []string{fmt.Sprintf("Error reading file: %v", err)}
	}

	lines := strings.Split(string(content), "\n")

	// Convert to 0-indexed
	start := startLine - 1
	end := endLine // endLine is inclusive, so we use it as the slice end

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= len(lines) {
		return []string{}
	}

	return lines[start:end]
}

// colorizeDiff colorizes diff text and filters out headers.
// If filePath is non-empty, syntax highlighting via chroma is applied.
func colorizeDiff(diff string, filePath string) []ColorizedLine {
	rawLines := util.SplitLines(diff)

	// Collect code lines (without diff prefix) for tokenization
	var codeLines []string
	var lineTypes []byte
	for _, line := range rawLines {
		if isUnifiedHeaderLine(line) {
			continue
		}
		if len(line) > 0 {
			switch line[0] {
			case '-':
				lineTypes = append(lineTypes, '-')
				codeLines = append(codeLines, line[1:])
			case '+':
				lineTypes = append(lineTypes, '+')
				codeLines = append(codeLines, line[1:])
			case ' ':
				lineTypes = append(lineTypes, ' ')
				codeLines = append(codeLines, line[1:])
			default:
				lineTypes = append(lineTypes, 'o')
				codeLines = append(codeLines, line)
			}
		} else {
			lineTypes = append(lineTypes, 'o')
			codeLines = append(codeLines, "")
		}
	}

	// Try to tokenize with chroma
	var allTokens [][]chroma.Token
	if filePath != "" {
		allTokens = util.TokenizeCode(filePath, codeLines)
	}

	// Compute inline diff masks for adjacent -/+ pairs
	inlineMasks := computeAllInlineMasks(codeLines, lineTypes)

	result := make([]ColorizedLine, len(codeLines))
	for i, codeLine := range codeLines {
		lt := lineTypes[i]
		var content string

		var bgColor, fgColor, inlineBg string
		switch lt {
		case '-':
			bgColor = util.DeletedLineBg
			fgColor = util.DeletedLineFg
			inlineBg = util.InlineDeletedBg
		case '+':
			bgColor = util.AddedLineBg
			fgColor = util.AddedLineFg
			inlineBg = util.InlineAddedBg
		}
		mask := inlineMasks[i]

		if allTokens != nil && len(allTokens[i]) > 0 {
			// Syntax highlighted rendering
			prefix := ""
			if lt == '-' || lt == '+' || lt == ' ' {
				prefix = string(lt)
			}
			var highlighted string
			if mask != nil {
				highlighted = util.RenderHighlightedLineWithMask(allTokens[i], bgColor, mask, inlineBg)
			} else {
				highlighted = util.RenderHighlightedLine(allTokens[i], bgColor)
			}
			if bgColor != "" {
				content = "[" + fgColor + ":" + bgColor + "]" + tview.Escape(prefix) + "[-:-]" + highlighted
			} else {
				content = tview.Escape(prefix) + highlighted
			}
		} else {
			// Fallback: use simple coloring
			if mask != nil {
				prefix := ""
				if lt == '-' || lt == '+' || lt == ' ' {
					prefix = string(lt)
				}
				prefixTag := ""
				if fgColor != "" {
					prefixTag = "[" + fgColor + ":" + bgColor + "]" + tview.Escape(prefix) + "[-:-]"
				} else {
					prefixTag = tview.Escape(prefix)
				}
				content = prefixTag + renderLineFallbackWithMask(codeLine, mask, fgColor, bgColor, inlineBg)
			} else {
				fullLine := ""
				if lt == '-' || lt == '+' || lt == ' ' {
					fullLine = string(lt) + codeLine
				} else {
					fullLine = codeLine
				}
				content = colorizeLineFallback(fullLine)
			}
		}

		result[i] = ColorizedLine{
			Content:  content,
			LineType: lt,
		}
	}

	return result
}

// ColorizeDiff is a public wrapper for backward compatibility
func ColorizeDiff(diff string) string {
	lines := colorizeDiff(diff, "")
	contents := make([]string, len(lines))
	for i, l := range lines {
		contents[i] = l.Content
	}
	return strings.Join(contents, "\n") + "\n"
}

// isUnifiedHeaderLine checks if the line is a header that should be skipped
func isUnifiedHeaderLine(line string) bool {
	return strings.HasPrefix(line, "diff --git") ||
		strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "--- ") ||
		strings.HasPrefix(line, "+++ ") ||
		strings.HasPrefix(line, "@@")
}

// colorizeLineFallback adds color tags to a single line based on its type (fallback when no syntax highlighting)
func colorizeLineFallback(line string) string {
	if len(line) > 0 {
		switch line[0] {
		case '-':
			return "[" + util.DeletedLineFg + "]" + tview.Escape(line) + "[-]"
		case '+':
			return "[" + util.AddedLineFg + "]" + tview.Escape(line) + "[-]"
		case ' ':
			return tview.Escape(line)
		default:
			return tview.Escape(line)
		}
	}
	return ""
}

// generateLineNumber generates the line number string for a given line type
func generateLineNumber(lineType byte, index int, maxDigits int, oldLineMap, newLineMap map[int]int) string {
	var lineNum string

	switch lineType {
	case '-':
		// Deletion line
		if num, ok := oldLineMap[index]; ok {
			lineNum = fmt.Sprintf("%*d", maxDigits, num)
		} else {
			lineNum = strings.Repeat(" ", maxDigits)
		}
		lineNum += " │ "
	case '+':
		// Addition line
		if num, ok := newLineMap[index]; ok {
			lineNum = fmt.Sprintf("%*d", maxDigits, num)
		} else {
			lineNum = strings.Repeat(" ", maxDigits)
		}
		lineNum += " │ "
	default:
		// Common line
		if num, ok := newLineMap[index]; ok {
			lineNum = fmt.Sprintf("%*d", maxDigits, num)
		} else if num, ok := oldLineMap[index]; ok {
			lineNum = fmt.Sprintf("%*d", maxDigits, num)
		} else {
			lineNum = strings.Repeat(" ", maxDigits)
		}
		lineNum += " │ "
	}

	return lineNum
}
