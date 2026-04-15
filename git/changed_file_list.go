package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileInfo represents a file with its status
type FileInfo struct {
	Path         string
	ChangeStatus string // "added", "deleted", "modified", "untracked"
}

// GetAllTrackedFiles returns all files tracked by git in the repository
func GetAllTrackedFiles(repoPath string) ([]FileInfo, error) {
	cmd := exec.Command("git", "-c", "core.quotepath=false", "ls-files")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		files = append(files, FileInfo{Path: line})
	}
	return files, nil
}

// FindGitRoot searches for the .git directory by traversing up from the current directory
func FindGitRoot(startPath string) (string, error) {
	dir, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the root directory
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}

func GetChangedFiles(repoPath string) ([]FileInfo, []FileInfo, []FileInfo, error) {
	// Find the Git repository root
	gitRoot, err := FindGitRoot(repoPath)
	if err != nil {
		return nil, nil, nil, err
	}
	// Run git status --porcelain to get both staged and unstaged files
	// -c core.quotepath=false prevents escaping of multibyte filenames
	cmd := exec.Command("git", "-c", "core.quotepath=false", "status", "--porcelain")
	cmd.Dir = gitRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, nil, err
	}

	var stagedFiles []FileInfo
	var modifiedFiles []FileInfo
	var untrackedFiles []FileInfo

	// Parse output (remove only empty lines, without using TrimSpace)
	lines := strings.Split(string(output), "\n")
	// Remove empty lines
	var filteredLines []string
	for _, line := range lines {
		if len(line) > 0 {
			filteredLines = append(filteredLines, line)
		}
	}
	lines = filteredLines
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		// git status --porcelain format: XY filename
		// X = index status, Y = worktree status
		indexStatus := line[0]
		worktreeStatus := line[1]
		filename := strings.TrimSpace(line[2:]) // Filename starts after the 2nd character

		// Handle rename case (R  old -> new)
		if indexStatus == 'R' {
			// Split by " -> " to get old and new filenames
			parts := strings.Split(filename, " -> ")
			oldFile := parts[0]
			newFile := parts[1]
			// Treat old file as deleted, new file as added
			stagedFiles = append(stagedFiles, FileInfo{Path: oldFile, ChangeStatus: "deleted"})
			stagedFiles = append(stagedFiles, FileInfo{Path: newFile, ChangeStatus: "added"})
		} else if worktreeStatus == 'R' {
			// Unstaged rename
			parts := strings.Split(filename, " -> ")
			oldFile := parts[0]
			newFile := parts[1]
			modifiedFiles = append(modifiedFiles, FileInfo{Path: oldFile, ChangeStatus: "deleted"})
			modifiedFiles = append(modifiedFiles, FileInfo{Path: newFile, ChangeStatus: "added"})
		} else if indexStatus == '?' && worktreeStatus == '?' {
			// Untracked file
			// Check if it is a directory
			fullPath := filepath.Join(gitRoot, filename)
			fileInfo, statErr := os.Stat(fullPath)
			if statErr == nil && fileInfo.IsDir() {
				// For directories, use git ls-files to quickly list untracked files
				lsCmd := exec.Command("git", "-c", "core.quotepath=false", "ls-files", "--others", "--exclude-standard", filename+"/")
				lsCmd.Dir = gitRoot
				lsOutput, lsErr := lsCmd.Output()
				if lsErr == nil {
					for _, f := range strings.Split(strings.TrimSpace(string(lsOutput)), "\n") {
						if f != "" {
							untrackedFiles = append(untrackedFiles, FileInfo{Path: f, ChangeStatus: "untracked"})
						}
					}
				}
			} else {
				// For files, add as-is
				untrackedFiles = append(untrackedFiles, FileInfo{Path: filename, ChangeStatus: "untracked"})
			}
		} else {
			// Add appropriate info based on status
			if indexStatus == 'A' {
				stagedFiles = append(stagedFiles, FileInfo{Path: filename, ChangeStatus: "added"})
			} else if indexStatus == 'D' {
				stagedFiles = append(stagedFiles, FileInfo{Path: filename, ChangeStatus: "deleted"})
			} else if indexStatus == 'M' {
				stagedFiles = append(stagedFiles, FileInfo{Path: filename, ChangeStatus: "modified"})
			} else if indexStatus != ' ' && indexStatus != '?' {
				stagedFiles = append(stagedFiles, FileInfo{Path: filename, ChangeStatus: "modified"})
			}

			// Unstaged changes
			if worktreeStatus == 'D' {
				modifiedFiles = append(modifiedFiles, FileInfo{Path: filename, ChangeStatus: "deleted"})
			} else if worktreeStatus == 'M' {
				modifiedFiles = append(modifiedFiles, FileInfo{Path: filename, ChangeStatus: "modified"})
			} else if worktreeStatus != ' ' && worktreeStatus != '?' {
				modifiedFiles = append(modifiedFiles, FileInfo{Path: filename, ChangeStatus: "modified"})
			}
		}
	}

	return stagedFiles, modifiedFiles, untrackedFiles, nil
}
