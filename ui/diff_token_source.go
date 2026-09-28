package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/sukechannnn/giff/util"
)

// diffTokenSource provides syntax highlight tokens for diff lines.
//
// The old and new versions of the file are tokenized separately, so the
// lexer state at hunk boundaries is correct. Tokenizing the diff body as a
// single pseudo source breaks when a hunk starts in the middle of a
// multi-line construct (e.g. a Python docstring): the closing `"""` is read
// as an opening one, and the whole hunk is colored as a string. Deleted+added
// copies of the same delimiter interleaved in the diff body corrupt the state
// further down as well.
//
// Only the regions the diff shows are tokenized, each preceded by
// tokenLeadInLines lines of the file so the lexer is in the right state when
// the hunk starts. Tokenizing whole files made a one-line change in a large
// file take seconds to render.
//
// The new side is read from the working tree. Lines whose content does not
// match the file on disk (staged diffs behind the working tree, commit
// diffs from the log view, watch-mode races) fall back to tokens from the
// diff-body pseudo source, which is what was rendered before.
type diffTokenSource struct {
	filePath   string
	oldFile    *fileTokens
	newFile    *fileTokens
	oldLineMap map[int]int // display index -> old file line number (1-based)
	newLineMap map[int]int // display index -> new file line number (1-based)

	// Diff-body pseudo-source tokens, used when per-file tokens are
	// unavailable or don't match the diff content. Tokenized on first use.
	fallbackLines  []string
	fallbackTokens [][]chroma.Token
	fallbackDone   bool
}

// tokenLeadInLines is how many lines before a requested region are tokenized
// along with it, to bring the lexer into the state the region starts in.
const tokenLeadInLines = 300

// tokenLazyChunkLines is how many lines are tokenized at once when a line
// outside the diff (an expanded fold) is asked for.
const tokenLazyChunkLines = 200

// fileTokens holds the tokens of one version of a file, tokenized region by
// region as they are needed.
type fileTokens struct {
	filePath string
	lines    []string
	tokens   [][]chroma.Token
	done     []bool
}

func newFileTokens(filePath string, lines []string) *fileTokens {
	return &fileTokens{
		filePath: filePath,
		lines:    lines,
		tokens:   make([][]chroma.Token, len(lines)),
		done:     make([]bool, len(lines)),
	}
}

// tokenize tokenizes lines start..end (1-based, inclusive), starting the
// lexer tokenLeadInLines earlier. Lines already tokenized keep their tokens.
func (f *fileTokens) tokenize(start, end int) {
	if start < 1 {
		start = 1
	}
	if end > len(f.lines) {
		end = len(f.lines)
	}
	if start > end {
		return
	}
	from := start - tokenLeadInLines
	if from < 1 {
		from = 1
	}
	tokens := util.TokenizeCode(f.filePath, f.lines[from-1:end])
	for n := start; n <= end; n++ {
		if f.done[n-1] {
			continue
		}
		if tokens != nil {
			f.tokens[n-1] = tokens[n-from]
		}
		f.done[n-1] = true
	}
}

// line returns the tokens of line n (1-based), tokenizing it on demand.
func (f *fileTokens) line(n int) []chroma.Token {
	if f == nil || n < 1 || n > len(f.lines) {
		return nil
	}
	if !f.done[n-1] {
		f.tokenize(n, n+tokenLazyChunkLines-1)
	}
	return f.tokens[n-1]
}

// tokenizeShownLines tokenizes the lines the diff shows (the values of
// lineMap), one lexer run per group of nearby lines.
func (f *fileTokens) tokenizeShownLines(lineMap map[int]int) {
	if f == nil || len(lineMap) == 0 {
		return
	}
	nums := make([]int, 0, len(lineMap))
	for _, n := range lineMap {
		nums = append(nums, n)
	}
	sort.Ints(nums)

	// Lines closer than the lead-in share a run: tokenizing the gap costs no
	// more than a fresh lead-in would.
	start, end := nums[0], nums[0]
	for _, n := range nums[1:] {
		if n-end > tokenLeadInLines {
			f.tokenize(start, end)
			start = n
		}
		end = n
	}
	f.tokenize(start, end)
}

// newDiffTokenSource builds a token source for the given diff. Returns nil
// when filePath is empty (no syntax highlighting).
func newDiffTokenSource(diffText, filePath, repoRoot string, oldLineMap, newLineMap map[int]int) *diffTokenSource {
	if filePath == "" {
		return nil
	}

	s := &diffTokenSource{
		filePath:   filePath,
		oldLineMap: oldLineMap,
		newLineMap: newLineMap,
	}

	s.fallbackLines, _ = extractDiffCodeLines(diffText)

	newLines := readFileAllLines(filePath, repoRoot)
	if newLines != nil {
		s.newFile = newFileTokens(filePath, newLines)
		s.newFile.tokenizeShownLines(newLineMap)
		if oldLines := reconstructOldFileLines(diffText, newLines); oldLines != nil {
			s.oldFile = newFileTokens(filePath, oldLines)
			s.oldFile.tokenizeShownLines(oldLineMap)
		}
	}

	return s
}

// fileLineTokens returns the tokens of line n of f when its content is
// codeLine, reporting whether it could.
func fileLineTokens(f *fileTokens, n int, ok bool, codeLine string) ([]chroma.Token, bool) {
	if !ok || f == nil || n < 1 || n > len(f.lines) || f.lines[n-1] != codeLine {
		return nil, false
	}
	return f.line(n), true
}

// lineTokens returns the tokens for a diff body line, or nil when the line
// should be rendered without syntax highlighting. displayIdx follows the
// createLineNumberMapping convention, codeLine is the line content without
// its diff prefix.
func (s *diffTokenSource) lineTokens(displayIdx int, lineType byte, codeLine string) []chroma.Token {
	if s == nil {
		return nil
	}

	switch lineType {
	case '-':
		n, ok := s.oldLineMap[displayIdx]
		if tokens, found := fileLineTokens(s.oldFile, n, ok, codeLine); found {
			return tokens
		}
	case '+', ' ':
		n, ok := s.newLineMap[displayIdx]
		if tokens, found := fileLineTokens(s.newFile, n, ok, codeLine); found {
			return tokens
		}
	}

	// Fall back to the diff-body pseudo-source tokens (previous behavior)
	if displayIdx >= 0 && displayIdx < len(s.fallbackLines) &&
		s.fallbackLines[displayIdx] == codeLine {
		if !s.fallbackDone {
			s.fallbackTokens = util.TokenizeCode(s.filePath, s.fallbackLines)
			s.fallbackDone = true
		}
		if s.fallbackTokens != nil {
			return s.fallbackTokens[displayIdx]
		}
	}
	return nil
}

// newFileLineTokens returns the tokens for a line of the new file (1-based),
// used for fold-expanded content. lineText guards against the file changing
// between reads.
func (s *diffTokenSource) newFileLineTokens(lineNum int, lineText string) []chroma.Token {
	if s == nil {
		return nil
	}
	tokens, _ := fileLineTokens(s.newFile, lineNum, true, lineText)
	return tokens
}

// extractDiffCodeLines collects the diff body lines without their prefixes,
// along with each line's type ('-', '+', ' ', or 'o'), matching the line
// order rendered by colorizeDiff.
func extractDiffCodeLines(diff string) ([]string, []byte) {
	rawLines := util.SplitLines(diff)

	var codeLines []string
	var lineTypes []byte
	for _, line := range rawLines {
		if isUnifiedHeaderLine(line) {
			continue
		}
		if len(line) > 0 {
			switch line[0] {
			case '-', '+', ' ':
				lineTypes = append(lineTypes, line[0])
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
	return codeLines, lineTypes
}

// readFileAllLines reads the whole file split into lines, or nil if it
// cannot be read.
func readFileAllLines(filePath, repoRoot string) []string {
	if filePath == "" || repoRoot == "" {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filePath))
	if err != nil {
		return nil
	}
	return strings.Split(string(content), "\n")
}

// reconstructOldFileLines rebuilds the old version of the file from the new
// file content and the diff: regions outside hunks are identical on both
// sides, inside hunks context and `-` lines make up the old side. Returns
// nil when the diff does not line up with newLines (e.g. the file changed
// on disk after the diff was taken).
func reconstructOldFileLines(diffText string, newLines []string) []string {
	var old []string
	newPos := 1 // next new-file line (1-based) not yet copied
	inHunk := false

	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "@@") {
			var oldStart, newStart int
			fmt.Sscanf(line, "@@ -%d", &oldStart)
			parts := strings.Split(line, " +")
			if len(parts) < 2 {
				return nil
			}
			fmt.Sscanf(parts[1], "%d", &newStart)

			// Copy the unchanged region between hunks from the new file
			if newStart < newPos || newStart-1 > len(newLines) {
				return nil
			}
			old = append(old, newLines[newPos-1:newStart-1]...)
			newPos = newStart

			// "@@ -0,0 ..." means insertion at the top: the old side starts
			// at line 1, not 0.
			if oldStart == 0 {
				oldStart = 1
			}
			if oldStart != len(old)+1 {
				return nil
			}
			inHunk = true
			continue
		}
		if !inHunk || isUnifiedHeaderLine(line) {
			continue
		}
		if line == "" || strings.HasPrefix(line, "\\") {
			continue
		}
		switch line[0] {
		case '-':
			old = append(old, line[1:])
		case '+':
			newPos++
		case ' ':
			old = append(old, line[1:])
			newPos++
		}
	}

	if !inHunk {
		return nil
	}
	// Copy the identical tail after the last hunk
	if newPos-1 <= len(newLines) {
		old = append(old, newLines[newPos-1:]...)
	}
	return old
}
