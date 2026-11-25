package ui

import (
	"os"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/comments"
)

// The comments left with 'i' show in the diff pane, after the pane is drawn:
// a mark on the line number of the lines that have one, and under the
// cursor's line the comments on it with their answers, as GitHub shows a
// review comment under its code. While a comment is written, blank rows under
// its lines make room for the text area, which lies over them.
//
// The rows added come after the cursor's, and the pane is drawn anew on every
// move, so the rows above the cursor, which the cursor and the selection count
// in, never shift.

// commentEditing is the comment being written: the text area lies over the
// height blank rows after row, after the comments on row when it is a reply
// to them, which take threadRows rows
type commentEditing struct {
	row        int
	height     int
	reply      bool
	threadRows int
}

// commentHosts maps the text views of a diff pane to its context, so any
// updater drawing into them can show the comments
var commentHosts = map[*tview.TextView]*DiffViewContext{}

// registerCommentViews lets the diff pane of ctx show the comments, those
// on what it shows already included
func registerCommentViews(ctx *DiffViewContext) {
	for _, view := range []*tview.TextView{ctx.diffView, ctx.beforeView, ctx.afterView} {
		if view != nil {
			commentHosts[view] = ctx
		}
	}
	// The first file was drawn before the pane was registered
	if ctx.isSplitView != nil && *ctx.isSplitView && ctx.afterView != nil {
		decorateComments(ctx.afterView, -1)
	} else if ctx.diffView != nil {
		decorateComments(ctx.diffView, -1)
	}
}

// commentCache keeps the comments of a repository read until their file
// changes
type commentCache struct {
	mod  time.Time
	file *comments.File
}

var commentCaches = map[string]*commentCache{}

// commentsOf returns the comments of the repository, read again when their
// file changed
func commentsOf(repoRoot string) []comments.Comment {
	path, err := comments.PathFor(repoRoot)
	if err != nil {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		delete(commentCaches, repoRoot)
		return nil
	}
	if c, ok := commentCaches[repoRoot]; ok && c.mod.Equal(info.ModTime()) {
		return c.file.Comments
	}
	f, err := comments.Read(repoRoot)
	if err != nil {
		return nil
	}
	commentCaches[repoRoot] = &commentCache{mod: info.ModTime(), file: f}
	return f.Comments
}

// rowFileLines returns, for every row of the diff pane, the line of the file
// as it is now that the row shows, 0 for a row that shows none: a fold, a '-'
// line
func rowFileLines(ctx *DiffViewContext) []int {
	if inFileView(ctx) {
		lines := make([]int, len(fileViewLines(*ctx.currentDiffText)))
		for i := range lines {
			lines[i] = i + 1
		}
		return lines
	}
	_, newLineMap := createLineNumberMapping(*ctx.currentDiffText)
	if *ctx.isSplitView {
		rows := getCachedSplitContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot).Rows
		lines := make([]int, len(rows))
		for r, row := range rows {
			switch {
			case row.FileLine > 0:
				lines[r] = row.FileLine
			case row.DiffStart >= 0:
				for i := row.DiffStart; i <= row.DiffEnd; i++ {
					if num, ok := newLineMap[i]; ok {
						lines[r] = num
					}
				}
			}
		}
		return lines
	}
	content := getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot).Lines
	lines := make([]int, len(content))
	diffIdx := 0
	for r, line := range content {
		if line.FoldID != "" {
			lines[r] = line.FileLine
			continue
		}
		if num, ok := newLineMap[diffIdx]; ok && line.LineType != '-' {
			lines[r] = num
		}
		diffIdx++
	}
	return lines
}

// commentRows maps the row that shows the last line of each comment on the
// file shown to the comments
func commentRows(ctx *DiffViewContext) map[int][]comments.Comment {
	var mine []comments.Comment
	for _, c := range commentsOf(ctx.repoRoot) {
		if c.Path == *ctx.currentFile {
			mine = append(mine, c)
		}
	}
	if len(mine) == 0 {
		return nil
	}
	lines := rowFileLines(ctx)
	byLine := map[int]int{}
	for r, line := range lines {
		if line > 0 {
			byLine[line] = r
		}
	}
	rows := map[int][]comments.Comment{}
	for _, c := range mine {
		if r, ok := byLine[c.EndLine]; ok {
			rows[r] = append(rows[r], c)
		}
	}
	return rows
}

// decorateComments shows the comments in the diff pane view belongs to,
// once it is drawn, with the cursor on cursorY (-1 when it is not shown)
func decorateComments(view *tview.TextView, cursorY int) {
	ctx, ok := commentHosts[view]
	if !ok || ctx.currentFile == nil || *ctx.currentFile == "" || ctx.currentDiffText == nil {
		return
	}
	views := []*tview.TextView{ctx.diffView}
	main := ctx.diffView
	if *ctx.isSplitView && !inFileView(ctx) {
		views = []*tview.TextView{ctx.beforeView, ctx.afterView}
		main = ctx.afterView
	}
	rows := commentRows(ctx)
	if len(rows) == 0 && ctx.commentEditing == nil {
		return
	}

	texts := make([][]string, len(views))
	for i, v := range views {
		texts[i] = strings.Split(v.GetText(false), "\n")
	}
	mainText := texts[len(texts)-1]

	// The mark on the line numbers
	for r, cs := range rows {
		if r < len(mainText) {
			mainText[r] = strings.Replace(mainText[r], "│", commentMark(cs), 1)
		}
	}

	// What goes under a row: the room for the comment being written, or the
	// comments on the cursor's line
	under, block := -1, []string(nil)
	_, _, width, _ := main.GetInnerRect()
	if e := ctx.commentEditing; e != nil {
		under = e.row
		if e.reply {
			block = commentBlock(rows[e.row], width, false)
		}
		e.threadRows = len(block)
		block = append(block, make([]string, e.height)...)
	} else if cs, ok := rows[cursorY]; ok && cursorY >= 0 && (ctx.leftPaneFocused == nil || !*ctx.leftPaneFocused) {
		under = cursorY
		block = commentBlock(cs, width, true)
	}
	if under >= 0 && under < len(mainText) {
		for i := range texts {
			lines := make([]string, len(block))
			if i == len(texts)-1 {
				copy(lines, block)
			}
			texts[i] = append(texts[i][:under+1], append(lines, texts[i][under+1:]...)...)
		}
	}

	for i, v := range views {
		row, col := v.GetScrollOffset()
		v.SetText(strings.Join(texts[i], "\n"))
		if under >= 0 {
			row = scrollToShow(v, row, under, len(block))
		}
		v.ScrollTo(row, col)
	}
}

// scrollToShow returns the scroll row that shows the rows from under to under
// plus extra, keeping under in sight, from row
func scrollToShow(view *tview.TextView, row, under, extra int) int {
	_, _, _, height := view.GetInnerRect()
	if height <= 0 {
		return row
	}
	if last := under + extra; last >= row+height {
		row = last - height + 1
	}
	return max(min(row, under), 0)
}

// commentMark is the mark on the line number of a line with comments: green
// once they are all answered
func commentMark(cs []comments.Comment) string {
	for _, c := range cs {
		if c.Status != comments.StatusAnswered {
			return "[gold]●[-]"
		}
	}
	return "[forestgreen]●[-]"
}

// commentBlockMaxLines is how many lines an answer shows under its code at
// most; I lists the comments in full
const commentBlockMaxLines = 12

// commentBlock draws the threads of the comments cs as boxes width columns
// wide, to show under the line they are on, with the key to reply when hint
func commentBlock(cs []comments.Comment, width int, hint bool) []string {
	const indent = "  "
	inner := max(width-len(indent)-4, 20)
	var lines []string
	for i, c := range cs {
		color, state := "gold", "waiting for /giff"
		switch c.Status {
		case comments.StatusAnswered:
			color, state = "forestgreen", "answered"
		case comments.StatusInProgress:
			color, state = "gold", "Claude Code is answering"
		}
		head := "┌ " + state + " "
		lines = append(lines, indent+"["+color+"]"+head+strings.Repeat("─", max(inner+2-len([]rune(head)), 0))+"[-]")
		add := func(label, text string, limit int) {
			wrapped := []string{}
			for _, para := range strings.Split(text, "\n") {
				w := tview.WordWrap(tview.Escape(para), inner-3)
				if len(w) == 0 {
					w = []string{""}
				}
				wrapped = append(wrapped, w...)
			}
			if limit > 0 && len(wrapped) > limit {
				wrapped = append(wrapped[:limit], "[gray]… I:comments for the rest[-]")
			}
			for i, w := range wrapped {
				prefix := "   "
				if i == 0 {
					prefix = label
				}
				lines = append(lines, indent+"["+color+"]│[-] "+prefix+w)
			}
		}
		for _, m := range c.Thread {
			if m.From == comments.FromClaude {
				add("[forestgreen]A.[-] ", m.Text, commentBlockMaxLines)
			} else {
				add("[gold]Q.[-] ", m.Text, 0)
			}
		}
		foot := "└"
		// The keys reply to and delete the last comment of the line
		if hint && i == len(cs)-1 {
			if c.Status == comments.StatusAnswered {
				foot += "─ i:reply  D:delete "
			} else {
				foot += "─ D:delete "
			}
		}
		lines = append(lines, indent+"["+color+"]"+foot+strings.Repeat("─", max(inner+2-len([]rune(foot)), 0))+"[-]")
	}
	return lines
}
