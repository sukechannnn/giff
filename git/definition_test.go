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
		lang string
		line string
		want bool
	}{
		{"go", "func Foo() {", true},
		{"go", "func (r *Repo) Foo(x int) error {", true},
		{"go", "type Foo struct {", true},
		{"go", "const Foo = 1", true},
		{"python", "def Foo(self):", true},
		{"ruby", "  def self.Foo", true},
		{"ruby", "class Foo < Base", true},
		{"js", "export default async function Foo() {", true},
		{"js", "export const Foo = () => {}", true},
		{"js", "export interface Foo {", true},
		{"rust", "pub(crate) fn Foo() {", true},
		{"go", "	return Foo()", false},
		{"go", "// Foo does things", false},
		{"go", "func FooBar() {", false},
		{"go", "x := Foo", false},
		{"ruby", "  FOO_BAR = 1", false},
		{"ruby", "  attr_reader :foo, :Foo", true},
		{"ruby", "  attr_accessor(:Foo)", true},
		{"ruby", "  delegate :bar, to: :Foo", false},
		{"ruby", "  let!(:Foo) { create(:user) }", true},
		{"ruby", "  subject(:Foo) { described_class.new }", true},
		{"ruby", "  alias_method :Foo, :bar", true},
		{"ruby", "  alias_method :bar, :Foo", false},
		{"ruby", "  alias Foo bar", true},
		{"ruby", "  define_method(:Foo) do", true},
		{"ruby", "  scope :Foo, -> { where(x: 1) }", true},
		{"ruby", "  has_many :Foo, dependent: :destroy", true},
		{"ruby", "  belongs_to :Foo", true},
		{"ruby", "  create(:Foo)", false},
		// A constant, and nothing that only reads or compares it
		{"ruby", "  Foo = Struct.new(:a)", true},
		{"ruby", "Admin::Foo ||= 1", true},
		{"ruby", "  Foo == 1", false},
		{"ruby", "  Foo =~ /x/", false},
		{"ruby", "  { Foo => 1 }", false},
		// The patterns of one language never count in another
		{"ruby", "function Foo() {", false},
		{"go", "def Foo", false},
		{"python", "  def self.Foo", false},
		{"python", "  scope :Foo", false},
		{"js", "  attr_reader :Foo", false},
	}
	for _, tt := range tests {
		if got := definitionRegexp(tt.lang, "Foo").MatchString(tt.line); got != tt.want {
			t.Errorf("%s %q: match = %v, want %v", tt.lang, tt.line, got, tt.want)
		}
	}
}

// A lowercase name is never a constant, so assigning to it does not count
func TestDefinitionRegexpLocalVariable(t *testing.T) {
	if definitionRegexp("ruby", "foo").MatchString("  foo = 1") {
		t.Error("an assignment to a local variable counts as its definition")
	}
	if !definitionRegexp("ruby", "foo").MatchString("  attr_reader :foo") {
		t.Error("attr_reader of a lowercase name does not count")
	}
}

// The lines of a language not known never count
func TestDefinitionRegexpUnknownLanguage(t *testing.T) {
	for _, lang := range []string{".java", ""} {
		if def := definitionRegexp(lang, "Foo"); def != nil {
			t.Errorf("%q: regexp = %v, want none", lang, def)
		}
	}
}

func TestLanguageOf(t *testing.T) {
	tests := map[string]string{
		"app/a.rb":     "ruby",
		"lib/Rakefile": "ruby",
		"web/c.TSX":    "js",
		"x.mjs":        "js",
		"main.go":      "go",
		"Foo.java":     ".java",
		"Makefile":     "",
	}
	for path, want := range tests {
		if got := LanguageOf(path); got != want {
			t.Errorf("LanguageOf(%q) = %q, want %q", path, got, want)
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
	write("sub/c.go", "package sub\n\nvar s = `\ndef Foo\n`\n")
	// No language giff knows of
	write("d.java", "class Foo {}\n")
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
