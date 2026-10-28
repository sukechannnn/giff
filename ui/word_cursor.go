package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/tview"
)

// The split view panes a word can sit in, in the order 'n' visits them
const (
	splitBefore = 0
	splitAfter  = 1
)

// wordSpan is a word of a diff pane row: rune offsets [start, end) into the
// content of the row, and in split view the pane it is in
type wordSpan struct {
	side  int
	start int
	end   int
	text  string
}

// before reports whether w comes before o in a row
func (w wordSpan) before(o wordSpan) bool {
	if w.side != o.side {
		return w.side < o.side
	}
	return w.start < o.start
}

// wordClass groups the characters a word is made of; 0 is a separator. Han,
// hiragana and katakana are classes of their own, so Japanese text breaks
// where the script changes.
func wordClass(r rune) int {
	switch {
	case unicode.Is(unicode.Han, r):
		return 2
	case unicode.Is(unicode.Hiragana, r):
		return 3
	case unicode.Is(unicode.Katakana, r) || r == 'ー':
		return 4
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r):
		return 1
	}
	return 0
}

// lineWords splits plain text into words: runs of identifier characters,
// skipping the symbols and spaces between them
func lineWords(plain string, side int) []wordSpan {
	runes := []rune(plain)
	var words []wordSpan
	for i := 0; i < len(runes); {
		class := wordClass(runes[i])
		if class == 0 {
			i++
			continue
		}
		start := i
		for i < len(runes) && wordClass(runes[i]) == class {
			i++
		}
		words = append(words, wordSpan{side: side, start: start, end: i, text: string(runes[start:i])})
	}
	return words
}

// fileViewLines returns the lines of the file the file viewer shows
func fileViewLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// viewRowCount returns how many rows the diff pane shows
func viewRowCount(ctx *DiffViewContext) int {
	if _, ok := ctx.viewUpdater.(*FileViewUpdater); ok {
		return len(fileViewLines(*ctx.currentDiffText))
	}
	if *ctx.isSplitView {
		return getSplitViewLineCount(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
	}
	return len(getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot).Lines)
}

// rowTexts returns the plain content of a row of the diff pane: the before
// and the after pane in split view, the one line otherwise. Fold rows have
// no content.
func rowTexts(ctx *DiffViewContext, row int) []string {
	if row < 0 {
		return nil
	}
	if _, ok := ctx.viewUpdater.(*FileViewUpdater); ok {
		lines := fileViewLines(*ctx.currentDiffText)
		if row >= len(lines) {
			return nil
		}
		return []string{lines[row]}
	}
	if *ctx.isSplitView {
		content := getCachedSplitContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
		if row >= len(content.Rows) || content.Rows[row].IsFoldIndicator {
			return nil
		}
		return []string{stripTviewTags(content.BeforeLines[row]), stripTviewTags(content.AfterLines[row])}
	}
	content := getCachedUnifiedContent(*ctx.currentDiffText, ctx.foldState, *ctx.currentFile, ctx.repoRoot)
	if row >= len(content.Lines) || content.Lines[row].IsFoldIndicator {
		return nil
	}
	return []string{stripTviewTags(content.Lines[row].Content)}
}

// rowWords returns the words of a row of the diff pane, those of the before
// pane first in split view
func rowWords(ctx *DiffViewContext, row int) []wordSpan {
	var words []wordSpan
	for side, text := range rowTexts(ctx, row) {
		words = append(words, lineWords(text, side)...)
	}
	return words
}

// wordCursorActive reports whether the word cursor is on the row and file the
// cursor is on; moving off them leaves it behind
func wordCursorActive(ctx *DiffViewContext) bool {
	return ctx.word.text != "" && ctx.wordRow == *ctx.cursorY && ctx.wordFile == *ctx.currentFile
}

// moveWordCursor puts the word cursor on the next (direction > 0) or previous
// word of the cursor row, continuing on the next or previous row with words
// at the row's end. With the word cursor off it starts at the row's first or
// last word. It stays put past the last or first word of the view.
func moveWordCursor(ctx *DiffViewContext, direction int) {
	row := *ctx.cursorY
	words := rowWords(ctx, row)
	next := -1
	if wordCursorActive(ctx) {
		for i, w := range words {
			if direction > 0 && ctx.word.before(w) {
				next = i
				break
			}
			if direction < 0 && w.before(ctx.word) {
				next = i
			}
		}
	} else if direction > 0 {
		next = 0
	} else {
		next = len(words) - 1
	}

	rowCount := viewRowCount(ctx)
	for next < 0 || next >= len(words) {
		row += direction
		if row < 0 || row >= rowCount {
			return
		}
		words = rowWords(ctx, row)
		if direction > 0 {
			next = 0
		} else {
			next = len(words) - 1
		}
	}

	*ctx.cursorY = row
	ctx.word = words[next]
	ctx.wordRow = row
	ctx.wordFile = *ctx.currentFile

	// Search for the word right away, so every occurrence lights up
	*ctx.searchQuery = ctx.word.text
	*ctx.searchMatches = findSearchMatches(*ctx.currentDiffText, *ctx.isSplitView, ctx.foldState, *ctx.currentFile, ctx.repoRoot, ctx.word.text)
	*ctx.searchMatchIndex = -1
	for i, m := range *ctx.searchMatches {
		if m == row {
			*ctx.searchMatchIndex = i
			break
		}
	}
	if ctx.viewUpdater != nil {
		ctx.viewUpdater.UpdateWithCursor(*ctx.currentDiffText, row)
	}
	showWordCursorStatus(ctx)
}

// nextWordMatch moves to the next row the word cursor's word is found in,
// and puts the word cursor on its first occurrence there
func nextWordMatch(ctx *DiffViewContext) {
	if len(*ctx.searchMatches) == 0 {
		return
	}
	moveToNextMatch(ctx)

	row := *ctx.cursorY
	// In case the match is in the line number, start before the first word
	putWordCursorOnRow(ctx, row)
	needle := strings.ToLower(ctx.word.text)
	for side, text := range rowTexts(ctx, row) {
		lower := strings.ToLower(text)
		if idx := strings.Index(lower, needle); idx >= 0 {
			start := utf8.RuneCountInString(lower[:idx])
			ctx.word.side, ctx.word.start, ctx.word.end = side, start, start+utf8.RuneCountInString(needle)
			break
		}
	}
	showWordCursorStatus(ctx)
}

// putWordCursorOnRow moves the word cursor to the start of row, before its
// first word, keeping the word it searches for
func putWordCursorOnRow(ctx *DiffViewContext, row int) {
	ctx.wordRow = row
	ctx.word.side, ctx.word.start, ctx.word.end = splitBefore, -1, -1
}

// scrollKeepingWordCursor scrolls the diff pane by direction. When the cursor
// row follows the scroll, the word cursor goes along to the new row, and Enter
// counts the next match from there.
func scrollKeepingWordCursor(ctx *DiffViewContext, direction int) {
	active := wordCursorActive(ctx)
	scrollDiffView(ctx, direction)
	if !active || ctx.wordRow == *ctx.cursorY {
		return
	}
	// The word cursor sits at the row's start, so the row's own match is next
	putWordCursorOnRow(ctx, *ctx.cursorY)
	*ctx.searchMatchIndex = -1
	for i, m := range *ctx.searchMatches {
		if m < *ctx.cursorY {
			*ctx.searchMatchIndex = i
		}
	}
	showWordCursorStatus(ctx)
}

// clearWordCursor turns the word cursor off along with the search it runs,
// and redraws the pane without them
func clearWordCursor(ctx *DiffViewContext) {
	ctx.word = wordSpan{}
	*ctx.searchQuery = ""
	*ctx.searchMatches = nil
	*ctx.searchMatchIndex = -1
	if ctx.viewUpdater != nil {
		ctx.viewUpdater.UpdateWithSelection(*ctx.currentDiffText, *ctx.cursorY, *ctx.selectStart, *ctx.selectEnd, *ctx.isSelecting)
	}
	restoreStatus(ctx.restoreStatus)
}

// showWordCursorStatus puts the word cursor's word and match position, and
// the keys it takes, on the status bar
func showWordCursorStatus(ctx *DiffViewContext) {
	if ctx.setGlobalStatusText == nil {
		return
	}
	ctx.setGlobalStatusText(fmt.Sprintf("[white]/%s [%d/%d][-]  Enter:next match  n/N:next/prev word  Esc:cancel",
		tview.Escape(ctx.word.text), *ctx.searchMatchIndex+1, len(*ctx.searchMatches)))
}
