package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/git"
)

func TestBlameGutter(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	a := git.BlameLine{Hash: strings.Repeat("a", 40), Author: "山田 太郎さんと仲間たち", Time: when, Summary: "first"}
	b := git.BlameLine{Hash: strings.Repeat("b", 40), Author: "Bob", Time: when}
	none := git.BlameLine{Hash: strings.Repeat("0", 40), Author: "Not Committed Yet"}

	gutter := blameGutter([]git.BlameLine{a, a, b, none})
	for i, g := range gutter {
		if w := tview.TaggedStringWidth(g); w != blameGutterWidth {
			t.Errorf("gutter[%d] = %q is %d wide, want %d", i, g, w, blameGutterWidth)
		}
	}
	if !strings.HasPrefix(gutter[0], "[gold]aaaaaaa[-] [dimgray]2026-01-02[-] 山田 太郎さ") {
		t.Errorf("gutter[0] = %q", gutter[0])
	}
	if strings.TrimSpace(gutter[1]) != "" {
		t.Errorf("gutter[1] = %q, want blank under the same commit", gutter[1])
	}
	if !strings.Contains(gutter[2], "bbbbbbb") || !strings.Contains(gutter[3], "Not committed yet") {
		t.Errorf("gutter[2:] = %q", gutter[2:])
	}
}
