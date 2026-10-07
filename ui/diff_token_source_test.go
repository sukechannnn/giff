package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

// dracula の文字列色。hunk 冒頭の閉じ """ を開きと誤認すると
// hunk 全体がこの色で塗られてしまう (リグレッション対象)。
const stringColor = "#f1fa8d"

const midDocstringNewFile = `"""Doc.

text
"""

import os

x = 2
`

const midDocstringDiff = "diff --git a/test.py b/test.py\n" +
	"index 0000000..1111111 100644\n" +
	"--- a/test.py\n" +
	"+++ b/test.py\n" +
	"@@ -4,5 +4,5 @@\n" +
	" \"\"\"\n" +
	" \n" +
	" import os\n" +
	" \n" +
	"-x = 1\n" +
	"+x = 2\n"

func writeMidDocstringFile(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "test.py"), []byte(midDocstringNewFile), 0o644); err != nil {
		t.Fatal(err)
	}
	return repoRoot
}

// docstring の途中から始まる hunk でも、import 行が文字列としてではなく
// コードとしてハイライトされること
func TestUnifiedViewHighlightsHunkStartingMidDocstring(t *testing.T) {
	repoRoot := writeMidDocstringFile(t)

	oldLineMap, newLineMap := createLineNumberMapping(midDocstringDiff)
	content := generateUnifiedViewContent(midDocstringDiff, oldLineMap, newLineMap, nil, "test.py", repoRoot)

	var importLine, deletedLine string
	for _, line := range content.Lines {
		if strings.Contains(line.Content, "import") {
			importLine = line.Content
		}
		if line.LineType == '-' {
			deletedLine = line.Content
		}
	}

	if importLine == "" {
		t.Fatal("import 行が見つからない")
	}
	if strings.Contains(importLine, stringColor) {
		t.Errorf("import 行が文字列としてハイライトされている: %s", importLine)
	}
	if !strings.Contains(importLine, "[#") {
		t.Errorf("import 行がシンタックスハイライトされていない: %s", importLine)
	}

	// 削除行は再構築した旧ファイルからトークナイズされる
	if deletedLine == "" {
		t.Fatal("削除行が見つからない")
	}
	if strings.Contains(deletedLine, stringColor) {
		t.Errorf("削除行が文字列としてハイライトされている: %s", deletedLine)
	}
}

// split view でも同様にハイライトされること
func TestSplitViewHighlightsHunkStartingMidDocstring(t *testing.T) {
	repoRoot := writeMidDocstringFile(t)

	oldLineMap, newLineMap := createLineNumberMapping(midDocstringDiff)
	content := generateSplitViewContent(midDocstringDiff, oldLineMap, newLineMap, nil, "test.py", repoRoot)

	// 閉じ """ の行は正当に文字列色になるため、import 行だけを検証する
	for _, lines := range [][]string{content.BeforeLines, content.AfterLines} {
		found := false
		for _, line := range lines {
			if !strings.Contains(line, "import") {
				continue
			}
			found = true
			if strings.Contains(line, stringColor) {
				t.Errorf("import 行が文字列としてハイライトされている: %s", line)
			}
			if !strings.Contains(line, "[#") {
				t.Errorf("import 行がシンタックスハイライトされていない: %s", line)
			}
		}
		if !found {
			t.Fatal("import 行が見つからない")
		}
	}
}

// 大きなファイルの末尾付近の hunk では、ファイル先頭からではなく hunk の
// 手前だけをトークナイズしつつ、docstring の途中から始まる hunk も正しく
// ハイライトされること
func TestLineTokensLargeFileTokenizesOnlyAroundHunks(t *testing.T) {
	const filler = 2000
	var sb strings.Builder
	for i := 0; i < filler; i++ {
		sb.WriteString("y = 0\n")
	}
	sb.WriteString(midDocstringNewFile)
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "test.py"), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := strings.Replace(midDocstringDiff, "@@ -4,5 +4,5 @@", "@@ -2004,5 +2004,5 @@", 1)

	oldLineMap, newLineMap := createLineNumberMapping(diff)
	source := newDiffTokenSource(diff, "test.py", repoRoot, oldLineMap, newLineMap)

	// displayIdx 2 = "import os"
	tokens := source.lineTokens(2, ' ', "import os")
	if len(tokens) == 0 {
		t.Fatal("import 行のトークンが返らない")
	}
	for _, tok := range tokens {
		if tok.Type.InCategory(chroma.LiteralString) {
			t.Errorf("import 行が文字列としてトークナイズされている: %v", tokens)
		}
	}

	if source.newFile.done[0] || source.oldFile.done[0] {
		t.Error("hunk から離れたファイル先頭までトークナイズされている")
	}

	// 展開した fold の行は必要になった時点でトークナイズされる
	if tokens := source.newFileLineTokens(1, "y = 0"); len(tokens) == 0 {
		t.Error("fold 展開行のトークンが返らない")
	}
}

// ファイルが読めない場合は従来どおり diff 本体のトークナイズにフォールバックすること
func TestLineTokensFallsBackWithoutFile(t *testing.T) {
	oldLineMap, newLineMap := createLineNumberMapping(midDocstringDiff)
	source := newDiffTokenSource(midDocstringDiff, "test.py", "", oldLineMap, newLineMap)

	// displayIdx 2 = "import os" (フォールバックでは hunk 冒頭の """ が
	// 開きと解釈されるため文字列トークンになる。従来挙動の維持を確認)
	tokens := source.lineTokens(2, ' ', "import os")
	if len(tokens) == 0 {
		t.Fatal("フォールバックのトークンが返らない")
	}
}

func TestReconstructOldFileLines(t *testing.T) {
	newLines := strings.Split(midDocstringNewFile, "\n")
	old := reconstructOldFileLines(midDocstringDiff, newLines)

	want := []string{
		`"""Doc.`,
		``,
		`text`,
		`"""`,
		``,
		`import os`,
		``,
		`x = 1`,
		``, // 末尾改行による空要素
	}
	if !reflect.DeepEqual(old, want) {
		t.Errorf("旧ファイルの再構築結果が不一致:\ngot:  %q\nwant: %q", old, want)
	}
}

// diff がファイル内容と一致しない場合 (watch モードのレースなど) に
// 不正なトークンを返さないこと
func TestReconstructOldFileLinesMismatch(t *testing.T) {
	newLines := []string{"totally", "different", "content"}
	// 新ファイル側の行数と hunk ヘッダが噛み合わないケース
	diff := "diff --git a/test.py b/test.py\n--- a/test.py\n+++ b/test.py\n@@ -10,3 +10,3 @@\n a\n-b\n+c\n"
	if got := reconstructOldFileLines(diff, newLines); got != nil {
		t.Errorf("不整合な diff で nil が返らない: %q", got)
	}
}
