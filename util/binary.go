package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// binarySniffLen is the number of leading bytes inspected for NUL bytes
// when deciding whether a file is binary (same heuristic as git).
const binarySniffLen = 8000

// IsBinaryFile reports whether the file looks binary by checking its
// leading bytes for NUL, without reading the whole file into memory.
func IsBinaryFile(filePath string, repoRoot string) (bool, error) {
	fullPath := filepath.Join(repoRoot, filePath)
	f, err := os.Open(fullPath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, binarySniffLen)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return false, err
	}
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

// IsBinaryDiff reports whether git diff output describes a binary file
// change. Body lines always start with ' ', '+', or '-', so a raw
// "Binary files ... differ" line can only be git's own marker.
func IsBinaryDiff(diffText string) bool {
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "Binary files ") && strings.HasSuffix(line, " differ") {
			return true
		}
		if line == "GIT binary patch" {
			return true
		}
	}
	return false
}

// FormatBinaryNotice returns the text shown in the diff view instead of a
// binary file's contents.
func FormatBinaryNotice(filePath string) string {
	return fmt.Sprintf("Binary file: %s (content not displayed)\n", filePath)
}
