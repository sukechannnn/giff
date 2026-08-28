package git

import (
	"os/exec"
	"strings"

	"github.com/sukechannnn/giff/util"
)

// Stage statuses used as the first half of a DiffGrepKey. They match the
// StageStatus values the file list assigns to each section.
const (
	GrepStageStaged    = "staged"
	GrepStageUnstaged  = "unstaged"
	GrepStageUntracked = "untracked"
)

// DiffGrepKey builds the key used to look up a file's hit count. Paths alone
// are ambiguous because the same file can appear in both the staged and the
// unstaged section with different changes.
func DiffGrepKey(stageStatus, path string) string {
	return stageStatus + ":" + path
}

// GrepDiff counts how many changed lines of each pending file contain query.
// Only added and removed lines are searched, so context lines never produce a
// hit; untracked files have no diff and are matched against their whole
// content instead. Matching is case-insensitive substring matching.
//
// The returned map is keyed by DiffGrepKey and only holds files with at least
// one hit. An empty query yields an empty map.
func GrepDiff(repoRoot, query string, untrackedFiles []FileInfo, ignoreWhitespace bool) (map[string]int, error) {
	hits := make(map[string]int)
	if query == "" {
		return hits, nil
	}
	needle := strings.ToLower(query)

	collect := func(stage string, extraArgs ...string) error {
		output, err := runDiffForGrep(repoRoot, ignoreWhitespace, extraArgs...)
		if err != nil {
			return err
		}
		for path, lines := range parseChangedLines(output) {
			if n := countMatches(lines, needle); n > 0 {
				hits[DiffGrepKey(stage, path)] = n
			}
		}
		return nil
	}

	if err := collect(GrepStageStaged, "--cached"); err != nil {
		return nil, err
	}
	if err := collect(GrepStageUnstaged); err != nil {
		return nil, err
	}

	for _, fileInfo := range untrackedFiles {
		isBinary, err := util.IsBinaryFile(fileInfo.Path, repoRoot)
		if err != nil || isBinary {
			continue
		}
		content, err := util.ReadFileContent(fileInfo.Path, repoRoot)
		if err != nil {
			continue
		}
		if n := countMatches(strings.Split(content, "\n"), needle); n > 0 {
			hits[DiffGrepKey(GrepStageUntracked, fileInfo.Path)] = n
		}
	}

	return hits, nil
}

// runDiffForGrep runs git diff in a form meant to be parsed: no external diff
// driver, no context lines, and fixed a/ b/ path prefixes regardless of the
// user's diff.noprefix / diff.mnemonicPrefix settings.
func runDiffForGrep(repoRoot string, ignoreWhitespace bool, extraArgs ...string) (string, error) {
	args := []string{
		"-c", "core.quotepath=false",
		"-c", "diff.noprefix=false",
		"-c", "diff.mnemonicPrefix=false",
		"diff", "--no-ext-diff", "-U0",
	}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = append(args, extraArgs...)

	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// parseChangedLines extracts the added and removed lines of every file in a
// unified diff, keyed by file path and stripped of their +/- marker.
func parseChangedLines(diffText string) map[string][]string {
	result := make(map[string][]string)

	var oldPath, currentPath string
	inHunk := false

	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			oldPath, currentPath = "", ""
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk && strings.HasPrefix(line, "--- "):
			oldPath = diffHeaderPath(line[len("--- "):])
		case !inHunk && strings.HasPrefix(line, "+++ "):
			// A deleted file has "+++ /dev/null", so fall back to the old path.
			currentPath = diffHeaderPath(line[len("+++ "):])
			if currentPath == "" {
				currentPath = oldPath
			}
		case inHunk && currentPath != "" && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")):
			result[currentPath] = append(result[currentPath], line[1:])
		}
	}

	return result
}

// diffHeaderPath turns the value of a "--- a/path" / "+++ b/path" header into
// a repository-relative path, returning "" for /dev/null.
func diffHeaderPath(value string) string {
	// Git appends a tab plus timestamp for some diff formats.
	if i := strings.IndexByte(value, '\t'); i >= 0 {
		value = value[:i]
	}
	if value == "/dev/null" {
		return ""
	}
	if strings.HasPrefix(value, "a/") || strings.HasPrefix(value, "b/") {
		return value[2:]
	}
	return value
}

// countMatches returns how many of lines contain lowerQuery, compared
// case-insensitively. lowerQuery must already be lower-cased.
func countMatches(lines []string, lowerQuery string) int {
	count := 0
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			count++
		}
	}
	return count
}
