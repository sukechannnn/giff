package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
)

// blameAuthorWidth is how many columns the author takes in the blame gutter
const blameAuthorWidth = 12

// blameGutterWidth is the width of the blame gutter: the hash, the date and
// the author
const blameGutterWidth = 7 + 1 + 10 + 1 + blameAuthorWidth

// blameGutter formats the blame of every line for the left of the file view.
// A commit shows on the first of the lines in a row that it changed, and the
// others are left blank.
func blameGutter(lines []git.BlameLine) []string {
	blank := strings.Repeat(" ", blameGutterWidth)
	gutter := make([]string, len(lines))
	for i, l := range lines {
		switch {
		case i > 0 && lines[i-1].Hash == l.Hash:
			gutter[i] = blank
		case l.Uncommitted():
			gutter[i] = "[gold]" + fitWidth("Not committed yet", blameGutterWidth) + "[-]"
		default:
			gutter[i] = fmt.Sprintf("[gold]%s[-] [dimgray]%s[-] %s",
				l.Hash[:7], l.Time.Format("2006-01-02"), tview.Escape(fitWidth(l.Author, blameAuthorWidth)))
		}
	}
	return gutter
}

// blameStatus describes the commit of a line for the status bar
func blameStatus(l git.BlameLine) string {
	if l.Uncommitted() {
		return "[gold]Not committed yet[-]"
	}
	return fmt.Sprintf("[gold]%s[-] [white]%s[-] [dimgray]%s[-]  %s",
		l.Hash[:7], tview.Escape(l.Author), l.Time.Format("2006-01-02 15:04"), tview.Escape(l.Summary))
}

// fitWidth cuts s to width columns, or pads it with spaces to them
func fitWidth(s string, width int) string {
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := tview.TaggedStringWidth(tview.Escape(string(r)))
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + strings.Repeat(" ", width-used)
}
