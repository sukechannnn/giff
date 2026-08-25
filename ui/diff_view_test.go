package ui

import (
	"reflect"
	"testing"
)

func TestExtractSplitCopyLines(t *testing.T) {
	diffText := `diff --git a/test.txt b/test.txt
index 123..456 789
--- a/test.txt
+++ b/test.txt
@@ -1,4 +1,4 @@
 line1
-line2
+line2_modified
 line3
+line4`

	oldLineMap := map[int]int{
		0: 1,
		1: 2,
		3: 3,
	}
	newLineMap := map[int]int{
		0: 1,
		2: 2,
		3: 3,
		4: 4,
	}

	content := generateSplitViewContent(diffText, oldLineMap, newLineMap, "", "")

	tests := []struct {
		name       string
		start      int
		end        int
		wantBefore []string
		wantAfter  []string
	}{
		{
			name:       "全行選択",
			start:      0,
			end:        3,
			wantBefore: []string{"line1", "line2", "line3"},
			wantAfter:  []string{"line1", "line2_modified", "line3", "line4"},
		},
		{
			name:       "変更行のみ選択",
			start:      1,
			end:        1,
			wantBefore: []string{"line2"},
			wantAfter:  []string{"line2_modified"},
		},
		{
			name:       "追加行のみ選択 (before はプレースホルダーのため空)",
			start:      3,
			end:        3,
			wantBefore: nil,
			wantAfter:  []string{"line4"},
		},
		{
			name:       "変更なし行のみ選択",
			start:      0,
			end:        0,
			wantBefore: []string{"line1"},
			wantAfter:  []string{"line1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := extractSplitCopyLines(content, tt.start, tt.end)
			if !reflect.DeepEqual(before, tt.wantBefore) {
				t.Errorf("beforeLines: got %q, want %q", before, tt.wantBefore)
			}
			if !reflect.DeepEqual(after, tt.wantAfter) {
				t.Errorf("afterLines: got %q, want %q", after, tt.wantAfter)
			}
		})
	}
}

func TestStripSplitLinePrefix(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"削除プレフィックス", "-deleted", "deleted"},
		{"追加プレフィックス", "+added", "added"},
		{"コンテキストプレフィックス", " context", "context"},
		{"空行", "", ""},
		{"プレフィックスなし", "other", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripSplitLinePrefix(tt.line); got != tt.want {
				t.Errorf("stripSplitLinePrefix(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestStripTviewTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "style tags are removed",
			input: "[green]+added line[-]",
			want:  "+added line",
		},
		{
			name:  "fg:bg tag and reset tag are removed",
			input: "[black:green]+[-:-]content",
			want:  "+content",
		},
		{
			name:  "hex color tags are removed",
			input: "[dimgray:#3a3a3a] folded[-:-]",
			want:  " folded",
		},
		{
			name:  "shell test brackets are preserved",
			input: `          if [ -n "$AFFECTED" ]; then`,
			want:  `          if [ -n "$AFFECTED" ]; then`,
		},
		{
			name:  "empty brackets are preserved",
			input: "var s []string",
			want:  "var s []string",
		},
		{
			name:  "numeric index brackets are preserved",
			input: "arr[0] = 1",
			want:  "arr[0] = 1",
		},
		{
			name:  "escaped tag is unescaped",
			input: "arr[i[]",
			want:  "arr[i]",
		},
		{
			name:  "double-escaped tag drops one bracket",
			input: "[xyz[[]",
			want:  "[xyz[]",
		},
		{
			name:  "region-like quoted brackets from escape are restored",
			input: `d["key"[]`,
			want:  `d["key"]`,
		},
		{
			name:  "mixed tags and literal brackets",
			input: `[green]+  if [ -n "$AFFECTED" ]; then[-]`,
			want:  `+  if [ -n "$AFFECTED" ]; then`,
		},
		{
			name:  "attribute tag is removed",
			input: "[::b]bold[::-]",
			want:  "bold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripTviewTags(tt.input); got != tt.want {
				t.Errorf("stripTviewTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHighlightSearchInTaggedTextPreservesLiteralBrackets(t *testing.T) {
	tagged := `[green]+if [ -n "$AFFECTED" ]; then[-]`
	got := highlightSearchInTaggedText(tagged, "AFFECTED")
	if stripped := stripTviewTags(got); stripped != `+if [ -n "$AFFECTED" ]; then` {
		t.Errorf("highlighted text lost literal brackets: %q (stripped %q)", got, stripped)
	}
}
