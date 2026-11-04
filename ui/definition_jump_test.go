package ui

import (
	"reflect"
	"testing"

	"github.com/sukechannnn/giff/git"
)

// The definitions in the file being read come first, then those next to it
func TestSortDefinitions(t *testing.T) {
	defs := []git.Location{
		{Path: "a/x.go", Line: 1},
		{Path: "b/y.go", Line: 2},
		{Path: "b/z.go", Line: 3},
		{Path: "a/x.go", Line: 4},
	}
	sortDefinitions(defs, "b/z.go")
	var got []int
	for _, d := range defs {
		got = append(got, d.Line)
	}
	if want := []int{3, 2, 1, 4}; !reflect.DeepEqual(got, want) {
		t.Errorf("sorted lines = %v, want %v", got, want)
	}
}

// Only the definitions in the language of the file being read are offered
func TestSameLanguageDefinitions(t *testing.T) {
	defs := []git.Location{
		{Path: "app/a.rb", Line: 1},
		{Path: "lib/b.py", Line: 2},
		{Path: "Rakefile", Line: 3},
		{Path: "web/c.tsx", Line: 4},
	}
	lines := func(defs []git.Location) []int {
		var got []int
		for _, d := range defs {
			got = append(got, d.Line)
		}
		return got
	}
	tests := []struct {
		currentFile string
		want        []int
	}{
		{"app/x.rb", []int{1, 3}},
		{"web/x.js", []int{4}},
		{"app/views/x.html.erb", []int{1, 3}},
		{"x.go", nil},
	}
	for _, tt := range tests {
		if got := lines(sameLanguageDefinitions(defs, tt.currentFile)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: lines = %v, want %v", tt.currentFile, got, tt.want)
		}
	}
}
