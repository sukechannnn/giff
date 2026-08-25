package util

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestTokenizeCode(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		code     []string
		wantNil  bool
	}{
		{
			name:     "Goファイルのトークン化",
			filePath: "main.go",
			code:     []string{"func main() {", "	fmt.Println(\"hello\")", "}"},
			wantNil:  false,
		},
		{
			name:     "未知の拡張子",
			filePath: "file.unknown_ext_xyz",
			code:     []string{"just some text"},
			wantNil:  true,
		},
		{
			name:     "空のコード",
			filePath: "empty.go",
			code:     []string{""},
			wantNil:  false,
		},
		{
			name:     "Pythonファイル",
			filePath: "script.py",
			code:     []string{"def hello():", "    print('world')"},
			wantNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TokenizeCode(tt.filePath, tt.code)
			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got tokens")
			}
			if !tt.wantNil && result == nil {
				t.Errorf("expected tokens, got nil")
			}
			if !tt.wantNil && result != nil {
				if len(result) != len(tt.code) {
					t.Errorf("expected %d lines of tokens, got %d", len(tt.code), len(result))
				}
			}
		})
	}
}

func TestTokenizeCodeCache(t *testing.T) {
	code := []string{"func foo() {}"}
	result1 := TokenizeCode("test.go", code)
	result2 := TokenizeCode("test.go", code)

	if result1 == nil || result2 == nil {
		t.Fatal("expected non-nil results")
	}
	if len(result1) != len(result2) {
		t.Errorf("cache miss: different lengths %d vs %d", len(result1), len(result2))
	}
}

func TestRenderHighlightedLine(t *testing.T) {
	tests := []struct {
		name    string
		tokens  []chroma.Token
		bgColor string
	}{
		{
			name:    "空トークン",
			tokens:  []chroma.Token{},
			bgColor: "",
		},
		{
			name: "キーワードトークン（背景なし）",
			tokens: []chroma.Token{
				{Type: chroma.Keyword, Value: "func"},
				{Type: chroma.Text, Value: " "},
				{Type: chroma.NameFunction, Value: "main"},
			},
			bgColor: "",
		},
		{
			name: "追加行背景",
			tokens: []chroma.Token{
				{Type: chroma.Keyword, Value: "func"},
				{Type: chroma.Text, Value: " "},
				{Type: chroma.NameFunction, Value: "main"},
			},
			bgColor: AddedLineBg,
		},
		{
			name: "削除行背景",
			tokens: []chroma.Token{
				{Type: chroma.LiteralString, Value: "\"hello\""},
			},
			bgColor: DeletedLineBg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderHighlightedLine(tt.tokens, tt.bgColor)

			if len(tt.tokens) == 0 {
				// Empty tokens should produce empty string or a minimal bg-only tag
				return
			}

			// Result should contain the token values
			for _, tok := range tt.tokens {
				if !strings.Contains(result, tok.Value) {
					t.Errorf("result %q does not contain token value %q", result, tok.Value)
				}
			}

			// If bgColor is set, result should contain the bg color
			if tt.bgColor != "" {
				if !strings.Contains(result, tt.bgColor) {
					t.Errorf("result %q does not contain bgColor %q", result, tt.bgColor)
				}
			}
		})
	}
}

func TestReplaceBackground(t *testing.T) {
	tests := []struct {
		name  string
		input string
		newBg string
		want  string
	}{
		{
			name:  "fg:bg タグの背景差し替え",
			input: "[#c678dd:#2a3a2a]func[-:-]",
			want:  "[#c678dd:blue]func[-:blue]",
			newBg: "blue",
		},
		{
			name:  "fg のみタグに背景追加",
			input: "[red]text[-]",
			want:  "[red:blue]text[-:blue]",
			newBg: "blue",
		},
		{
			name:  "タグなし文字列",
			input: "plain text",
			want:  "plain text",
			newBg: "blue",
		},
		{
			name:  "複数タグ",
			input: "[#c678dd:#2a3a2a]func[-:-] [#61afef:]main[-:-]",
			want:  "[#c678dd:dimgrey]func[-:dimgrey] [#61afef:dimgrey]main[-:dimgrey]",
			newBg: "dimgrey",
		},
		{
			name:  "シェルの test 括弧は書き換えない",
			input: `[red]if [ -n "$AFFECTED" ]; then[-]`,
			want:  `[red:blue]if [ -n "$AFFECTED" ]; then[-:blue]`,
			newBg: "blue",
		},
		{
			name:  "空括弧・数字添字は書き換えない",
			input: "[green]var s []string = arr[0][-]",
			want:  "[green:blue]var s []string = arr[0][-:blue]",
			newBg: "blue",
		},
		{
			name:  "エスケープ済みタグはそのまま",
			input: "[white]arr[i[] = 1[-]",
			want:  "[white:blue]arr[i[] = 1[-:blue]",
			newBg: "blue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplaceBackground(tt.input, tt.newBg)
			if got != tt.want {
				t.Errorf("ReplaceBackground(%q, %q) = %q, want %q", tt.input, tt.newBg, got, tt.want)
			}
		})
	}
}
