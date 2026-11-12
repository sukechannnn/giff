package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// branchPickerRows is how many branches the picker lists at once
const branchPickerRows = 16

// branchLabel is how the picker lists b: its name, its pull request, and
// where it is on the chain of bases of the current branch
func branchLabel(b git.Branch) string {
	label := tview.Escape(b.Name)
	if b.PR != nil {
		draft := ""
		if b.PR.IsDraft {
			draft = "[gray](draft)[-] "
		}
		label += fmt.Sprintf("  [%s]#%d[-] %s%s", util.FileListTitleColor, b.PR.Number, draft, tview.Escape(b.PR.Title))
	}
	switch {
	case b.BaseDepth == 1:
		label += fmt.Sprintf("  [%s](base)[-]", util.ModeTitleColor)
	case b.BaseDepth > 1:
		label += fmt.Sprintf("  [%s](base of base ×%d)[-]", util.ModeTitleColor, b.BaseDepth-1)
	}
	return label
}

// branchMatches reports whether b has query in its name or its pull
// request's number or title, ignoring case
func branchMatches(b git.Branch, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	text := b.Name
	if b.PR != nil {
		text += fmt.Sprintf(" #%d %s", b.PR.Number, b.PR.Title)
	}
	return strings.Contains(strings.ToLower(text), query)
}

// showBranchPicker lists branches over mainView. Enter calls onPick with the
// selected one, Esc closes the picker and gives focus back to focusBack, and
// '/' narrows the list by name or pull request.
func showBranchPicker(app *tview.Application, mainView, focusBack tview.Primitive, branches []git.Branch, current string, onPick func(git.Branch)) {
	closePicker := func() {
		app.SetRoot(mainView, true)
		app.SetFocus(focusBack)
	}

	background := util.ModalBackgroundColor.ToTcellColor()
	borderColor := tcell.GetColor(string(util.CommitAreaBorderColor))

	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	list.SetBackgroundColor(background)
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	list.SetSelectedBackgroundColor(tcell.GetColor(string(util.CursorLineColor)))
	// While the filter is typed into, the selection is drawn as in a pane
	// without focus, so it is clear which one the keys go to
	list.SetFocusFunc(func() { list.SetSelectedBackgroundColor(tcell.GetColor(string(util.CursorLineColor))) })
	list.SetBlurFunc(func() { list.SetSelectedBackgroundColor(tcell.GetColor(string(util.NotSelectedFileLineColor))) })

	filter := tview.NewInputField().SetLabel("/")
	filter.SetBackgroundColor(background)
	filter.SetLabelStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	filter.SetFieldStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))

	// The branches the filter lets through, in the order of the list
	var shown []git.Branch
	fill := func() {
		shown = shown[:0]
		list.Clear()
		for _, b := range branches {
			if branchMatches(b, filter.GetText()) {
				shown = append(shown, b)
				list.AddItem(branchLabel(b), "", 0, nil)
			}
		}
	}
	fill()

	list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i < len(shown) {
			closePicker()
			onPick(shown[i])
		}
	})
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc && filter.GetText() != "":
			filter.SetText("")
			fill()
			return nil
		case event.Key() == tcell.KeyEsc, event.Key() == tcell.KeyRune && event.Rune() == 'q':
			closePicker()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == '/':
			app.SetFocus(filter)
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case event.Key() == tcell.KeyRune && event.Rune() == 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
		return event
	})

	filter.SetChangedFunc(func(string) { fill() })
	filter.SetDoneFunc(func(key tcell.Key) {
		// Narrowed down to one, Enter switches to it right away
		if key == tcell.KeyEnter && len(shown) == 1 {
			closePicker()
			onPick(shown[0])
			return
		}
		if key == tcell.KeyEsc {
			filter.SetText("")
			fill()
		}
		app.SetFocus(list)
	})
	filter.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Pick from the narrowed list without leaving the filter
		switch event.Key() {
		case tcell.KeyDown, tcell.KeyCtrlN:
			list.SetCurrentItem(min(list.GetCurrentItem()+1, max(list.GetItemCount()-1, 0)))
			return nil
		case tcell.KeyUp, tcell.KeyCtrlP:
			list.SetCurrentItem(max(list.GetCurrentItem()-1, 0))
			return nil
		}
		return event
	})

	title := " Switch branch  (j/k:select  Enter:switch  /:filter  Esc:cancel) "
	if current != "" {
		title = fmt.Sprintf(" Switch from %s  (j/k:select  Enter:switch  /:filter  Esc:cancel) ", tview.Escape(current))
	}
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(filter, 1, 0, false).
		AddItem(list, 0, 1, true)
	box.SetBackgroundColor(background)
	box.SetBorder(true).SetBorderColor(borderColor).SetTitle(title)

	// Centred over the main view, which shows around it
	height := min(len(branches), branchPickerRows) + 3
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(box, height, 0, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	pages := tview.NewPages().
		AddPage("main", mainView, true, true).
		AddPage("branches", modal, true, true)
	app.SetRoot(pages, true)
	app.SetFocus(list)
}
