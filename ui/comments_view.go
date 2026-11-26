package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/comments"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// commentCodeMaxLines is how many lines of code a comment carries at most
const commentCodeMaxLines = 200

// commentTarget returns the comment the cursor's line, or the lines selected with
// V, would get: the file, its lines and their code as shown. ok is false
// when they are no line of the file, such as a folded row only.
func commentTarget(ctx *DiffViewContext) (n comments.Comment, ok bool) {
	if *ctx.currentFile == "" {
		return n, false
	}
	start, end := *ctx.cursorY, *ctx.cursorY
	if *ctx.isSelecting && *ctx.selectStart >= 0 && *ctx.selectEnd >= 0 {
		start, end = *ctx.selectStart, *ctx.selectEnd
	}
	if start > end {
		start, end = end, start
	}
	if inFileView(ctx) {
		lines := fileViewLines(*ctx.currentDiffText)
		start, end = max(start, 0), min(end, len(lines)-1)
		if start > end {
			return n, false
		}
		n.StartLine, n.EndLine = start+1, end+1
	} else {
		// The lines the rows show; rows of '-' lines only fall back to the
		// nearest line that still exists
		n.StartLine, n.EndLine = -1, -1
		lines := rowFileLines(ctx)
		for r := max(start, 0); r <= end && r < len(lines); r++ {
			if line := lines[r]; line > 0 {
				if n.StartLine < 0 || line < n.StartLine {
					n.StartLine = line
				}
				n.EndLine = max(n.EndLine, line)
			}
		}
		if n.StartLine < 0 {
			n.StartLine, n.EndLine = diffRowsFileLineRange(ctx, start, end)
		}
		if n.StartLine < 0 {
			return n, false
		}
	}
	n.Path = *ctx.currentFile
	n.Code = commentCode(ctx, start, end)
	n.View = "diff"
	if inFileView(ctx) {
		n.View = "file"
	} else if ctx.commentView != nil {
		n.View = ctx.commentView()
	}
	return n, true
}

// commentCode returns the rows start to end of the pane as plain text: the
// file's lines, the diff's lines with their '+' and '-', or in split view the
// lines after the change, with those before it when they differ
func commentCode(ctx *DiffViewContext, start, end int) string {
	var lines []string
	switch {
	case inFileView(ctx):
		all := fileViewLines(*ctx.currentDiffText)
		lines = all[max(start, 0):min(end+1, len(all))]
	case *ctx.isSplitView:
		content := getCachedSplitContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
		var before, after []string
		for i := max(start, 0); i <= end && i < len(content.Rows); i++ {
			if content.Rows[i].IsFoldIndicator {
				continue
			}
			if b := stripTviewTags(content.BeforeLines[i]); strings.TrimSpace(b) != "" {
				before = append(before, b)
			}
			if a := stripTviewTags(content.AfterLines[i]); strings.TrimSpace(a) != "" {
				after = append(after, a)
			}
		}
		lines = after
		if strings.Join(before, "\n") != strings.Join(after, "\n") && len(before) > 0 {
			lines = append(append([]string{"Before:"}, before...), append([]string{"After:"}, after...)...)
		}
	default:
		content := getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
		for i := max(start, 0); i <= end && i < len(content.Lines); i++ {
			if !content.Lines[i].IsFoldIndicator {
				lines = append(lines, stripTviewTags(content.Lines[i].Content))
			}
		}
	}
	if len(lines) > commentCodeMaxLines {
		lines = append(lines[:commentCodeMaxLines:commentCodeMaxLines], fmt.Sprintf("... %d more lines", len(lines)-commentCodeMaxLines))
	}
	return strings.Join(lines, "\n")
}

// addComment opens a text area under the cursor's line, or the lines
// selected, for a question or a request on them, and keeps it for Claude
// Code to answer with `giff comment`
func addComment(ctx *DiffViewContext) {
	// On a line with a comment, i opens it under the line, or closes it
	if !*ctx.isSelecting && len(commentRows(ctx)[*ctx.cursorY]) > 0 {
		toggleCommentAtCursor(ctx)
		return
	}
	target, ok := commentTarget(ctx)
	if !ok {
		ctx.updateGlobalStatus("Put the cursor on a line of the file to leave a comment", "tomato")
		return
	}
	row := *ctx.cursorY
	if *ctx.isSelecting && *ctx.selectStart >= 0 && *ctx.selectEnd >= 0 {
		row = max(*ctx.selectStart, *ctx.selectEnd)
	}
	title := fmt.Sprintf(" Comment on %s  (Option+Enter:save  Esc:cancel) ", tview.Escape(target.Location()))
	showCommentInput(ctx, title, row, func(question string) {
		target.Question = question
		saved, err := comments.Add(ctx.repoRoot, target)
		if err != nil {
			ctx.updateGlobalStatus("Failed to save the comment: "+err.Error(), "tomato")
			return
		}
		*ctx.isSelecting = false
		*ctx.selectStart, *ctx.selectEnd = -1, -1
		// The comment stays open under its lines, as it was written there
		for r, cs := range commentRows(ctx) {
			for _, c := range cs {
				if c.ID == saved.ID {
					*ctx.cursorY = r
					ctx.commentOpen, ctx.commentOpenRow = true, r
				}
			}
		}
		ctx.updateGlobalStatus("Comment left on "+tview.Escape(saved.Location())+" for /giff in Claude Code  I:comments", "forestgreen")
	})
}

// closeModal puts the main view back and the focus where it was, the file
// list or the diff pane
func closeModal(ctx *DiffViewContext, focus tview.Primitive) {
	ctx.app.SetRoot(ctx.mainView, true)
	ctx.app.SetFocus(focus)
}

// showModal puts box over the main view, centred, height rows high
func showModal(ctx *DiffViewContext, name string, box tview.Primitive, height int, focus tview.Primitive) {
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(box, height, 0, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)
	pages := tview.NewPages().
		AddPage("main", ctx.mainView, true, true).
		AddPage(name, modal, true, true)
	ctx.app.SetRoot(pages, true)
	ctx.app.SetFocus(focus)
}

// deleteCommentAtCursor deletes the comment on the cursor's line, the last
// of them when it has several
func deleteCommentAtCursor(ctx *DiffViewContext) {
	cs := commentRows(ctx)[*ctx.cursorY]
	if len(cs) == 0 {
		ctx.updateGlobalStatus("No comment on this line", "tomato")
		return
	}
	c := cs[len(cs)-1]
	if err := comments.Remove(ctx.repoRoot, c.ID); err != nil {
		ctx.updateGlobalStatus("Failed to delete the comment: "+err.Error(), "tomato")
		return
	}
	redrawDiffPane(ctx)
	ctx.updateGlobalStatus("Deleted the comment on "+tview.Escape(c.Location()), "forestgreen")
}

// commentInputHeight is how many rows the text area of a comment takes
// under its lines, its border included
const commentInputHeight = 7

// redrawDiffPane draws the diff pane again as it is
func redrawDiffPane(ctx *DiffViewContext) {
	if ctx.viewUpdater != nil {
		ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
	}
}

// showCommentInput opens a text area under row of the diff pane, as GitHub
// does under the lines of a review comment, and calls save with its text on
// Option+Enter
func showCommentInput(ctx *DiffViewContext, title string, row int, save func(string)) {
	focus := ctx.app.GetFocus()
	background := util.ModalBackgroundColor.ToTcellColor()

	input := tview.NewTextArea().SetPlaceholder("Ask Claude Code about these lines, or ask it to change them")
	input.SetTextStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	input.SetPlaceholderStyle(tcell.StyleDefault.Foreground(util.PlaceholderColor.ToTcellColor()).Background(background))
	input.SetBackgroundColor(background)
	// Gold, as the comment it becomes is until it is answered
	gold := tcell.GetColor("gold")
	input.SetBorder(true).SetBorderColor(gold).SetTitleColor(gold).SetTitle(title).SetTitleAlign(tview.AlignLeft)

	// Room under the lines, which the text area lies over
	editing := &commentEditing{row: row, height: commentInputHeight}
	ctx.commentEditing = editing
	redrawDiffPane(ctx)
	closeInput := func() {
		ctx.commentEditing = nil
		closeModal(ctx, focus)
	}
	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc:
			closeInput()
			redrawDiffPane(ctx)
			return nil
		case isAltEnter(event):
			text := strings.TrimSpace(input.GetText())
			if text == "" {
				ctx.updateGlobalStatus("The comment is empty", "tomato")
				return nil
			}
			closeInput()
			save(text)
			redrawDiffPane(ctx)
			return nil
		}
		return event
	})

	anchored := &anchoredBox{Box: tview.NewBox(), inner: input, place: func() (int, int, int, int) {
		view := ctx.diffView
		x, y, width, height := view.GetInnerRect()
		if *ctx.isSplitView && !inFileView(ctx) {
			view = ctx.afterView
			bx, _, _, _ := ctx.beforeView.GetInnerRect()
			ax, ay, aw, ah := ctx.afterView.GetInnerRect()
			x, y, width, height = bx, ay, ax+aw-bx, ah
		}
		offset, _ := view.GetScrollOffset()
		top := y + row + 1 - offset
		rows := min(commentInputHeight, y+height-top)
		return x + 2, top, max(width-2, 10), max(rows, 3)
	}}
	pages := tview.NewPages().
		AddPage("main", ctx.mainView, true, true).
		AddPage("comment", anchored, true, true)
	ctx.app.SetRoot(pages, true)
	ctx.app.SetFocus(input)
}

// anchoredBox draws inner at the place place gives, worked out anew on every
// draw, such as under a row of the diff pane however the screen is resized
type anchoredBox struct {
	*tview.Box
	inner tview.Primitive
	place func() (x, y, width, height int)
}

func (a *anchoredBox) Draw(screen tcell.Screen) {
	a.inner.SetRect(a.place())
	a.inner.Draw(screen)
}

func (a *anchoredBox) Focus(delegate func(p tview.Primitive)) { delegate(a.inner) }

func (a *anchoredBox) HasFocus() bool { return a.inner.HasFocus() }

func (a *anchoredBox) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return a.inner.InputHandler()
}

func (a *anchoredBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	return a.inner.MouseHandler()
}

// commentStatusLabel is how a comment's state shows in the list of comments
func commentStatusLabel(n comments.Comment) string {
	switch n.Status {
	case comments.StatusAnswered:
		return "[forestgreen]answered[-]"
	case comments.StatusInProgress:
		return "[gold]answering[-]"
	}
	return "[gray]waiting[-]"
}

// showComments lists the comments of the repository, the newest first, with the
// question, the code and the answer of the one selected. Enter shows its
// lines, d deletes it and Esc closes the list.
func showComments(ctx *DiffViewContext) {
	focus := ctx.app.GetFocus()
	f, err := comments.Read(ctx.repoRoot)
	if err != nil {
		ctx.updateGlobalStatus("Failed to read the comments: "+err.Error(), "tomato")
		return
	}
	if len(f.Comments) == 0 {
		ctx.updateGlobalStatus("No comments yet: i on a line leaves one", "tomato")
		return
	}
	list := f.Comments
	// The newest first
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}

	background := util.ModalBackgroundColor.ToTcellColor()
	border := tcell.GetColor(string(util.CommitAreaBorderColor))

	detail := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	detail.SetBackgroundColor(background)
	detail.SetBorder(true).SetBorderColor(border)
	showDetail := func(i int) {
		n := list[i]
		var b strings.Builder
		fmt.Fprintf(&b, "[white]%s[-]  [gray]%s, %s[-]\n\n", tview.Escape(n.Location()), tview.Escape(n.View), n.CreatedAt.Format("2006-01-02 15:04"))
		fmt.Fprintf(&b, "[gray]%s[-]\n\n", tview.Escape(n.Code))
		for _, m := range n.Thread {
			if m.From == comments.FromClaude {
				fmt.Fprintf(&b, "[forestgreen]A.[-] %s\n\n", tview.Escape(m.Text))
			} else {
				fmt.Fprintf(&b, "[gold]Q.[-] %s\n\n", tview.Escape(m.Text))
			}
		}
		switch n.Status {
		case comments.StatusAnswered:
		case comments.StatusInProgress:
			b.WriteString("[gold]Claude Code is answering...[-]\n")
		default:
			b.WriteString("[gray]Not picked up yet: run /giff in Claude Code, in this repository[-]\n")
		}
		detail.SetText(b.String())
		detail.ScrollToBeginning()
	}

	picker := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	picker.SetBackgroundColor(background)
	picker.SetMainTextStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	picker.SetSelectedBackgroundColor(tcell.GetColor(string(util.CursorLineColor)))
	fill := func() {
		picker.Clear()
		for _, n := range list {
			question, _, _ := strings.Cut(n.Question, "\n")
			picker.AddItem(fmt.Sprintf("%s  %s  %s", commentStatusLabel(n), tview.Escape(n.Location()), tview.Escape(question)), "", 0, nil)
		}
	}
	fill()
	picker.SetChangedFunc(func(i int, _, _ string, _ rune) { showDetail(i) })
	picker.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		closeModal(ctx, focus)
		if ctx.jumpTo == nil {
			ctx.updateGlobalStatus("Cannot show "+tview.Escape(list[i].Location())+" here", "tomato")
			return
		}
		ctx.jumpTo(git.Location{Path: list[i].Path, Line: list[i].StartLine})
	})
	picker.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc, event.Key() == tcell.KeyRune && (keyRune(event) == 'q' || keyRune(event) == 'I'):
			closeModal(ctx, focus)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case event.Key() == tcell.KeyRune && keyRune(event) == 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case event.Key() == tcell.KeyCtrlE:
			row, col := detail.GetScrollOffset()
			detail.ScrollTo(row+1, col)
			return nil
		case event.Key() == tcell.KeyCtrlY:
			row, col := detail.GetScrollOffset()
			detail.ScrollTo(max(row-1, 0), col)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'i':
			closeModal(ctx, focus)
			showCommentChat(ctx, list[picker.GetCurrentItem()].ID)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'D':
			i := picker.GetCurrentItem()
			if err := comments.Remove(ctx.repoRoot, list[i].ID); err != nil {
				ctx.updateGlobalStatus("Failed to delete the comment: "+err.Error(), "tomato")
				return nil
			}
			list = append(list[:i], list[i+1:]...)
			if len(list) == 0 {
				closeModal(ctx, focus)
				ctx.updateGlobalStatus("Deleted the last comment", "forestgreen")
				return nil
			}
			fill()
			picker.SetCurrentItem(min(i, len(list)-1))
			showDetail(picker.GetCurrentItem())
			return nil
		}
		return event
	})
	showDetail(0)

	listRows := min(len(list), 8)
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(picker, listRows, 0, true).
		AddItem(detail, 0, 1, false)
	box.SetBackgroundColor(background)
	box.SetBorder(true).SetBorderColor(border).
		SetTitle(fmt.Sprintf(" %s  (j/k:select  Enter:show  i:chat  D:delete  C-e/C-y:scroll  Esc:close) ", plural(len(list), "comment")))
	_, _, _, height := ctx.mainView.GetRect()
	showModal(ctx, "comments", box, max(height-4, listRows+10), picker)
}

// commentAnswerPollInterval is how often giff looks for answers to the comments
const commentAnswerPollInterval = 2 * time.Second

// watchCommentAnswers tells, through notify, of the comments of the
// repository at repoRoot that Claude Code answers while giff runs, and calls
// changed whenever the comments change, by giff or by Claude Code
func watchCommentAnswers(app *tview.Application, repoRoot string, notify func(string), changed func()) {
	path, err := comments.PathFor(repoRoot)
	if err != nil {
		return
	}
	// When each comment was last answered: a reply answered again is news too
	answered := map[string]time.Time{}
	var lastMod time.Time
	first := true
	for {
		if info, err := os.Stat(path); err == nil && !info.ModTime().Equal(lastMod) {
			lastMod = info.ModTime()
			if f, err := comments.Read(repoRoot); err == nil {
				var fresh []comments.Comment
				for _, n := range f.Comments {
					if n.Status != comments.StatusAnswered || n.AnsweredAt == nil {
						continue
					}
					if at, ok := answered[n.ID]; !ok || n.AnsweredAt.After(at) {
						answered[n.ID] = *n.AnsweredAt
						fresh = append(fresh, n)
					}
				}
				// Those answered before giff started are not news
				message := ""
				if len(fresh) > 0 && !first {
					message = "Claude Code answered the comment on " + tview.Escape(fresh[0].Location())
					if len(fresh) > 1 {
						message += fmt.Sprintf(" and %d more", len(fresh)-1)
					}
				}
				if !first {
					// The marks and the comments shown change with the file,
					// wherever it was changed from
					app.QueueUpdateDraw(func() {
						changed()
						if message != "" {
							notify(message + "  I:comments")
						}
					})
				}
			}
		}
		first = false
		time.Sleep(commentAnswerPollInterval)
	}
}

// plural is n things, as "1 comment" or "2 comments"
func plural(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return fmt.Sprintf("%d %ss", n, thing)
}
