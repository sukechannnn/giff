package git

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// Location is a line of a file in the repository
type Location struct {
	Path string // relative to the repository root
	Line int    // 1-based
	Text string // the line itself
}

// definitionPatterns match a line that defines the name substituted for NAME,
// in the languages giff is most often pointed at. They only look at the start
// of the line, so a call or a mention of the name never counts.
var definitionPatterns = []string{
	// Go
	`^\s*func\s+(\([^)]*\)\s*)?NAME\b`,
	`^\s*(type|var|const)\s+NAME\b`,
	// Python, Ruby
	`^\s*(async\s+)?def\s+(self\.)?NAME\b`,
	`^\s*(class|module)\s+NAME\b`,
	// JavaScript, TypeScript
	`^\s*(export\s+)?(default\s+)?(async\s+)?function\s*\*?\s*NAME\b`,
	`^\s*(export\s+)?(default\s+)?(abstract\s+)?(class|interface|type|enum)\s+NAME\b`,
	`^\s*(export\s+)?(const|let|var)\s+NAME\b`,
	// Rust
	`^\s*(pub(\([^)]*\))?\s+)?(async\s+)?(fn|struct|enum|trait|type|const|static|mod)\s+NAME\b`,
}

// definitionRegexp returns the regexp of a line defining name
func definitionRegexp(name string) *regexp.Regexp {
	alternatives := make([]string, len(definitionPatterns))
	for i, p := range definitionPatterns {
		alternatives[i] = strings.ReplaceAll(p, "NAME", regexp.QuoteMeta(name))
	}
	return regexp.MustCompile("(?:" + strings.Join(alternatives, ")|(?:") + ")")
}

// FindDefinitions returns the lines of the working tree that look like they
// define name: git grep finds the lines the word is on, tracked and untracked
// files alike, and definitionPatterns keep the ones that define it.
func FindDefinitions(repoRoot, name string) ([]Location, error) {
	if name == "" {
		return nil, nil
	}
	cmd := exec.Command("git", "grep", "--untracked", "-I", "-z", "-n", "-w", "-F", "-e", name)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		// git grep exits with 1 when nothing matches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parseDefinitions(output, definitionRegexp(name)), nil
}

// parseDefinitions picks the lines matching def out of git grep -z -n
// output, whose lines read "path\x00line\x00text"
func parseDefinitions(output []byte, def *regexp.Regexp) []Location {
	var locations []Location
	for _, line := range bytes.Split(output, []byte("\n")) {
		fields := bytes.SplitN(line, []byte{0}, 3)
		if len(fields) != 3 || !def.Match(fields[2]) {
			continue
		}
		n, err := strconv.Atoi(string(fields[1]))
		if err != nil {
			continue
		}
		locations = append(locations, Location{Path: string(fields[0]), Line: n, Text: string(fields[2])})
	}
	return locations
}
