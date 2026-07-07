package ui

import (
	"testing"
)

func TestGenerateUnifiedViewContent(t *testing.T) {
	tests := []struct {
		name         string
		diffText     string
		oldLineMap   map[int]int
		newLineMap   map[int]int
		wantContent  []string
		wantLineNums []string
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
			wantContent: []string{
				" line1",
				"[#E7454E]-line2[-]",
				" line3",
			},
			wantLineNums: []string{
				"1 │ ",
				"2 │ ",
				"2 │ ",
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
			wantContent: []string{
				" line1",
				"[#00AC37]+line2[-]",
				" line3",
			},
			wantLineNums: []string{
				"1 │ ",
				"2 │ ",
				"3 │ ",
			},
		},
		{
			name: "変更（削除と追加）",
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
			wantContent: []string{
				" line1",
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]line2[-:-]",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]line2[-:-][#00AC37:#1A4D1A]_modified[-:-]",
				" line3",
			},
			wantLineNums: []string{
				"1 │ ",
				"2 │ ",
				"2 │ ",
				"3 │ ",
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
			// Due to fold feature, a fold indicator is displayed for lines 1-97
			wantContent: []string{
				"[dimgray]... 97 lines hidden (press 'e' to expand) ...[-]",
				" line98",
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]line99[-:-]",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]line99[-:-][#00AC37:#1A4D1A]_modified[-:-]",
				" line100",
			},
			wantLineNums: []string{
				"    │ ",
				" 98 │ ",
				" 99 │ ",
				" 99 │ ",
				"100 │ ",
			},
		},
		{
			name: "ヘッダー行の除外",
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
			wantContent: []string{
				"[#E7454E:#3A0000]-[-:-][#E7454E:#5C1A1A]old[-:-]",
				"[#00AC37:#002500]+[-:-][#00AC37:#1A4D1A]new[-:-]",
			},
			wantLineNums: []string{
				"1 │ ",
				"1 │ ",
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
			wantContent: []string{
				"[#E7454E:#3A0000]-[-:-][#E7454E:#3A0000]var foo [[-:-][#E7454E:#5C1A1A]int[-:-][#E7454E:#3A0000]]string[-:-]",
				"[#00AC37:#002500]+[-:-][#00AC37:#002500]var foo [[-:-][#00AC37:#1A4D1A]white[-:-][#00AC37:#002500]]string[-:-]",
			},
			wantLineNums: []string{
				"1 │ ",
				"1 │ ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := generateUnifiedViewContent(tt.diffText, tt.oldLineMap, tt.newLineMap, nil, "", "")

			// Check number of lines
			if len(content.Lines) != len(tt.wantContent) {
				t.Errorf("Lines count mismatch: got %d, want %d", len(content.Lines), len(tt.wantContent))
			}

			// Check content and line numbers
			for i := range tt.wantContent {
				if i < len(content.Lines) {
					if content.Lines[i].Content != tt.wantContent[i] {
						t.Errorf("Line[%d].Content: got %q, want %q", i, content.Lines[i].Content, tt.wantContent[i])
					}
					if content.Lines[i].LineNumber != tt.wantLineNums[i] {
						t.Errorf("Line[%d].LineNumber: got %q, want %q", i, content.Lines[i].LineNumber, tt.wantLineNums[i])
					}
				}
			}
		})
	}
}

func TestColorizeDiff(t *testing.T) {
	tests := []struct {
		name     string
		diffText string
		want     string
	}{
		{
			name: "基本的な色付け",
			diffText: ` line1
-deleted
+added`,
			want: " line1\n[#E7454E:#3A0000]-[-:-][#E7454E:#5C1A1A]delet[-:-][#E7454E:#3A0000]ed[-:-]\n[#00AC37:#002500]+[-:-][#00AC37:#1A4D1A]add[-:-][#00AC37:#002500]ed[-:-]\n",
		},
		{
			name: "ヘッダー行の除外",
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,1 +1,1 @@
-old
+new`,
			want: "[#E7454E:#3A0000]-[-:-][#E7454E:#5C1A1A]old[-:-]\n[#00AC37:#002500]+[-:-][#00AC37:#1A4D1A]new[-:-]\n",
		},
		{
			name:     "空のdiff",
			diffText: "",
			want:     "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ColorizeDiff(tt.diffText)
			if got != tt.want {
				t.Errorf("ColorizeDiff() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsUnifiedHeaderLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"diff --git", "diff --git a/file b/file", true},
		{"index", "index 123..456 789", true},
		{"---", "--- a/file", true},
		{"+++", "+++ b/file", true},
		{"ハンクヘッダー", "@@ -1,3 +1,3 @@", true},
		{"通常の行", " normal line", false},
		{"削除行", "-deleted line", false},
		{"追加行", "+added line", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnifiedHeaderLine(tt.line); got != tt.want {
				t.Errorf("isUnifiedHeaderLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestColorizeLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"削除行", "-deleted", "[#E7454E]-deleted[-]"},
		{"追加行", "+added", "[#00AC37]+added[-]"},
		{"通常の行", " normal", " normal"},
		{"空行", "", ""},
		{"その他の行", "other", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colorizeLineFallback(tt.line); got != tt.want {
				t.Errorf("colorizeLineFallback(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestDetectFoldableRanges(t *testing.T) {
	tests := []struct {
		name       string
		diffText   string
		minGap     int
		totalLines int
		want       []FoldableRange
	}{
		{
			name: "deletions followed by context and more deletions should not produce a mid fold",
			// Without the fix, the gap between the context line (new=182) and the
			// following `-` line (old=190) was computed as 7, producing a spurious
			// fold that expanded arbitrary lines from the new file.
			diffText: `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -180,14 +180,4 @@
 ctx1
 ctx2
-del1
-del2
-del3
-del4
-del5
-del6
-del7
 ctx3
-del8
-del9
 ctx4`,
			minGap:     3,
			totalLines: 0,
			want: []FoldableRange{
				{StartLine: 1, EndLine: 179, InsertAt: -1, LineCount: 179, ID: "fold-top-1-179"},
			},
		},
		{
			name: "single hunk with large initial gap produces top fold only",
			diffText: `diff --git a/test.txt b/test.txt
@@ -100,3 +100,3 @@
 ctx
-old
+new`,
			minGap:     3,
			totalLines: 0,
			want: []FoldableRange{
				{StartLine: 1, EndLine: 99, InsertAt: -1, LineCount: 99, ID: "fold-top-1-99"},
			},
		},
		{
			name: "multi-hunk diff produces mid fold between hunks",
			diffText: `diff --git a/test.txt b/test.txt
@@ -1,3 +1,3 @@
 a
-b
+B
@@ -50,3 +50,3 @@
 x
-y
+Y`,
			minGap:     3,
			totalLines: 0,
			want: []FoldableRange{
				{StartLine: 3, EndLine: 49, InsertAt: 2, LineCount: 47, ID: "fold-3-49"},
			},
		},
		{
			// Real `git diff` output ends with a newline. Splitting on "\n"
			// yields a trailing empty element that must not be tracked as a
			// context line, otherwise newLineNum overshoots and the bottom
			// fold drops one boundary line on expansion.
			name: "trailing newline does not shift bottom fold start",
			diffText: `diff --git a/file.txt b/file.txt
@@ -10,9 +10,8 @@
 line10
 line11
 line12
-line13
-line14
-line15
+NEW_A
+NEW_B
 line16
 line17
 line18
`,
			minGap:     3,
			totalLines: 29,
			want: []FoldableRange{
				{StartLine: 1, EndLine: 9, InsertAt: -1, LineCount: 9, ID: "fold-top-1-9"},
				{StartLine: 18, EndLine: 29, InsertAt: -2, LineCount: 12, ID: "fold-bottom-18-29"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFoldableRanges(tt.diffText, tt.minGap, tt.totalLines)
			if len(got) != len(tt.want) {
				t.Fatalf("ranges count: got %d %+v, want %d %+v", len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("range[%d]: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGenerateLineNumber(t *testing.T) {
	tests := []struct {
		name       string
		lineType   byte
		index      int
		maxDigits  int
		oldLineMap map[int]int
		newLineMap map[int]int
		want       string
	}{
		{
			name:       "削除行",
			lineType:   '-',
			index:      0,
			maxDigits:  2,
			oldLineMap: map[int]int{0: 10},
			newLineMap: map[int]int{},
			want:       "10 │ ",
		},
		{
			name:       "追加行",
			lineType:   '+',
			index:      1,
			maxDigits:  2,
			oldLineMap: map[int]int{},
			newLineMap: map[int]int{1: 11},
			want:       "11 │ ",
		},
		{
			name:       "共通行（新しい行番号）",
			lineType:   ' ',
			index:      2,
			maxDigits:  3,
			oldLineMap: map[int]int{},
			newLineMap: map[int]int{2: 100},
			want:       "100 │ ",
		},
		{
			name:       "共通行（古い行番号）",
			lineType:   ' ',
			index:      3,
			maxDigits:  3,
			oldLineMap: map[int]int{3: 101},
			newLineMap: map[int]int{},
			want:       "101 │ ",
		},
		{
			name:       "行番号なし",
			lineType:   'o',
			index:      4,
			maxDigits:  2,
			oldLineMap: map[int]int{},
			newLineMap: map[int]int{},
			want:       "   │ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateLineNumber(tt.lineType, tt.index, tt.maxDigits, tt.oldLineMap, tt.newLineMap)
			if got != tt.want {
				t.Errorf("generateLineNumber() = %q, want %q", got, tt.want)
			}
		})
	}
}
