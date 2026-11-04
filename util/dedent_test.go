package util

import (
	"reflect"
	"testing"
)

func TestDedent(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{"single line", []string{"\t\treturn nil"}, []string{"return nil"}},
		{"shallowest line sets the base",
			[]string{"\tif x {", "\t\treturn nil", "\t}"},
			[]string{"if x {", "\treturn nil", "}"}},
		{"spaces",
			[]string{"    def foo", "      bar", "    end"},
			[]string{"def foo", "  bar", "end"}},
		{"blank lines don't count",
			[]string{"\t\tfoo()", "", "  \t", "\t\tbar()"},
			[]string{"foo()", "", "", "bar()"}},
		{"tabs and spaces share only what matches",
			[]string{"\t\tfoo()", "\t    bar()"},
			[]string{"\tfoo()", "    bar()"}},
		{"nothing to remove",
			[]string{"func f() {", "\treturn", "}"},
			[]string{"func f() {", "\treturn", "}"}},
	}
	for _, tt := range tests {
		if got := Dedent(tt.lines); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Dedent(%q) = %q, want %q", tt.name, tt.lines, got, tt.want)
		}
	}
}
