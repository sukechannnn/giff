package ui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
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
}

// generateSplitViewContent generates content for split view from diff text
func generateSplitViewContent(diffText string, oldLineMap, newLineMap map[int]int, filePath string) *SplitViewContent {
	lines := strings.Split(diffText, "\n")
	content := &SplitViewContent{
		BeforeLines:    []string{},
		AfterLines:     []string{},
		BeforeLineNums: []string{},
		AfterLineNums:  []string{},
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
			content.BeforeLines = append(content.BeforeLines, " "+escapedLine)
			content.AfterLines = append(content.AfterLines, " "+escapedLine)
			content.BeforeLineNums = append(content.BeforeLineNums, strings.Repeat(" ", maxDigits))
			content.AfterLineNums = append(content.AfterLineNums, strings.Repeat(" ", maxDigits))
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

	for _, line := range lines {
		// Hide header lines
		if isHeaderLine(line) {
			continue
		}

		if strings.HasPrefix(line, "@@") {
			// Hunk header (hidden)
			inHunk = true
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

	// Collect code lines (without prefix) for tokenization
	var codeLines []string
	for _, dl := range diffLines {
		if len(dl.content) > 0 && (dl.content[0] == '-' || dl.content[0] == '+' || dl.content[0] == ' ') {
			codeLines = append(codeLines, dl.content[1:])
		} else {
			codeLines = append(codeLines, dl.content)
		}
	}

	// Tokenize
	var allTokens [][]chroma.Token
	if filePath != "" {
		allTokens = util.TokenizeCode(filePath, codeLines)
	}

	// Helper: render a code line with syntax highlighting or fallback
	// mask is optional; when non-nil, inline diff highlighting is applied
	renderLine := func(idx int, prefix byte, bgColor string, fgColor string, mask []bool, maskBg string) string {
		if allTokens != nil && len(allTokens[idx]) > 0 {
			var highlighted string
			if mask != nil {
				highlighted = util.RenderHighlightedLineWithMask(allTokens[idx], bgColor, mask, maskBg)
			} else {
				highlighted = util.RenderHighlightedLine(allTokens[idx], bgColor)
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

	// Pairing: group consecutive - and + lines together
	i := 0
	codeIdx := 0 // tracks index into codeLines/allTokens
	for i < len(diffLines) {
		line := diffLines[i]

		switch line.lineType {
		case "-":
			// Collect consecutive - lines
			startIdx := codeIdx
			deletions := []diffLine{line}
			j := i + 1
			codeIdx++
			for j < len(diffLines) && diffLines[j].lineType == "-" {
				deletions = append(deletions, diffLines[j])
				j++
				codeIdx++
			}

			// Collect consecutive + lines
			addStartIdx := codeIdx
			additions := []diffLine{}
			for j < len(diffLines) && diffLines[j].lineType == "+" {
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

				content.BeforeLines = append(content.BeforeLines, beforeLine)
				content.AfterLines = append(content.AfterLines, afterLine)
				content.BeforeLineNums = append(content.BeforeLineNums, beforeLineNum)
				content.AfterLineNums = append(content.AfterLineNums, afterLineNum)
			}

			i = j
		case "+":
			// Unpaired + line (addition without deletion)
			content.BeforeLines = append(content.BeforeLines, splitPlaceholderLine)
			content.AfterLines = append(content.AfterLines, renderLine(codeIdx, '+', util.AddedLineBg, util.AddedLineFg, nil, ""))

			content.BeforeLineNums = append(content.BeforeLineNums, strings.Repeat(" ", maxDigits))
			if line.newLineNum >= 0 {
				content.AfterLineNums = append(content.AfterLineNums, fmt.Sprintf("%*d", maxDigits, line.newLineNum))
			} else {
				content.AfterLineNums = append(content.AfterLineNums, strings.Repeat(" ", maxDigits))
			}
			i++
			codeIdx++
		case " ":
			// Unchanged context line
			contextLine := renderLine(codeIdx, ' ', "", "", nil, "")
			content.BeforeLines = append(content.BeforeLines, contextLine)
			content.AfterLines = append(content.AfterLines, contextLine)

			if line.oldLineNum >= 0 {
				content.BeforeLineNums = append(content.BeforeLineNums, fmt.Sprintf("%*d", maxDigits, line.oldLineNum))
			} else {
				content.BeforeLineNums = append(content.BeforeLineNums, strings.Repeat(" ", maxDigits))
			}
			if line.newLineNum >= 0 {
				content.AfterLineNums = append(content.AfterLineNums, fmt.Sprintf("%*d", maxDigits, line.newLineNum))
			} else {
				content.AfterLineNums = append(content.AfterLineNums, strings.Repeat(" ", maxDigits))
			}
			i++
			codeIdx++
		default:
			// Other lines
			escapedLine := tview.Escape(line.content)
			content.BeforeLines = append(content.BeforeLines, " "+escapedLine)
			content.AfterLines = append(content.AfterLines, " "+escapedLine)

			if line.oldLineNum >= 0 {
				content.BeforeLineNums = append(content.BeforeLineNums, fmt.Sprintf("%*d", maxDigits, line.oldLineNum))
			} else {
				content.BeforeLineNums = append(content.BeforeLineNums, strings.Repeat(" ", maxDigits))
			}
			if line.newLineNum >= 0 {
				content.AfterLineNums = append(content.AfterLineNums, fmt.Sprintf("%*d", maxDigits, line.newLineNum))
			} else {
				content.AfterLineNums = append(content.AfterLineNums, strings.Repeat(" ", maxDigits))
			}
			i++
			codeIdx++
		}
	}

	return content
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
