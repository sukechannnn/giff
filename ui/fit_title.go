package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// fitTitle lays out a pane whose title is centred while it fits on the
// border, and moved to the left when it does not, so it is cut short at its
// end rather than at both
type fitTitle struct {
	tview.Primitive
	box *tview.Box
}

// withFitTitle returns p, whose box is box, to be laid out in its place
func withFitTitle(p tview.Primitive, box *tview.Box) tview.Primitive {
	return fitTitle{Primitive: p, box: box}
}

// Draw aligns the title for the width the pane has, then draws it
func (f fitTitle) Draw(screen tcell.Screen) {
	_, _, width, _ := f.box.GetRect()
	if tview.TaggedStringWidth(f.box.GetTitle()) > width-2 {
		f.box.SetTitleAlign(tview.AlignLeft)
	} else {
		f.box.SetTitleAlign(tview.AlignCenter)
	}
	f.Primitive.Draw(screen)
}
