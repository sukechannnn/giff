package ui

import (
	"testing"
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
			// Pairing logic: deletion and addition lines are displayed on the same row
			wantBefore: []string{
				" line98",
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]line99[-:-]",
				" line100",
			},
			wantAfter: []string{
				" line98",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]line99[-:-][#00AC37:#1A4D1A]_modified[-:-]",
				" line100",
			},
			wantBeforeNums: []string{
				" 98",
				" 99",
				"100",
			},
			wantAfterNums: []string{
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
			content := generateSplitViewContent(tt.diffText, tt.oldLineMap, tt.newLineMap, "")

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
