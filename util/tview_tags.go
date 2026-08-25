package util

import (
	"regexp"
	"strings"
)

// Regexes matching only what tview actually recognizes as tags, so literal
// brackets in code (e.g. shell's `[ -n "$VAR" ]` or Go's `[]string`) are left
// untouched.
var (
	// Style tag: [fg:bg:attrs:url] where fg/bg are a color name, #rrggbb, or
	// "-", attrs are flag letters or "-", and every part is optional — but a
	// bare "[]" is not a tag.
	tviewStyleTagRegex = regexp.MustCompile(`^\[(?:(?:-|#[0-9a-fA-F]{6}|[a-zA-Z][a-zA-Z0-9]*)(?::(?:-|#[0-9a-fA-F]{6}|[a-zA-Z][a-zA-Z0-9]*)?(?::(?:-|[buildsrBUILDSR]+)?(?::[^\[\]]*)?)?)?|:(?:-|#[0-9a-fA-F]{6}|[a-zA-Z][a-zA-Z0-9]*)?(?::(?:-|[buildsrBUILDSR]+)?(?::[^\[\]]*)?)?)\]`)
	// Region tag: ["name"]
	tviewRegionTagRegex = regexp.MustCompile(`^\["[a-zA-Z0-9_,;: \-\.]*"\]`)
	// Escaped tag produced by tview.Escape: "[xyz[]" is displayed as "[xyz]"
	tviewEscapedTagRegex = regexp.MustCompile(`^\[[^\[\]]+\[+\]`)
)

// MatchTviewStyleTag returns the style tag at the start of s, or "" if none.
func MatchTviewStyleTag(s string) string {
	return tviewStyleTagRegex.FindString(s)
}

// MatchTviewRegionTag returns the region tag at the start of s, or "" if none.
func MatchTviewRegionTag(s string) string {
	return tviewRegionTagRegex.FindString(s)
}

// MatchTviewTag returns the style or region tag at the start of s, or "" if s
// does not start with one.
func MatchTviewTag(s string) string {
	if m := tviewStyleTagRegex.FindString(s); m != "" {
		return m
	}
	return tviewRegionTagRegex.FindString(s)
}

// MatchTviewEscapedTag returns the escaped tag (e.g. "[xyz[]") at the start of
// s, or "" if none.
func MatchTviewEscapedTag(s string) string {
	return tviewEscapedTagRegex.FindString(s)
}

// StripTviewTags returns the text as tview displays it: style/region tags are
// removed and escaped tags ("[xyz[]") are unescaped back to "[xyz]".
func StripTviewTags(text string) string {
	var b strings.Builder
	for len(text) > 0 {
		if text[0] == '[' {
			if m := MatchTviewTag(text); m != "" {
				text = text[len(m):]
				continue
			}
			if m := MatchTviewEscapedTag(text); m != "" {
				// Drop one "[" from the closing run: "[xyz[]" -> "[xyz]"
				b.WriteString(m[:len(m)-2])
				b.WriteByte(']')
				text = text[len(m):]
				continue
			}
		}
		b.WriteByte(text[0])
		text = text[1:]
	}
	return b.String()
}
