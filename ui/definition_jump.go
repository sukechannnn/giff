package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
	"github.com/sukechannnn/giff/util"
)

// definitionPickerRows is how many candidates the picker lists at once
const definitionPickerRows = 10

// definitionPreviewRows is how many lines of code the picker previews
const definitionPreviewRows = 12

// jumpToDefinition shows the definition of the word cursor's word in the file
// view. When git grep turns up more than one, a picker lists them with a
// preview of the code, and the one picked is shown.
func jumpToDefinition(ctx *DiffViewContext) {
	if ctx.jumpTo == nil {
		ctx.updateGlobalStatus("Jump to definition is not available here", "tomato")
		return
	}
	if !wordCursorActive(ctx) {
		ctx.updateGlobalStatus("Put the cursor on a word with n first, then go", "tomato")
		return
	}
	word := ctx.word.text
	defs, err := git.FindDefinitions(ctx.repoRoot, word)
	if err != nil {
		ctx.updateGlobalStatus("Failed to search for the definition: "+err.Error(), "tomato")
		return
	}
	if len(defs) == 0 {
		ctx.updateGlobalStatus("No definition of "+tview.Escape(word)+" found", "tomato")
		return
	}
	defs = sameLanguageDefinitions(defs, *ctx.currentFile)
	sortDefinitions(defs, *ctx.currentFile)

	// On a definition already, the others are what is wanted
	here := -1
	if inFileView(ctx) {
		for i, d := range defs {
			if d.Path == *ctx.currentFile && d.Line == *ctx.cursorY+1 {
				here = i
				break
			}
		}
	}
	switch {
	case len(defs) == 1 && here == 0:
		ctx.updateGlobalStatus("This is the only definition of "+tview.Escape(word)+" found", "tomato")
	case len(defs) == 1:
		showDefinition(ctx, defs[0], word)
	default:
		showDefinitionPicker(ctx, defs, word, (here+1)%len(defs))
	}
}

// languageFamilies maps the extensions of the languages whose files share
// names to one key. Any other extension is a language of its own.
var languageFamilies = map[string]string{
	".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".ts": "js", ".tsx": "js", ".mts": "js", ".cts": "js",
	".rb": "ruby", ".rake": "ruby", ".ru": "ruby", ".gemspec": "ruby",
	".py": "python", ".pyi": "python",
	".h": "c", ".c": "c",
}

// languageFileNames are the files without an extension a language is known by
var languageFileNames = map[string]string{
	"Rakefile": "ruby", "Gemfile": "ruby",
}

// languageOf returns the key of the language path is written in, or "" when
// it cannot tell
func languageOf(path string) string {
	if lang, ok := languageFileNames[filepath.Base(path)]; ok {
		return lang
	}
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := languageFamilies[ext]; ok {
		return lang
	}
	return ext
}

// sameLanguageDefinitions keeps the definitions written in the language of
// currentFile. All of them are kept when its language is unknown or none is
// written in it, as a definition elsewhere beats finding nothing.
func sameLanguageDefinitions(defs []git.Location, currentFile string) []git.Location {
	lang := languageOf(currentFile)
	if lang == "" {
		return defs
	}
	var same []git.Location
	for _, d := range defs {
		if languageOf(d.Path) == lang {
			same = append(same, d)
		}
	}
	if len(same) == 0 {
		return defs
	}
	return same
}

// sortDefinitions puts the definitions in the file being read first, then
// those in its directory, keeping git grep's order otherwise
func sortDefinitions(defs []git.Location, currentFile string) {
	rank := func(d git.Location) int {
		switch {
		case d.Path == currentFile:
			return 0
		case filepath.Dir(d.Path) == filepath.Dir(currentFile):
			return 1
		}
		return 2
	}
	sort.SliceStable(defs, func(i, j int) bool { return rank(defs[i]) < rank(defs[j]) })
}

// showDefinition shows d with the word cursor on the word it defines
func showDefinition(ctx *DiffViewContext, d git.Location, word string) {
	ctx.jumpTo(d)
	if *ctx.currentFile != d.Path || !inFileView(ctx) {
		// The file could not be opened, and the status says why
		return
	}
	row := *ctx.cursorY
	span := wordSpan{start: -1, end: -1, text: word}
	for _, w := range rowWords(ctx, row) {
		if w.text == word {
			span = w
			break
		}
	}
	selectWord(ctx, row, span)
	if ctx.setGlobalStatusText != nil {
		ctx.setGlobalStatusText(fmt.Sprintf("[white]%s:%d[-]  C-o:back  go:definition  Enter:next match  Esc:cancel",
			tview.Escape(d.Path), d.Line))
	}
}

// showDefinitionPicker lists defs over the main view, with the code around
// the selected one below the list, starting at selected. Enter shows the
// selected definition, and Esc closes the picker.
func showDefinitionPicker(ctx *DiffViewContext, defs []git.Location, word string, selected int) {
	closePicker := func() {
		ctx.app.SetRoot(ctx.mainView, true)
		if *ctx.isSplitView {
			ctx.app.SetFocus(ctx.splitViewFlex)
		} else {
			ctx.app.SetFocus(ctx.diffView)
		}
	}

	background := util.ModalBackgroundColor.ToTcellColor()
	preview := tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	preview.SetBackgroundColor(background)
	preview.SetBorder(true).SetBorderColor(tcell.GetColor(string(util.CommitAreaBorderColor)))
	contents := map[string]string{}
	showPreview := func(i int) {
		d := defs[i]
		content, ok := contents[d.Path]
		if !ok {
			var err error
			if content, err = util.ReadFileContent(d.Path, ctx.repoRoot); err != nil {
				content = ""
			}
			contents[d.Path] = content
		}
		preview.SetTitle(" " + tview.Escape(d.Path) + " ")
		renderFileView(preview, content, d.Line-1, -1, -1, false, d.Path, word)
		// A few lines of what leads up to the definition
		preview.ScrollTo(max(d.Line-1-3, 0), 0)
	}

	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	list.SetBackgroundColor(background)
	list.SetMainTextStyle(tcell.StyleDefault.Foreground(util.MainTextColor.ToTcellColor()).Background(background))
	list.SetSelectedBackgroundColor(tcell.GetColor(string(util.CursorLineColor)))
	// The path alone tells the candidates apart, but for those in one file
	perPath := map[string]int{}
	for _, d := range defs {
		perPath[d.Path]++
	}
	for _, d := range defs {
		label := tview.Escape(d.Path)
		if perPath[d.Path] > 1 {
			label += fmt.Sprintf("[gray]:%d[-]", d.Line)
		}
		list.AddItem(label, "", 0, nil)
	}
	list.SetChangedFunc(func(i int, _, _ string, _ rune) { showPreview(i) })
	list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		closePicker()
		showDefinition(ctx, defs[i], word)
	})
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEsc, event.Key() == tcell.KeyRune && event.Rune() == 'q':
			closePicker()
			return nil
		case event.Key() == tcell.KeyRune && event.Rune() == 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case event.Key() == tcell.KeyRune && event.Rune() == 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
		return event
	})
	list.SetCurrentItem(selected)
	showPreview(selected)

	listRows := min(len(defs), definitionPickerRows)
	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, listRows, 0, true).
		AddItem(preview, definitionPreviewRows+2, 0, false)
	box.SetBackgroundColor(background)
	box.SetBorder(true).
		SetBorderColor(tcell.GetColor(string(util.CommitAreaBorderColor))).
		SetTitle(fmt.Sprintf(" %d definitions of %s  (j/k:select  Enter:open  Esc:cancel) ", len(defs), tview.Escape(word)))

	// Centred over the main view, which shows around it
	height := listRows + definitionPreviewRows + 4
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(box, height, 0, true).
			AddItem(nil, 0, 1, false), 0, 8, true).
		AddItem(nil, 0, 1, false)

	pages := tview.NewPages().
		AddPage("main", ctx.mainView, true, true).
		AddPage("definitions", modal, true, true)
	ctx.app.SetRoot(pages, true)
	ctx.app.SetFocus(list)
}
