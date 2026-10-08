package commands

import (
	"fmt"
	"strings"

	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// CommandDParams contains parameters for commandD function
type CommandDParams struct {
	CurrentFile   string
	CurrentStatus string
	RepoRoot      string
}

// CommandD handles the 'd' command for discarding changes
func CommandD(params CommandDParams) error {
	// Notify that discarding staged changes is not supported
	if params.CurrentStatus == "staged" {
		return fmt.Errorf("Cannot discard staged changes. Use 'a' to unstage first.")
	}

	// No file selected
	if params.CurrentFile == "" {
		return fmt.Errorf("No file selected")
	}

	// Delete the file if it is untracked
	if params.CurrentStatus == "untracked" {
		if err := git.DeleteUntrackedFile(params.CurrentFile, params.RepoRoot); err != nil {
			return fmt.Errorf("failed to delete %s: %w", params.CurrentFile, err)
		}
		return nil
	}

	// Discard changes via git checkout for unstaged files
	if err := git.DiscardFileChanges(params.CurrentFile, params.RepoRoot); err != nil {
		return fmt.Errorf("discard changes for %s failed: %w", params.CurrentFile, err)
	}

	return nil
}

// DiscardSelectedLinesParams contains parameters for DiscardSelectedLines
type DiscardSelectedLinesParams struct {
	SelectStart     int // Diff display index (headers excluded) of the first selected line
	SelectEnd       int // Diff display index (headers excluded) of the last selected line
	CurrentFile     string
	CurrentDiffText string // Unstaged diff of CurrentFile
	RepoRoot        string
}

// DiscardSelectedLines reverts the selected changes of an unstaged diff in the
// working tree, keeping the rest of the changes and the index untouched
func DiscardSelectedLines(params DiscardSelectedLinesParams) error {
	if params.CurrentFile == "" {
		return fmt.Errorf("No file selected")
	}
	start, end := params.SelectStart, params.SelectEnd
	if start > end {
		start, end = end, start
	}
	mapping := MapDisplayToOriginalIdx(params.CurrentDiffText)
	origStart, okStart := mapping[start]
	origEnd, okEnd := mapping[end]
	if !okStart || !okEnd {
		return fmt.Errorf("Selection is out of the diff")
	}
	if err := git.DiscardSelectedChanges(params.CurrentFile, params.RepoRoot, params.CurrentDiffText, origStart, origEnd); err != nil {
		return fmt.Errorf("Failed to discard changes: %w", err)
	}
	return nil
}

// HasChangesInRange reports whether diff display lines start..end (headers
// excluded) contain a '+' or '-' line
func HasChangesInRange(diffText string, start, end int) bool {
	if start < 0 || end < 0 {
		return false
	}
	if start > end {
		start, end = end, start
	}
	mapping := MapDisplayToOriginalIdx(diffText)
	diffLines := util.SplitLines(diffText)
	for i := start; i <= end; i++ {
		idx, ok := mapping[i]
		if !ok || idx >= len(diffLines) {
			continue
		}
		if strings.HasPrefix(diffLines[idx], "+") || strings.HasPrefix(diffLines[idx], "-") {
			return true
		}
	}
	return false
}
