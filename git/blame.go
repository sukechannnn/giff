package git

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// BlameLine is the commit that last changed a line of a file
type BlameLine struct {
	Hash    string
	Author  string
	Time    time.Time
	Summary string
}

// Uncommitted reports whether the line has changes not committed yet
func (b BlameLine) Uncommitted() bool {
	return strings.Trim(b.Hash, "0") == ""
}

// Blame returns, for every line of the file at path in the working tree, the
// commit that last changed it
func Blame(repoRoot, path string) ([]BlameLine, error) {
	cmd := exec.Command("git", "blame", "--porcelain", "--", path)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		// git says why, such as for a file it does not track
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, errors.New(strings.TrimSpace(strings.TrimPrefix(string(exitErr.Stderr), "fatal: ")))
		}
		return nil, err
	}
	return parseBlamePorcelain(string(output)), nil
}

// parseBlamePorcelain reads the output of git blame --porcelain. A commit's
// author and summary are given at its first line only.
func parseBlamePorcelain(output string) []BlameLine {
	commits := map[string]*BlameLine{}
	var lines []BlameLine
	var current *BlameLine
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "\t"):
			// The content of the line ends its entry
			if current != nil {
				lines = append(lines, *current)
			}
			current = nil
		case current == nil:
			fields := strings.Fields(line)
			if len(fields) < 3 || len(fields[0]) != 40 {
				continue
			}
			c, ok := commits[fields[0]]
			if !ok {
				c = &BlameLine{Hash: fields[0]}
				commits[fields[0]] = c
			}
			current = c
		default:
			key, value, _ := strings.Cut(line, " ")
			switch key {
			case "author":
				current.Author = value
			case "author-time":
				if sec, err := strconv.ParseInt(value, 10, 64); err == nil {
					current.Time = time.Unix(sec, 0)
				}
			case "summary":
				current.Summary = value
			}
		}
	}
	return lines
}
