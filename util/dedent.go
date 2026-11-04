package util

import "strings"

// Dedent removes the indentation every non-blank line shares, so the least
// indented line starts at column zero. The shared indentation is the longest
// run of leading spaces and tabs common to those lines, so a line indented
// with tabs is never cut into by one indented with spaces. Blank lines don't
// count, and lose whatever whitespace they have.
func Dedent(lines []string) []string {
	common := ""
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if !found {
			common, found = indent, true
			continue
		}
		i := 0
		for i < len(common) && i < len(indent) && common[i] == indent[i] {
			i++
		}
		common = common[:i]
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		result[i] = strings.TrimPrefix(line, common)
	}
	return result
}
