package treesitter_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// matchPredicate finds the pattern of a `#match?` family predicate.
var matchPredicate = regexp.MustCompile(`#(?:not-|any-|any-not-)?match\?\s+@[\w.-]+\s+"((?:[^"\\]|\\.)*)"`)

// bundledQueryRegexes collects every regex the bundled .scm files use.
func bundledQueryRegexes(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("grammars", "*", "testdata", "*", "queries", "*.scm"))
	require.NoError(t, err)
	tsFiles, err := filepath.Glob(filepath.Join("grammars", "typescript", "testdata", "tree-sitter-typescript", "queries", "*.scm"))
	require.NoError(t, err)
	files = append(files, tsFiles...)
	require.NotEmpty(t, files, "no bundled .scm files: are the submodules checked out?")
	sort.Strings(files)

	result := map[string][]string{}
	for _, file := range files {
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		for _, m := range matchPredicate.FindAllStringSubmatch(string(source), -1) {
			result[file] = append(result[file], unescapeQueryString(m[1]))
		}
	}
	return result
}

// unescapeQueryString applies the escapes ts_query__parse_string_literal
// applies.
func unescapeQueryString(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			out = append(out, s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case '0':
			out = append(out, 0)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// Every regex a bundled query uses compiles under Go's RE2 syntax, and uses
// no construct whose meaning differs from the Rust regex crate's.
func TestBundledQueryRegexesCompileUnderRE2(t *testing.T) {
	regexes := bundledQueryRegexes(t)
	require.NotEmpty(t, regexes)
	total := 0
	for file, patterns := range regexes {
		for _, pattern := range patterns {
			total++
			_, err := regexp.Compile(pattern)
			require.NoError(t, err, "%s: %q", file, pattern)
			for _, d := range regexSyntaxDivergences {
				require.False(t, d.detect.MatchString(pattern),
					"%s: %q uses %s, which Go's regexp reads differently from the Rust regex crate", file, pattern, d.name)
			}
		}
	}
	t.Logf("%d regexes across %d query files compile under RE2", total, len(regexes))
}

// regexSyntaxDivergence is a construct the Rust regex crate accepts and Go's
// regexp either rejects or reads differently.
type regexSyntaxDivergence struct {
	name    string
	example string
	detect  *regexp.Regexp
	// goRejects is true when Go's regexp refuses the example outright.
	goRejects bool
}

// The known divergences between the Rust regex crate and Go's RE2 syntax.
// A bundled query that used one would silently change meaning, so the test
// above refuses them.
var regexSyntaxDivergences = []regexSyntaxDivergence{
	{name: `\u{...} escapes`, example: `\u{41}`, detect: regexp.MustCompile(`\\[uU]`), goRejects: true},
	{name: "character class set operations", example: `[a-z&&[^aeiou]]`, detect: regexp.MustCompile(`&&|--|~~`), goRejects: false},
	{name: "nested character classes", example: `[a[bc]]`, detect: regexp.MustCompile(`\[[^\]]*\[[^:]`), goRejects: false},
	{name: "the x flag for insignificant whitespace", example: `(?x) a b`, detect: regexp.MustCompile(`\(\?[a-zA-Z-]*x`), goRejects: true},
	{name: "the R flag for CRLF line anchors", example: `(?R)^a$`, detect: regexp.MustCompile(`\(\?[a-zA-Z-]*R`), goRejects: true},
	{name: `\b{start} word boundary variants`, example: `\b{start}a`, detect: regexp.MustCompile(`\\b\{`), goRejects: true},
}

// Each divergence behaves as recorded: Go rejects the example or reads it as
// something other than the Rust crate does.
func TestRegexSyntaxDivergencesAreRecorded(t *testing.T) {
	for _, d := range regexSyntaxDivergences {
		t.Run(d.name, func(t *testing.T) {
			require.True(t, d.detect.MatchString(d.example), "the detector misses its own example")
			_, err := regexp.Compile(d.example)
			if d.goRejects {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	// Go reads `[a-z&&[^aeiou]]` as a class holding `&` and `[`, then a literal `]`.
	re := regexp.MustCompile(`^[a-z&&[^aeiou]]$`)
	require.False(t, re.MatchString("b"))
	require.True(t, re.MatchString("&]"))
}
