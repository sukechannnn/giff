package ui

import (
	"reflect"
	"testing"
)

func maskedString(s string, mask []bool) string {
	runes := []rune(s)
	out := make([]rune, len(runes))
	for i, r := range runes {
		if i < len(mask) && mask[i] {
			out[i] = r
		} else {
			out[i] = '.'
		}
	}
	return string(out)
}

func TestComputeInlineDiffMasksWordLikeChunks(t *testing.T) {
	// Character-level diff without semantic cleanup would keep the common
	// letters of "Name"/"Info" ("n") and produce fragmented highlights.
	// Semantic cleanup should merge them into one contiguous changed block.
	oldLine := "value := userName"
	newLine := "value := userInfo"

	delMask, addMask := computeInlineDiffMasks(oldLine, newLine)

	if got, want := maskedString(oldLine, delMask), ".............Name"; got != want {
		t.Errorf("delMask highlights %q, want %q", got, want)
	}
	if got, want := maskedString(newLine, addMask), ".............Info"; got != want {
		t.Errorf("addMask highlights %q, want %q", got, want)
	}
}

func TestComputeInlineDiffMasksUnchangedPartsStayUnmasked(t *testing.T) {
	oldLine := "foo(bar)"
	newLine := "foo(baz)"

	delMask, addMask := computeInlineDiffMasks(oldLine, newLine)

	for i, masked := range delMask[:4] {
		if masked {
			t.Errorf("delMask[%d] = true for unchanged prefix %q", i, oldLine[:4])
		}
	}
	for i, masked := range addMask[:4] {
		if masked {
			t.Errorf("addMask[%d] = true for unchanged prefix %q", i, newLine[:4])
		}
	}
	// A single-character change keeps a single-character highlight.
	if got, want := maskedString(oldLine, delMask), "......r."; got != want {
		t.Errorf("delMask highlights %q, want %q", got, want)
	}
	if got, want := maskedString(newLine, addMask), "......z."; got != want {
		t.Errorf("addMask highlights %q, want %q", got, want)
	}
}

func TestFillSingleCharGaps(t *testing.T) {
	tests := []struct {
		name string
		in   []bool
		want []bool
	}{
		{
			name: "fills single gap between changed runs",
			in:   []bool{true, false, true},
			want: []bool{true, true, true},
		},
		{
			name: "keeps two-char gaps",
			in:   []bool{true, false, false, true},
			want: []bool{true, false, false, true},
		},
		{
			name: "keeps leading and trailing unchanged chars",
			in:   []bool{false, true, false},
			want: []bool{false, true, false},
		},
		{
			name: "empty mask",
			in:   []bool{},
			want: []bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mask := make([]bool, len(tt.in))
			copy(mask, tt.in)
			fillSingleCharGaps(mask)
			if !reflect.DeepEqual(mask, tt.want) {
				t.Errorf("fillSingleCharGaps(%v) = %v, want %v", tt.in, mask, tt.want)
			}
		})
	}
}
