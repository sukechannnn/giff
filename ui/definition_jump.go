package ui

import (
	"fmt"
	"path/filepath"
	"sort"

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
// preview of the code, and the one picked is shown. On the source of a
// Terraform module, it shows the module's files instead, with or without the
// word cursor.
func jumpToDefinition(ctx *DiffViewContext) {
	if ctx.jumpTo == nil {
		ctx.updateGlobalStatus("Jump to definition is not available here", "tomato")
		return
	}
	if git.IsTerraformFile(*ctx.currentFile) {
		// The after side first in split view, as the file is now
		texts := rowTexts(ctx, *ctx.cursorY)
		for i := len(texts) - 1; i >= 0; i-- {
			if source, dir, ok := git.TerraformModuleSource(*ctx.currentFile, texts[i]); ok {
				jumpToTerraformModule(ctx, source, dir)
				return
			}
		}
	}
	if !wordCursorActive(ctx) {
		ctx.updateGlobalStatus("Put the cursor on a word with n first, then go", "tomato")
		return
	}
	if !git.SupportsDefinitions(*ctx.currentFile) {
		ctx.updateGlobalStatus("Jump to definition does not support "+tview.Escape(filepath.Base(*ctx.currentFile))+" yet", "tomato")
		return
	}
	word := ctx.word.text
	defs, err := git.FindDefinitions(ctx.repoRoot, word)
	if err != nil {
		ctx.updateGlobalStatus("Failed to search for the definition: "+err.Error(), "tomato")
		return
	}
	defs = sameLanguageDefinitions(defs, *ctx.currentFile)
	if len(defs) == 0 {
		ctx.updateGlobalStatus("No definition of "+tview.Escape(word)+" found", "tomato")
		return
	}
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
		title := fmt.Sprintf(" %d definitions of %s  (j/k:select  Enter:open  Esc:cancel) ", len(defs), tview.Escape(word))
		showDefinitionPicker(ctx, defs, title, word, (here+1)%len(defs), func(d git.Location) {
			showDefinition(ctx, d, word)
		})
	}
}

// jumpToTerraformModule shows the module that source, the source of a
// module block, points to: its one Terraform file, or a picker of them with
// main.tf first. dir is the module's directory, "" when it is not in the
// repository.
func jumpToTerraformModule(ctx *DiffViewContext, source, dir string) {
	if dir == "" {
		ctx.updateGlobalStatus(tview.Escape(source)+" is not a module in the repository", "tomato")
		return
	}
	files, err := git.TerraformModuleFiles(ctx.repoRoot, dir)
	if err != nil {
		ctx.updateGlobalStatus("Cannot open the module "+tview.Escape(dir)+": it is not in the working tree", "tomato")
		return
	}
	switch len(files) {
	case 0:
		ctx.updateGlobalStatus("No Terraform file in "+tview.Escape(dir), "tomato")
	case 1:
		showModuleFile(ctx, files[0])
	default:
		title := fmt.Sprintf(" %d files of the module %s  (j/k:select  Enter:open  Esc:cancel) ", len(files), tview.Escape(dir))
		showDefinitionPicker(ctx, files, title, "", 0, func(d git.Location) {
			showModuleFile(ctx, d)
		})
	}
}

// showModuleFile shows a file of a Terraform module from its top
func showModuleFile(ctx *DiffViewContext, d git.Location) {
	ctx.jumpTo(d)
	if *ctx.currentFile != d.Path || !inFileView(ctx) {
		// The file could not be opened, and the status says why
		return
	}
	if ctx.setGlobalStatusText != nil {
		ctx.setGlobalStatusText(fmt.Sprintf("[white]%s[-]  C-o:back", tview.Escape(d.Path)))
	}
}

// sameLanguageDefinitions keeps the definitions written in the language of
// currentFile
func sameLanguageDefinitions(defs []git.Location, currentFile string) []git.Location {
	lang := git.LanguageOf(currentFile)
	var same []git.Location
	for _, d := range defs {
		if git.LanguageOf(d.Path) == lang {
			same = append(same, d)
		}
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

// showDefinitionPicker lists defs over the main view under title, with the
// code around the selected one below the list and word lit up in it,
// starting at selected. Enter calls open with the selected one, and Esc
// closes the picker.
func showDefinitionPicker(ctx *DiffViewContext, defs []git.Location, title, word string, selected int, open func(git.Location)) {
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
		open(defs[i])
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
		SetTitle(title)

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
