package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DiscardSelectedChanges reverts the changes on diff lines selectedStart..selectedEnd
// (indices into diffText split by "\n") in the working tree file, leaving every other
// change as it is. diffText must be the unstaged diff of the file (index -> working tree).
//
// The working tree file is used as the base so lines outside the selection keep their
// current content even when the diff was taken with whitespace changes hidden: selected
// '+' lines are removed and selected '-' lines are put back.
func DiscardSelectedChanges(filePath string, repoRoot string, diffText string, selectedStart, selectedEnd int) error {
	fullPath := filepath.Join(repoRoot, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	result, err := discardSelectedDiffLines(string(content), strings.Split(diffText, "\n"), selectedStart, selectedEnd)
	if err != nil {
		return err
	}

	// os.WriteFile keeps the mode of an existing file
	if err := os.WriteFile(fullPath, []byte(result), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// discardSelectedDiffLines returns workingContent with the selected diff lines reverted
func discardSelectedDiffLines(workingContent string, diffLines []string, selectedStart, selectedEnd int) (string, error) {
	workingLines := strings.Split(workingContent, "\n")
	result := make([]string, 0, len(workingLines))

	pos := 0 // index into workingLines
	inHunk := false

	for i, line := range diffLines {
		if strings.HasPrefix(line, "@@") {
			inHunk = true
			// A hunk starts at its new-file line; with a zero count the
			// number refers to the line before the (empty) new range.
			newStart, newCount := parseNewRange(line)
			target := newStart - 1
			if newCount == 0 {
				target = newStart
			}
			if target < pos || target > len(workingLines) {
				return "", fmt.Errorf("the file no longer matches the diff; refresh and try again")
			}
			result = append(result, workingLines[pos:target]...)
			pos = target
			continue
		}
		if !inHunk || line == "" || strings.HasPrefix(line, "\\") {
			continue
		}

		isSelected := i >= selectedStart && i <= selectedEnd

		switch line[0] {
		case '+':
			// '+' lines are exactly what the working tree holds
			if pos >= len(workingLines) || workingLines[pos] != line[1:] {
				return "", fmt.Errorf("the file no longer matches the diff; refresh and try again")
			}
			if !isSelected {
				result = append(result, workingLines[pos])
			}
			pos++
		case '-':
			if isSelected {
				result = append(result, line[1:])
			}
		default:
			// Context line: keep the working tree content
			if pos < len(workingLines) {
				result = append(result, workingLines[pos])
				pos++
			}
		}
	}

	result = append(result, workingLines[pos:]...)
	return strings.Join(result, "\n"), nil
}

// parseNewRange parses the "+start[,count]" part of a hunk header
func parseNewRange(header string) (start, count int) {
	count = 1
	for _, f := range strings.Fields(header)[1:] {
		if !strings.HasPrefix(f, "+") {
			continue
		}
		if i := strings.IndexByte(f, ','); i >= 0 {
			fmt.Sscanf(f[1:i], "%d", &start)
			fmt.Sscanf(f[i+1:], "%d", &count)
		} else {
			fmt.Sscanf(f[1:], "%d", &start)
		}
		break
	}
	return start, count
}
