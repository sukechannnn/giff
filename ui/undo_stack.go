package ui

import (
	"fmt"
	"os/exec"
	"strings"
)

// UndoEntry is a snapshot of the git index at a point in time, taken before a
// staging operation. Description is a human-readable label for status messages.
type UndoEntry struct {
	Snapshot    string
	Description string
}

// UndoStack tracks undo/redo history for git index changes using tree snapshots
// produced by `git write-tree`. Undo/redo restore the index with `git read-tree`
// without touching the working tree.
type UndoStack struct {
	repoRoot string
	undo     []UndoEntry
	redo     []UndoEntry
}

func NewUndoStack(repoRoot string) *UndoStack {
	return &UndoStack{repoRoot: repoRoot}
}

// CaptureSnapshot writes the current index to a tree object and returns its SHA.
func (s *UndoStack) CaptureSnapshot() (string, error) {
	if s == nil {
		return "", nil
	}
	cmd := exec.Command("git", "write-tree")
	cmd.Dir = s.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *UndoStack) restoreSnapshot(sha string) error {
	cmd := exec.Command("git", "read-tree", sha)
	cmd.Dir = s.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("read-tree %s: %w: %s", sha, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Push records a pre-operation snapshot. Call this BEFORE performing the
// operation so Undo can restore the prior state. Clears the redo stack.
func (s *UndoStack) Push(snapshot, description string) {
	if s == nil || snapshot == "" {
		return
	}
	s.undo = append(s.undo, UndoEntry{Snapshot: snapshot, Description: description})
	s.redo = nil
}

// Undo restores the index to the snapshot at the top of the undo stack and
// pushes the current state onto the redo stack. Returns the description of
// the operation being undone.
func (s *UndoStack) Undo() (string, error) {
	if s == nil || len(s.undo) == 0 {
		return "", fmt.Errorf("nothing to undo")
	}
	n := len(s.undo) - 1
	entry := s.undo[n]

	current, err := s.CaptureSnapshot()
	if err != nil {
		return "", err
	}

	if err := s.restoreSnapshot(entry.Snapshot); err != nil {
		return "", err
	}

	s.undo = s.undo[:n]
	s.redo = append(s.redo, UndoEntry{Snapshot: current, Description: entry.Description})
	return entry.Description, nil
}

// Redo restores the index to the snapshot at the top of the redo stack and
// pushes the current state onto the undo stack. Returns the description of
// the operation being redone.
func (s *UndoStack) Redo() (string, error) {
	if s == nil || len(s.redo) == 0 {
		return "", fmt.Errorf("nothing to redo")
	}
	n := len(s.redo) - 1
	entry := s.redo[n]

	current, err := s.CaptureSnapshot()
	if err != nil {
		return "", err
	}

	if err := s.restoreSnapshot(entry.Snapshot); err != nil {
		return "", err
	}

	s.redo = s.redo[:n]
	s.undo = append(s.undo, UndoEntry{Snapshot: current, Description: entry.Description})
	return entry.Description, nil
}

func (s *UndoStack) CanUndo() bool {
	return s != nil && len(s.undo) > 0
}

func (s *UndoStack) CanRedo() bool {
	return s != nil && len(s.redo) > 0
}
