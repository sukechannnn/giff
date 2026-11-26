package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/comments"
	"github.com/sukechannnn/giff/util"
)

// commentChatRefresh, while a chat is open, draws it again with the comments
// as they are now, as Claude Code answers
var commentChatRefresh func()

// commentChatCodeMaxLines is how many lines of the code the chat shows above
// the exchange
const commentChatCodeMaxLines = 8

// commentChatText is the exchange of c as a chat: the code it is on, then
// each message under who wrote it, then where the comment is at
func commentChatText(c comments.Comment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[white]%s[-]  [gray]%s[-]\n", tview.Escape(c.Location()), tview.Escape(c.View))
	code := strings.Split(c.Code, "\n")
	if len(code) > commentChatCodeMaxLines {
		code = append(code[:commentChatCodeMaxLines:commentChatCodeMaxLines], fmt.Sprintf("... %d more lines", len(code)-commentChatCodeMaxLines))
	}
	for _, line := range code {
		fmt.Fprintf(&b, "[gray]│ %s[-]\n", tview.Escape(line))
	}
	for _, m := range c.Thread {
		who := "[gold::b]You[-::-]"
		if m.From == comments.FromClaude {
			who = "[forestgreen::b]Claude Code[-::-]"
		}
		fmt.Fprintf(&b, "\n%s  [gray]%s[-]\n", who, m.At.Local().Format("01-02 15:04"))
		for _, line := range strings.Split(m.Text, "\n") {
			fmt.Fprintf(&b, "  %s\n", tview.Escape(line))
		}
	}
	switch c.Status {
	case comments.StatusInProgress:
		b.WriteString("\n[gold]Claude Code is answering...[-]\n")
	case comments.StatusPending:
		b.WriteString("\n[gray]Waiting for /giff in Claude Code, in this repository[-]\n")
	}
	return b.String()
}

// findComment returns the comment id of the repository at repoRoot
func findComment(repoRoot, id string) (comments.Comment, bool) {
	f, err := comments.Read(repoRoot)
	if err != nil {
		return comments.Comment{}, false
	}
	for _, c := range f.Comments {
		if c.ID == id {
			return c, true
		}
	}
	return comments.Comment{}, false
}

// showCommentChat opens the exchange of the comment id as a chat, with a text
// area under it to go on with it: Option+Enter sends what is written as the
// next question, once Claude Code has answered the last one. The answers come
// in as Claude Code writes them.
func showCommentChat(ctx *DiffViewContext, id string) {
	focus := ctx.app.GetFocus()
	c, ok := findComment(ctx.repoRoot, id)
	if !ok {
		ctx.updateGlobalStatus("The comment is gone", "tomato")
		return
	}
	background := util.ModalBackgroundColor.ToTcellColor()
	border := tcell.GetColor(string(util.CommitAreaBorderColor))

	history := tview.NewTextView().SetDynamicColors(true).SetWrap(true).SetWordWrap(true)
	history.SetBackgroundColor(background)

	input := tview.NewTextArea()
	input.SetTextStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	input.SetPlaceholderStyle(tcell.StyleDefault.Foreground(util.PlaceholderColor.ToTcellColor()).Background(background))
	input.SetBackgroundColor(background)
	input.SetBorder(true).SetBorderColor(border).SetTitleAlign(tview.AlignLeft)

	// render shows the comment as it is now, keeping the history scrolled
	// back if it was
	render := func() {
		if fresh, ok := findComment(ctx.repoRoot, id); ok {
			c = fresh
		} else {
			c.Status = comments.StatusAnswered
		}
		row, _ := history.GetScrollOffset()
		_, _, _, height := history.GetInnerRect()
		atEnd := history.GetText(false) == "" || row+height >= history.GetOriginalLineCount()
		history.SetText(commentChatText(c))
		if atEnd {
			history.ScrollToEnd()
		}
		if c.Status == comments.StatusAnswered {
			input.SetPlaceholder("Go on with Claude Code about these lines")
			input.SetTitle(" Reply  (Option+Enter:send) ")
		} else {
			input.SetPlaceholder("Claude Code has not answered yet")
			input.SetTitle(" Reply  (after the answer) ")
		}
	}
	render()

	closeChat := func() {
		commentChatRefresh = nil
		closeModal(ctx, focus)
	}
	commentChatRefresh = render
	scroll := func(by int) {
		row, col := history.GetScrollOffset()
		history.ScrollTo(max(row+by, 0), col)
	}

	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc:
			closeChat()
			return nil
		case event.Key() == tcell.KeyTab:
			ctx.app.SetFocus(history)
			return nil
		case isAltEnter(event):
			text := strings.TrimSpace(input.GetText())
			if text == "" {
				return nil
			}
			if c.Status != comments.StatusAnswered {
				ctx.updateGlobalStatus("Wait for Claude Code to answer before you reply", "tomato")
				return nil
			}
			if err := comments.Reply(ctx.repoRoot, id, text); err != nil {
				ctx.updateGlobalStatus("Failed to send the reply: "+err.Error(), "tomato")
				return nil
			}
			input.SetText("", false)
			render()
			history.ScrollToEnd()
			return nil
		}
		return event
	})
	history.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc, event.Key() == tcell.KeyRune && keyRune(event) == 'q':
			closeChat()
			return nil
		case event.Key() == tcell.KeyTab, event.Key() == tcell.KeyRune && keyRune(event) == 'i':
			ctx.app.SetFocus(input)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'j', event.Key() == tcell.KeyCtrlE:
			scroll(1)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'k', event.Key() == tcell.KeyCtrlY:
			scroll(-1)
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'g':
			history.ScrollToBeginning()
			return nil
		case event.Key() == tcell.KeyRune && keyRune(event) == 'G':
			history.ScrollToEnd()
			return nil
		}
		return event
	})

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(history, 0, 1, false).
		AddItem(input, 5, 0, true)
	box.SetBackgroundColor(background)
	box.SetBorder(true).SetBorderColor(border).
		SetTitle(" Chat with Claude Code  (Tab:scroll the chat / write  Esc:close) ")
	_, _, _, height := ctx.mainView.GetRect()
	showModal(ctx, "chat", box, max(height-4, 16), input)
}
