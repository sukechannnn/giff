package ui

import (
	"strings"

	"github.com/rivo/tview"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// computeInlineDiffMasks computes character-level diff masks for a pair of old/new plain text lines.
// Returns boolean masks where true indicates a changed character position.
func computeInlineDiffMasks(oldPlain, newPlain string) (delMask []bool, addMask []bool) {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(oldPlain, newPlain, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	delMask = make([]bool, len([]rune(oldPlain)))
	addMask = make([]bool, len([]rune(newPlain)))

	delIdx := 0
	addIdx := 0
	for _, d := range diffs {
		runes := []rune(d.Text)
		switch d.Type {
		case diffmatchpatch.DiffEqual:
			delIdx += len(runes)
			addIdx += len(runes)
		case diffmatchpatch.DiffDelete:
			for i := range runes {
				if delIdx+i < len(delMask) {
					delMask[delIdx+i] = true
				}
			}
			delIdx += len(runes)
		case diffmatchpatch.DiffInsert:
			for i := range runes {
				if addIdx+i < len(addMask) {
					addMask[addIdx+i] = true
				}
			}
			addIdx += len(runes)
		}
	}

	fillSingleCharGaps(delMask)
	fillSingleCharGaps(addMask)

	return delMask, addMask
}

// fillSingleCharGaps absorbs unchanged runs of a single character sandwiched
// between changed runs (e.g. the '.' in "foo.bar" when both "foo" and "bar"
// changed), so highlights render as one continuous block instead of
// fragmented segments.
func fillSingleCharGaps(mask []bool) {
	for i := 1; i < len(mask)-1; i++ {
		if !mask[i] && mask[i-1] && mask[i+1] {
			mask[i] = true
		}
	}
}

// computeAllInlineMasks computes inline diff masks for all lines.
// It pairs adjacent groups of '-'/'+' lines and computes character-level masks.
// Returns a slice of masks (nil entry means no inline highlighting for that line).
func computeAllInlineMasks(codeLines []string, lineTypes []byte) [][]bool {
	masks := make([][]bool, len(codeLines))
	i := 0
	for i < len(codeLines) {
		if lineTypes[i] != '-' {
			i++
			continue
		}

		delStart := i
		for i < len(codeLines) && lineTypes[i] == '-' {
			i++
		}
		delEnd := i

		addStart := i
		for i < len(codeLines) && lineTypes[i] == '+' {
			i++
		}
		addEnd := i

		if addStart == addEnd {
			continue
		}

		pairs := delEnd - delStart
		if addEnd-addStart < pairs {
			pairs = addEnd - addStart
		}

		for k := 0; k < pairs; k++ {
			delMask, addMask := computeInlineDiffMasks(codeLines[delStart+k], codeLines[addStart+k])
			masks[delStart+k] = delMask
			masks[addStart+k] = addMask
		}
	}
	return masks
}

// renderLineFallbackWithMask renders a code line (without prefix) with inline diff mask applied.
func renderLineFallbackWithMask(codeLine string, mask []bool, baseFg, baseBg, maskBg string) string {
	runes := []rune(codeLine)
	if len(runes) == 0 {
		return ""
	}

	var sb strings.Builder
	segStart := 0
	for segStart < len(runes) {
		masked := segStart < len(mask) && mask[segStart]
		segEnd := segStart + 1
		for segEnd < len(runes) {
			nextMasked := segEnd < len(mask) && mask[segEnd]
			if nextMasked != masked {
				break
			}
			segEnd++
		}

		segText := tview.Escape(string(runes[segStart:segEnd]))
		bg := baseBg
		if masked && maskBg != "" {
			bg = maskBg
		}
		if baseFg != "" {
			if bg != "" {
				sb.WriteString("[" + baseFg + ":" + bg + "]" + segText + "[-:-]")
			} else {
				sb.WriteString("[" + baseFg + "]" + segText + "[-]")
			}
		} else {
			if bg != "" {
				sb.WriteString("[:" + bg + "]" + segText + "[-:-]")
			} else {
				sb.WriteString(segText)
			}
		}
		segStart = segEnd
	}
	return sb.String()
}
