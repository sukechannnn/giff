package util

import (
	"strconv"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ColorColde string

const (
	BackgroundColor          = ColorColde("#34373E") // ("#272A32")
	ModalBackgroundColor     = ColorColde("#22242A") // darker than BackgroundColor, so a modal stands out over it
	NotSelectedFileLineColor = ColorColde("#383E50")
	MainTextColor            = ColorColde("#FFFFFF")
	CommitAreaBorderColor    = ColorColde("#4A5568")
	PlaceholderColor         = ColorColde("#808080")
	CursorLineColor          = ColorColde("#6A71F7") // RGB, so the cursor looks the same in every terminal palette
)

// The colors of the pane and status bar titles, one for each kind, so each
// stands out from the others at a glance
const (
	BranchTitleColor     = ColorColde("#98C379") // Branch: the branch checked out
	ModeTitleColor       = ColorColde("#FF79C6") // Mode: PR
	WatchTitleColor      = ColorColde("#61AFEF") // Watch: on
	WhitespaceTitleColor = ColorColde("#F1FA8C") // Hide whitespace: on
	FileListTitleColor   = ColorColde("#8BE9FD") // a file list other than the changes: PR, commit, file browser
	PaneTitleColor       = ColorColde("#999999") // the other pane titles: key hints, Before / After
)

// TitleSeparator goes between the titles shown together on a border
var TitleSeparator = ColoredTitle(PlaceholderColor, " | ")

// ColoredTitle returns text as a title drawn in c. The background is spelled
// out, as tview draws a title cut short to fit on black otherwise. It does not
// reset the color at its end: tview would take the title for one cut short
// and put an ellipsis on it, and what follows it sets a color of its own.
func ColoredTitle(c ColorColde, text string) string {
	return "[" + string(c) + ":" + string(BackgroundColor) + "]" + tview.Escape(text)
}

func (c ColorColde) hex() string {
	return string(c)[1:]
}

func (c ColorColde) ToTcellColor() tcell.Color {
	hexValue, _ := strconv.ParseInt(c.hex(), 16, 32)
	return tcell.NewHexColor(int32(hexValue))
}
