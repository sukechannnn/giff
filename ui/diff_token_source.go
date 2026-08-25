package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/sukechannnn/giff/util"
)

// diffTokenSource provides syntax highlight tokens for diff lines.
//
// The old and new versions of the file are tokenized separately as full
// files, so the lexer state at hunk boundaries is correct. Tokenizing the
// diff body as a single pseudo source breaks when a hunk starts in the
// middle of a multi-line construct (e.g. a Python docstring): the closing
// `"""` is read as an opening one, and the whole hunk is colored as a
// string. Deleted+added copies of the same delimiter interleaved in the
// diff body corrupt the state further down as well.
//
// The new side is read from the working tree. Lines whose content does not
// match the file on disk (staged diffs behind the working tree, commit
// diffs from the log view, watch-mode races) fall back to tokens from the
// diff-body pseudo source, which is what was rendered before.
type diffTokenSource struct {
	oldLines   []string
	newLines   []string
	oldTokens  [][]chroma.Token
	newTokens  [][]chroma.Token
	oldLineMap map[int]int // display index -> old file line number (1-based)
	newLineMap map[int]int // display index -> new file line number (1-based)

	// Diff-body pseudo-source tokens, used when per-file tokens are
	// unavailable or don't match the diff content.
	fallbackLines  []string
	fallbackTokens [][]chroma.Token
}

// newDiffTokenSource builds a token source for the given diff. Returns nil
// when filePath is empty (no syntax highlighting).
func newDiffTokenSource(diffText, filePath, repoRoot string, oldLineMap, newLineMap map[int]int) *diffTokenSource {
	if filePath == "" {
		return nil
	}

	s := &diffTokenSource{
		oldLineMap: oldLineMap,
		newLineMap: newLineMap,
	}

	s.fallbackLines, _ = extractDiffCodeLines(diffText)
	s.fallbackTokens = util.TokenizeCode(filePath, s.fallbackLines)

	newLines := readFileAllLines(filePath, repoRoot)
	if newLines != nil {
		s.newLines = newLines
		s.newTokens = util.TokenizeCode(filePath, newLines)
		if oldLines := reconstructOldFileLines(diffText, newLines); oldLines != nil {
			s.oldLines = oldLines
			s.oldTokens = util.TokenizeCode(filePath, oldLines)
		}
	}

	return s
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
		if n, ok := s.oldLineMap[displayIdx]; ok && s.oldTokens != nil &&
			n >= 1 && n <= len(s.oldLines) && s.oldLines[n-1] == codeLine {
			return s.oldTokens[n-1]
		}
	case '+', ' ':
		if n, ok := s.newLineMap[displayIdx]; ok && s.newTokens != nil &&
			n >= 1 && n <= len(s.newLines) && s.newLines[n-1] == codeLine {
			return s.newTokens[n-1]
		}
	}

	// Fall back to the diff-body pseudo-source tokens (previous behavior)
	if s.fallbackTokens != nil && displayIdx >= 0 && displayIdx < len(s.fallbackLines) &&
		s.fallbackLines[displayIdx] == codeLine {
		return s.fallbackTokens[displayIdx]
	}
	return nil
}

// newFileLineTokens returns the tokens for a line of the new file (1-based),
// used for fold-expanded content. lineText guards against the file changing
// between reads.
func (s *diffTokenSource) newFileLineTokens(lineNum int, lineText string) []chroma.Token {
	if s == nil || s.newTokens == nil {
		return nil
	}
	if lineNum < 1 || lineNum > len(s.newLines) || s.newLines[lineNum-1] != lineText {
		return nil
	}
	return s.newTokens[lineNum-1]
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
