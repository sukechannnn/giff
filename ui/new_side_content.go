package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sukechannnn/giff/git"
)

// newSideContent returns the file a diff's new side is: the blob its index
// line names, such as the staged file for a staged diff or the commit's for
// a commit, or the file in the working tree when the repository does not
// have the blob, as for a diff of the working tree. Folds show its lines.
func newSideContent(diffText, filePath, repoRoot string) (string, bool) {
	if filePath == "" || repoRoot == "" {
		return "", false
	}
	if hash := newBlobHash(diffText); hash != "" {
		if content, ok := git.ReadBlob(repoRoot, hash); ok {
			return content, true
		}
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filePath))
	if err != nil {
		return "", false
	}
	return string(content), true
}

// newBlobHash returns the hash of the new side's blob on the index line of
// diffText's header, or "" when there is none or the file is deleted
func newBlobHash(diffText string) string {
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "@@") {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "index" {
			continue
		}
		_, hash, ok := strings.Cut(fields[1], "..")
		if !ok || strings.Trim(hash, "0") == "" {
			return ""
		}
		return hash
	}
	return ""
}
