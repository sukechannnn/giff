package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefinitionRegexp(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"func Foo() {", true},
		{"func (r *Repo) Foo(x int) error {", true},
		{"type Foo struct {", true},
		{"const Foo = 1", true},
		{"def Foo(self):", true},
		{"  def self.Foo", true},
		{"class Foo < Base", true},
		{"export default async function Foo() {", true},
		{"export const Foo = () => {}", true},
		{"export interface Foo {", true},
		{"pub(crate) fn Foo() {", true},
		{"	return Foo()", false},
		{"// Foo does things", false},
		{"func FooBar() {", false},
		{"x := Foo", false},
	}
	def := definitionRegexp("Foo")
	for _, tt := range tests {
		if got := def.MatchString(tt.line); got != tt.want {
			t.Errorf("%q: match = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestFindDefinitions(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	write("a.go", "package a\n\nfunc Foo() {}\n\nfunc bar() { Foo() }\n")
	write("sub/b.go", "package sub\n\n// Foo is not here\ntype Foo int\n")
	run("add", "a.go")
	// sub/b.go stays untracked

	got, err := FindDefinitions(dir, "Foo")
	if err != nil {
		t.Fatal(err)
	}
	want := []Location{
		{Path: "a.go", Line: 3, Text: "func Foo() {}"},
		{Path: "sub/b.go", Line: 4, Text: "type Foo int"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FindDefinitions = %+v, want %+v", got, want)
	}

	if got, err := FindDefinitions(dir, "missing"); err != nil || got != nil {
		t.Errorf("FindDefinitions(missing) = %+v, %v; want nothing", got, err)
	}
}
