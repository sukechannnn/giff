package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rivo/tview"
	"github.com/sukechannnn/giff/util"
)

func TestGenerateSplitViewContent(t *testing.T) {
	tests := []struct {
		name           string
		diffText       string
		oldLineMap     map[int]int
		newLineMap     map[int]int
		wantBefore     []string
		wantAfter      []string
		wantBeforeNums []string
		wantAfterNums  []string
	}{
		{
			name: "削除行のみ",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,2 @@
 line1
-line2
 line3`,
			oldLineMap: map[int]int{
				0: 1,
				1: 2,
				2: 3,
			},
			newLineMap: map[int]int{
				0: 1,
				2: 2,
			},
			wantBefore: []string{
				" line1",
				"[#E7454E]-line2[-]",
				" line3",
			},
			wantAfter: []string{
				" line1",
				"[dimgray] [-]",
				" line3",
			},
			wantBeforeNums: []string{
				"1",
				"2",
				"3",
			},
			wantAfterNums: []string{
				"1",
				" ",
				"2",
			},
		},
		{
			name: "追加行のみ",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,2 +1,3 @@
 line1
+line2
 line3`,
			oldLineMap: map[int]int{
				0: 1,
				2: 2,
			},
			newLineMap: map[int]int{
				0: 1,
				1: 2,
				2: 3,
			},
			wantBefore: []string{
				" line1",
				"[dimgray] [-]",
				" line3",
			},
			wantAfter: []string{
				" line1",
				"[#00AC37]+line2[-]",
				" line3",
			},
			wantBeforeNums: []string{
				"1",
				" ",
				"2",
			},
			wantAfterNums: []string{
				"1",
				"2",
				"3",
			},
		},
		{
			name: "変更なし行のみ",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
 line2
 line3`,
			oldLineMap: map[int]int{
				0: 1,
				1: 2,
				2: 3,
			},
			newLineMap: map[int]int{
				0: 1,
				1: 2,
				2: 3,
			},
			wantBefore: []string{
				" line1",
				" line2",
				" line3",
			},
			wantAfter: []string{
				" line1",
				" line2",
				" line3",
			},
			wantBeforeNums: []string{
				"1",
				"2",
				"3",
			},
			wantAfterNums: []string{
				"1",
				"2",
				"3",
			},
		},
		{
			name: "混合パターン",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2_modified
 line3`,
			oldLineMap: map[int]int{
				0: 1,
				1: 2,
				3: 3,
			},
			newLineMap: map[int]int{
				0: 1,
				2: 2,
				3: 3,
			},
			// Pairing logic: deletion and addition lines are displayed on the same row
			wantBefore: []string{
				" line1",
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]line2[-:-]",
				" line3",
			},
			wantAfter: []string{
				" line1",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]line2[-:-][#00AC37:#1A4D1A]_modified[-:-]",
				" line3",
			},
			wantBeforeNums: []string{
				"1",
				"2",
				"3",
			},
			wantAfterNums: []string{
				"1",
				"2",
				"3",
			},
		},
		{
			name: "行番号の桁数が異なる場合",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -98,3 +98,3 @@
 line98
-line99
+line99_modified
 line100`,
			oldLineMap: map[int]int{
				0: 98,
				1: 99,
				3: 100,
			},
			newLineMap: map[int]int{
				0: 98,
				2: 99,
				3: 100,
			},
			// Pairing logic: deletion and addition lines are displayed on the same row.
			// Lines 1-97 before the hunk are folded.
			wantBefore: []string{
				"[dimgray]... 97 lines hidden (press 'e' to expand) ...[-]",
				" line98",
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]line99[-:-]",
				" line100",
			},
			wantAfter: []string{
				"[dimgray]... 97 lines hidden (press 'e' to expand) ...[-]",
				" line98",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]line99[-:-][#00AC37:#1A4D1A]_modified[-:-]",
				" line100",
			},
			wantBeforeNums: []string{
				"   ",
				" 98",
				" 99",
				"100",
			},
			wantAfterNums: []string{
				"   ",
				" 98",
				" 99",
				"100",
			},
		},
		{
			name: "ヘッダー行の処理",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,1 +1,1 @@
-old
+new`,
			oldLineMap: map[int]int{
				0: 1,
			},
			newLineMap: map[int]int{
				1: 1,
			},
			// Pairing logic: deletion and addition lines are displayed on the same row
			wantBefore: []string{
				"[#E7454E:#3A0000]-[-:-][#E7454E:#5C1A1A]old[-:-]",
			},
			wantAfter: []string{
				"[#00AC37:#002500]+[-:-][#00AC37:#1A4D1A]new[-:-]",
			},
			wantBeforeNums: []string{
				"1",
			},
			wantAfterNums: []string{
				"1",
			},
		},
		{
			name: "ブラケットを含むテキストのエスケープ",
			diffText: `diff --git a/test.go b/test.go
index 123..456 789
--- a/test.go
+++ b/test.go
@@ -1,2 +1,2 @@
-var foo [int]string
+var foo [white]string`,
			oldLineMap: map[int]int{
				0: 1,
			},
			newLineMap: map[int]int{
				1: 1,
			},
			// Pairing logic: deletion and addition lines are displayed on the same row
			wantBefore: []string{
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]var foo [[-:-][#E7454E:#5C1A1A]int[-:-][#E7454E:#3A0000]]string[-:-]",
			},
			wantAfter: []string{
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]var foo [[-:-][#00AC37:#1A4D1A]white[-:-][#00AC37:#002500]]string[-:-]",
			},
			wantBeforeNums: []string{
				"1",
			},
			wantAfterNums: []string{
				"1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := generateSplitViewContent(tt.diffText, tt.oldLineMap, tt.newLineMap, nil, "", "")

			// Check BeforeLines
			if len(content.BeforeLines) != len(tt.wantBefore) {
				t.Errorf("BeforeLines length mismatch: got %d, want %d", len(content.BeforeLines), len(tt.wantBefore))
			}
			for i := range tt.wantBefore {
				if i < len(content.BeforeLines) && content.BeforeLines[i] != tt.wantBefore[i] {
					t.Errorf("BeforeLines[%d]: got %q, want %q", i, content.BeforeLines[i], tt.wantBefore[i])
				}
			}

			// Check AfterLines
			if len(content.AfterLines) != len(tt.wantAfter) {
				t.Errorf("AfterLines length mismatch: got %d, want %d", len(content.AfterLines), len(tt.wantAfter))
			}
			for i := range tt.wantAfter {
				if i < len(content.AfterLines) && content.AfterLines[i] != tt.wantAfter[i] {
					t.Errorf("AfterLines[%d]: got %q, want %q", i, content.AfterLines[i], tt.wantAfter[i])
				}
			}

			// Check BeforeLineNums
			if len(content.BeforeLineNums) != len(tt.wantBeforeNums) {
				t.Errorf("BeforeLineNums length mismatch: got %d, want %d", len(content.BeforeLineNums), len(tt.wantBeforeNums))
			}
			for i := range tt.wantBeforeNums {
				if i < len(content.BeforeLineNums) && content.BeforeLineNums[i] != tt.wantBeforeNums[i] {
					t.Errorf("BeforeLineNums[%d]: got %q, want %q", i, content.BeforeLineNums[i], tt.wantBeforeNums[i])
				}
			}

			// Check AfterLineNums
			if len(content.AfterLineNums) != len(tt.wantAfterNums) {
				t.Errorf("AfterLineNums length mismatch: got %d, want %d", len(content.AfterLineNums), len(tt.wantAfterNums))
			}
			for i := range tt.wantAfterNums {
				if i < len(content.AfterLineNums) && content.AfterLineNums[i] != tt.wantAfterNums[i] {
					t.Errorf("AfterLineNums[%d]: got %q, want %q", i, content.AfterLineNums[i], tt.wantAfterNums[i])
				}
			}
		})
	}
}

func TestIsHeaderLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"diff --git", "diff --git a/file b/file", true},
		{"index", "index 123..456 789", true},
		{"---", "--- a/file", true},
		{"+++", "+++ b/file", true},
		{"通常の行", " normal line", false},
		{"削除行", "-deleted line", false},
		{"追加行", "+added line", false},
		{"ハンクヘッダー", "@@ -1,3 +1,3 @@", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHeaderLine(tt.line); got != tt.want {
				t.Errorf("isHeaderLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// splitFoldDiff inserts "A" after l2, replaces l10 with "X" and appends "B"
// after l20, leaving a one-line gap (l6) and a four-line gap (l14-l17)
// between the hunks.
const splitFoldDiff = `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,5 +1,6 @@
 l1
 l2
+A
 l3
 l4
 l5
@@ -7,7 +8,7 @@
 l7
 l8
 l9
-l10
+X
 l11
 l12
 l13
@@ -18,3 +19,4 @@
 l18
 l19
 l20
+B
`

func writeSplitFoldFile(t *testing.T) string {
	t.Helper()
	lines := []string{"l1", "l2", "A", "l3", "l4", "l5", "l6", "l7", "l8", "l9", "X"}
	for i := 11; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("l%d", i))
	}
	lines = append(lines, "B")
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "test.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}

func findSplitRow(content *SplitViewContent, after string) int {
	for i, line := range content.AfterLines {
		if strings.HasSuffix(stripTviewTags(line), after) {
			return i
		}
	}
	return -1
}

func TestSplitViewFolds(t *testing.T) {
	repoRoot := writeSplitFoldFile(t)
	oldLineMap, newLineMap := createLineNumberMapping(splitFoldDiff)
	foldState := NewFoldState()

	content := generateSplitViewContent(splitFoldDiff, oldLineMap, newLineMap, foldState, "test.txt", repoRoot)

	// The one-line gap is always shown, with its old and new line numbers
	l6 := findSplitRow(content, " l6")
	if l6 < 0 {
		t.Fatalf("l6 is missing from the split view: %q", content.AfterLines)
	}
	if got := content.BeforeLineNums[l6] + "/" + content.AfterLineNums[l6]; got != " 6/ 7" {
		t.Errorf("l6 line numbers: got %q, want %q", got, " 6/ 7")
	}
	if id := content.FoldIDAtRow(l6); id != "" {
		t.Errorf("fixed fold row must not be toggleable, got fold ID %q", id)
	}

	// The four-line gap is collapsed into an indicator before the last hunk
	indicator := content.FoldRow("fold-15-18")
	if indicator < 0 {
		t.Fatalf("fold-15-18 is missing: %q", content.AfterLines)
	}
	if !strings.Contains(content.AfterLines[indicator], "4 lines hidden") {
		t.Errorf("indicator: got %q", content.AfterLines[indicator])
	}
	if next := stripTviewTags(content.AfterLines[indicator+1]); next != " l18" {
		t.Errorf("row after indicator: got %q, want %q", next, " l18")
	}
	if content.FoldIDAtRow(indicator) != "fold-15-18" {
		t.Errorf("indicator row must be toggleable")
	}

	// The paired row maps to both the '-' and the '+' diff line
	pair := findSplitRow(content, "X")
	if start, end, ok := content.DiffRange(pair, pair); !ok || start != 9 || end != 10 {
		t.Errorf("DiffRange(pair): got %d-%d %v, want 9-10 true", start, end, ok)
	}
	if row := content.RowForDiffIdx(10); row != pair {
		t.Errorf("RowForDiffIdx(10): got %d, want %d", row, pair)
	}
	if _, _, ok := content.DiffRange(indicator, indicator); ok {
		t.Errorf("DiffRange over a fold row must report no diff lines")
	}
	if start, end, ok := content.DiffRange(0, len(content.Rows)-1); !ok || start != 0 || end != 17 {
		t.Errorf("DiffRange(all): got %d-%d %v, want 0-17 true", start, end, ok)
	}

	// Expanding shows the file lines with old numbers shifted by the hunks above
	foldState.ToggleExpand("fold-15-18")
	content = generateSplitViewContent(splitFoldDiff, oldLineMap, newLineMap, foldState, "test.txt", repoRoot)
	for n := 14; n <= 17; n++ {
		row := findSplitRow(content, fmt.Sprintf(" l%d", n))
		if row < 0 {
			t.Fatalf("l%d is missing after expanding: %q", n, content.AfterLines)
		}
		want := fmt.Sprintf("%2d/%2d", n, n+1)
		if got := content.BeforeLineNums[row] + "/" + content.AfterLineNums[row]; got != want {
			t.Errorf("l%d line numbers: got %q, want %q", n, got, want)
		}
		if content.FoldIDAtRow(row) != "fold-15-18" {
			t.Errorf("expanded row l%d must belong to fold-15-18", n)
		}
	}
}

// A '-' line ending one hunk must not be paired with a '+' line starting the next
func TestSplitViewDoesNotPairAcrossHunks(t *testing.T) {
	diffText := `diff --git a/test.txt b/test.txt
@@ -1,2 +1,1 @@
 a
-b
@@ -10,1 +9,2 @@
+Y
 z
`
	oldLineMap, newLineMap := createLineNumberMapping(diffText)
	content := generateSplitViewContent(diffText, oldLineMap, newLineMap, nil, "", "")
	for i := range content.Rows {
		before := stripTviewTags(content.BeforeLines[i])
		after := stripTviewTags(content.AfterLines[i])
		if before == "-b" && after == "+Y" {
			t.Fatalf("'-b' and '+Y' from different hunks were paired on row %d", i)
		}
	}
}

func TestSearchInSplitContent(t *testing.T) {
	oldLineMap, newLineMap := createLineNumberMapping(splitFoldDiff)
	content := generateSplitViewContent(splitFoldDiff, oldLineMap, newLineMap, nil, "", "")

	// "l10" is on the '-' side of the paired row only; a row matches when either side does
	pair := findSplitRow(content, "X")
	got := searchInSplitContent(content, "L10")
	if len(got) != 1 || got[0] != pair {
		t.Errorf("searchInSplitContent(L10): got %v, want [%d]", got, pair)
	}

	// "x" is only on the '+' side of the same row
	got = searchInSplitContent(content, "x")
	if len(got) != 1 || got[0] != pair {
		t.Errorf("searchInSplitContent(x): got %v, want [%d]", got, pair)
	}
}

func TestWriteSplitSideHighlightsSearch(t *testing.T) {
	oldLineMap, newLineMap := createLineNumberMapping(splitFoldDiff)
	content := generateSplitViewContent(splitFoldDiff, oldLineMap, newLineMap, nil, "", "")
	pair := findSplitRow(content, "X")
	l9 := findSplitRow(content, " l9")
	highlight := "[:" + util.SearchHighlightBg + "]"

	for _, cursor := range []int{-1, pair, l9} {
		view := tview.NewTextView().SetDynamicColors(true)
		writeSplitSide(view, content.BeforeLines, content.BeforeLineNums, content.Rows, cursor, -1, -1, false, "l1")
		rows := strings.Split(view.GetText(false), "\n")

		// "l10" on the '-' side of the pair row, "l1" in the context row "l11"
		for _, row := range []int{pair, findSplitRow(content, " l11")} {
			if !strings.Contains(rows[row], highlight) {
				t.Errorf("cursor %d: row %d is not highlighted: %q", cursor, row, rows[row])
			}
		}
		if strings.Contains(rows[l9], highlight) {
			t.Errorf("cursor %d: row l9 has no match but is highlighted: %q", cursor, rows[l9])
		}
	}
}
