package util

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func contrastOf(fg, bg string) float64 {
	return contrastRatio(luminance(tcell.GetColor(fg).RGB()), luminance(tcell.GetColor(bg).RGB()))
}

func TestReadableOn(t *testing.T) {
	cursor := string(CursorLineColor)

	// A comment on the cursor row is lightened until it can be read, and
	// stays blue.
	got := readableOn("#6272a5", cursor)
	if c := contrastOf(got, cursor); c < minReadableContrast {
		t.Errorf("readableOn(comment) = %s, contrast %.2f, want at least %.1f", got, c, minReadableContrast)
	}
	if r, _, b := tcell.GetColor(got).RGB(); b <= r {
		t.Errorf("readableOn(comment) = %s, lost its blue", got)
	}

	// Colors that read well already, and the default color, are left alone.
	for _, fg := range []string{"#f8f8f2", "-", ""} {
		if got := readableOn(fg, cursor); got != fg {
			t.Errorf("readableOn(%q) = %q, want it unchanged", fg, got)
		}
	}
}

func TestEnsureReadableForeground(t *testing.T) {
	cursor := string(CursorLineColor)
	line := "[#f8f8f2:" + cursor + "]x [#6272a5:" + cursor + "]# note[-:" + cursor + "] [#6272a5:" + SearchHighlightBg + "]hit[-:-] arr[i[]"
	want := "[#f8f8f2:" + cursor + "]x [" + readableOn("#6272a5", cursor) + ":" + cursor + "]# note[-:" + cursor + "] [" +
		readableOn("#6272a5", SearchHighlightBg) + ":" + SearchHighlightBg + "]hit[-:-] arr[i[]"
	if got := EnsureReadableForeground(line, cursor); got != want {
		t.Errorf("EnsureReadableForeground() =\n%s\nwant\n%s", got, want)
	}
	if StripTviewTags(EnsureReadableForeground(line, cursor)) != StripTviewTags(line) {
		t.Error("EnsureReadableForeground() changed the text")
	}
}
