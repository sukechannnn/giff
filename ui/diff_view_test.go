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

	content := generateSplitViewContent(diffText, oldLineMap, newLineMap, "")

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
