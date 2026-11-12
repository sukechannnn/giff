package git

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// GrepFiles counts, per path, how many lines of the tracked files in the
// working tree contain query, compared case-insensitively like GrepDiff.
// Binary files are skipped. Files without a hit are left out, and an empty
// query yields an empty map.
func GrepFiles(repoRoot, query string) (map[string]int, error) {
	hits := make(map[string]int)
	if query == "" {
		return hits, nil
	}

	cmd := exec.Command("git", "-c", "core.quotepath=false", "grep", "-c", "-z", "-I", "-i", "-F", "-e", query)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		// git grep exits with 1 when nothing matches.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return hits, nil
		}
		return nil, err
	}
	return parseGrepCounts(string(output)), nil
}

// parseGrepCounts reads the output of git grep -c -z: a path, a NUL and the
// count of matching lines, one file per line.
func parseGrepCounts(output string) map[string]int {
	hits := make(map[string]int)
	for _, line := range strings.Split(output, "\n") {
		path, count, ok := strings.Cut(line, "\x00")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(count); err == nil && n > 0 {
			hits[path] = n
		}
	}
	return hits
}
