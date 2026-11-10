package git

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Location is a line of a file in the repository
type Location struct {
	Path string // relative to the repository root
	Line int    // 1-based
	Text string // the line itself
}

// languageExtensions maps an extension to the key of its language, the same
// key for every extension of one. Any other extension is a language of its
// own, with the extension for its key.
var languageExtensions = map[string]string{
	".go": "go", ".rs": "rust", ".c": "c", ".h": "c",
	".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".ts": "js", ".tsx": "js", ".mts": "js", ".cts": "js", ".vue": "js", ".svelte": "js",
	".rb": "ruby", ".rake": "ruby", ".ru": "ruby", ".gemspec": "ruby", ".erb": "ruby",
	".py": "python", ".pyi": "python",
}

// languageFileNames are the files without an extension a language is known by
var languageFileNames = map[string]string{
	"Rakefile": "ruby", "Gemfile": "ruby",
}

// LanguageOf returns the key of the language path is written in, or "" when
// it cannot tell
func LanguageOf(path string) string {
	if lang, ok := languageFileNames[filepath.Base(path)]; ok {
		return lang
	}
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := languageExtensions[ext]; ok {
		return lang
	}
	return ext
}

// A definition is found on a line of one of four kinds, each of which a
// language or a framework spells out on its own:
//
//  1. a declaration led by a keyword, such as def foo or func Foo
//  2. a constant, an assignment whose name says it is one, such as FOO = 1
//  3. a member of a class, declared without a keyword of its own, such as a
//     method foo() { in JavaScript, or made by a call of the language, such
//     as attr_reader :foo in Ruby
//  4. one made by a framework, such as scope :foo in Rails
//
// The patterns have NAME where the name goes. They only look at the start of
// the line, so a call or a mention of the name never counts.

// language is how the definitions of one language read
type language struct {
	declarations []string // 1
	constants    []string // 2, used when isConstant says the name is one
	isConstant   func(string) bool
	members      []string  // 3, declared without a keyword
	definers     []definer // 3, made by a call

	// notDefinitions match the lines that never define anything, such as an
	// if statement, which a pattern of a member could take for one
	notDefinitions []string

	// firstArgCall and everyArgCall are how a call naming what it defines by
	// its first argument, or by each of its leading ones, reads. CALL is where
	// the name of the call goes and NAME where the defined name goes. The
	// definers of a language without them are left out.
	firstArgCall string
	everyArgCall string
}

// definer is the calls that define what their arguments name
type definer struct {
	calls    []string
	everyArg bool // attr_reader :a, :b defines both, alias_method :a, :b only a
}

// framework is what defines things in a framework, which counts in the files
// of its language
type framework struct {
	language string
	patterns []string  // lines defining NAME, such as foo = models.CharField()
	definers []definer // calls defining what their arguments name
}

// jsModifiers are the words that can come before a class member in
// JavaScript and TypeScript
const jsModifiers = `(static|async|get|set|public|private|protected|readonly|override|abstract|declare|accessor)`

// languages are the languages known by their LanguageOf key
var languages = map[string]language{
	"go": {
		declarations: []string{
			`^\s*func\s+(\([^)]*\)\s*)?NAME\b`,
			`^\s*(type|var|const)\s+NAME\b`,
		},
	},
	"python": {
		declarations: []string{
			`^\s*(async\s+)?def\s+NAME\b`,
			`^\s*class\s+NAME\b`,
		},
		// An assignment at the top of a module, which makes a global of it
		constants: []string{`^NAME\s*(:[^=]+)?=([^=]|$)`},
		members: []string{
			// An annotated field of a class, such as a dataclass. A parameter
			// of a function split over lines ends with a comma.
			`^\s+NAME\s*:\s*[A-Za-z_'"](.*[^,\s])?\s*$`,
		},
		notDefinitions: []string{
			`^\s*(if|elif|else|for|while|try|except|finally|with|lambda|return|match|case)\b`,
		},
	},
	"ruby": {
		declarations: []string{
			`^\s*def\s+(self\.)?NAME\b`,
			`^\s*(class|module)\s+NAME\b`,
			`^\s*alias\s+:?NAME\b`,
		},
		constants:  []string{`^\s*(\w+::)*NAME\s*(\|\|)?=([^=~>]|$)`},
		isConstant: startsUpper,
		definers: []definer{
			{calls: []string{"attr_reader", "attr_writer", "attr_accessor"}, everyArg: true},
			{calls: []string{"alias_method", "define_method"}},
		},
		firstArgCall: `^\s*(CALL)\s*\(?\s*[:'"]NAME\b`,
		everyArgCall: `^\s*(CALL)\s*\(?\s*([:'"]\w+[?!]?['"]?\s*,\s*)*[:'"]NAME\b`,
	},
	"js": {
		declarations: []string{
			`^\s*(export\s+)?(default\s+)?(async\s+)?function\s*\*?\s*NAME\b`,
			`^\s*(export\s+)?(default\s+)?(abstract\s+)?(class|interface|type|enum)\s+NAME\b`,
			`^\s*(export\s+)?(const|let|var)\s+NAME\b`,
		},
		members: []string{
			// A method of a class or an object: foo(a, b) {, or foo( with the
			// parameters on the lines below after a modifier, as a call can
			// spill over lines too. A $ in the parameters is a GraphQL query
			// in a string, such as getJob(id: $id) {.
			`^\s*(` + jsModifiers + `\s+)*\*?\s*#?NAME\s*[?!]?\s*(<[^>]*>)?\s*\([^()$]*\)\s*(:\s*[^={;]+)?\s*\{\s*$`,
			`^\s*(` + jsModifiers + `\s+)+\*?\s*#?NAME\s*(<[^>]*>)?\s*\(\s*$`,
			// A field: with a modifier, private, typed or holding a function
			`^\s*(` + jsModifiers + `\s+)+#?NAME\s*[?!]?\s*(:[^=;]+)?\s*(=|;|$)`,
			`^\s*#NAME\s*(=|;|$)`,
			`^\s*#?NAME\s*[?!]?\s*:\s*[^;,]+;\s*$`,
			`^\s*(` + jsModifiers + `\s+)*#?NAME\s*(:[^=]+)?=\s*(async\s+)?(\([^()]*\)|\w+)\s*(:\s*[^=]+)?=>`,
			// A function in an object: foo: function (, foo: (a) =>
			`^\s*NAME\s*:\s*(async\s+)?(function\b|\([^()]*\)\s*(:\s*[^=]+)?=>|\w+\s*=>)`,
		},
		notDefinitions: []string{
			`^\s*(return|if|for|while|switch|catch|else|new|await|throw|typeof|case|do|yield|delete|void)\b`,
		},
	},
	"rust": {
		declarations: []string{
			`^\s*(pub(\([^)]*\))?\s+)?(async\s+)?(fn|struct|enum|trait|type|const|static|mod)\s+NAME\b`,
		},
	},
}

// frameworks count whether or not the repository uses them: their calls are
// hardly ever a call of something else
var frameworks = map[string]framework{
	"rails": {
		language: "ruby",
		definers: []definer{
			{calls: []string{"scope", "has_many", "has_one", "belongs_to", "has_and_belongs_to_many"}},
			{calls: []string{"delegate"}, everyArg: true},
		},
	},
	"factory_bot": {
		language: "ruby",
		definers: []definer{
			{calls: []string{"factory", "trait", "sequence"}},
		},
	},
	"django": {
		language: "python",
		patterns: []string{
			`^\s+NAME(\s*:[^=]+)?\s+=\s*(models|serializers|forms|filters)\.[A-Z]\w*\(`,
		},
	},
	"sqlalchemy": {
		language: "python",
		patterns: []string{
			`^\s+NAME(\s*:[^=]+)?\s+=\s*(\w+\.)?(Column|mapped_column|relationship|column_property|synonym)\(`,
		},
	},
	"rspec": {
		language: "ruby",
		definers: []definer{
			{calls: []string{"let", "let!", "subject"}},
		},
	},
}

// startsUpper reports whether name starts with an uppercase letter
func startsUpper(name string) bool {
	return name != "" && unicode.IsUpper([]rune(name)[0])
}

// sortedKeys returns the keys of m in order, so the regexps built from a map
// come out the same each time
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// definitionPatterns returns the patterns of a line defining name in lang,
// with the name in them
func definitionPatterns(lang, name string) []string {
	l := languages[lang]
	patterns := append([]string{}, l.declarations...)
	if l.isConstant == nil || l.isConstant(name) {
		patterns = append(patterns, l.constants...)
	}
	patterns = append(patterns, l.members...)
	definers := append([]definer{}, l.definers...)
	for _, key := range sortedKeys(frameworks) {
		if f := frameworks[key]; f.language == lang {
			patterns = append(patterns, f.patterns...)
			definers = append(definers, f.definers...)
		}
	}
	for _, d := range definers {
		call := l.firstArgCall
		if d.everyArg {
			call = l.everyArgCall
		}
		if call == "" {
			continue
		}
		calls := make([]string, len(d.calls))
		for i, c := range d.calls {
			calls[i] = regexp.QuoteMeta(c)
		}
		patterns = append(patterns, strings.ReplaceAll(call, "CALL", strings.Join(calls, "|")))
	}
	for i, p := range patterns {
		patterns[i] = strings.ReplaceAll(p, "NAME", regexp.QuoteMeta(name))
	}
	return patterns
}

// SupportsDefinitions reports whether the definitions in the files of path's
// language can be found
func SupportsDefinitions(path string) bool {
	_, ok := languages[LanguageOf(path)]
	return ok
}

// definitionMatcher tells the lines defining a name in one language
type definitionMatcher struct {
	def *regexp.Regexp
	not *regexp.Regexp // nil when every line def matches counts
}

// MatchString reports whether line defines the name
func (m *definitionMatcher) MatchString(line string) bool {
	return m.def.MatchString(line) && (m.not == nil || !m.not.MatchString(line))
}

// newDefinitionMatcher returns the matcher of a line defining name in lang,
// or nil for a language not known, whose lines never count
func newDefinitionMatcher(lang, name string) *definitionMatcher {
	l, ok := languages[lang]
	if !ok {
		return nil
	}
	m := &definitionMatcher{def: alternation(definitionPatterns(lang, name))}
	if len(l.notDefinitions) > 0 {
		m.not = alternation(l.notDefinitions)
	}
	return m
}

// alternation returns the regexp matching any of patterns
func alternation(patterns []string) *regexp.Regexp {
	return regexp.MustCompile("(?:" + strings.Join(patterns, ")|(?:") + ")")
}

// FindDefinitions returns the lines of the working tree that look like they
// define name: git grep finds the lines the word is on, tracked and untracked
// files alike, and the patterns of the language of each file keep the ones
// that define it.
func FindDefinitions(repoRoot, name string) ([]Location, error) {
	if name == "" {
		return nil, nil
	}
	cmd := exec.Command("git", "grep", "--untracked", "-I", "-z", "-n", "-w", "-F", "-e", name)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		// git grep exits with 1 when nothing matches
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	return parseDefinitions(output, name), nil
}

// parseDefinitions picks the lines defining name out of git grep -z -n
// output, whose lines read "path\x00line\x00text", each by the patterns
// of the language of its file
func parseDefinitions(output []byte, name string) []Location {
	var locations []Location
	matchers := map[string]*definitionMatcher{}
	for _, line := range bytes.Split(output, []byte("\n")) {
		fields := bytes.SplitN(line, []byte{0}, 3)
		if len(fields) != 3 {
			continue
		}
		lang := LanguageOf(string(fields[0]))
		m, ok := matchers[lang]
		if !ok {
			m = newDefinitionMatcher(lang, name)
			matchers[lang] = m
		}
		if m == nil || !m.MatchString(string(fields[2])) {
			continue
		}
		n, err := strconv.Atoi(string(fields[1]))
		if err != nil {
			continue
		}
		locations = append(locations, Location{Path: string(fields[0]), Line: n, Text: string(fields[2])})
	}
	return locations
}
