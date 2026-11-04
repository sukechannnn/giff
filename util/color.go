package util

import (
	"strconv"

	"github.com/gdamore/tcell/v2"
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

func (c ColorColde) hex() string {
	return string(c)[1:]
}

func (c ColorColde) ToTcellColor() tcell.Color {
	hexValue, _ := strconv.ParseInt(c.hex(), 16, 32)
	return tcell.NewHexColor(int32(hexValue))
}
