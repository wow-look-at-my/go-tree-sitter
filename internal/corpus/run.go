package corpus

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Result counts the outcome of a corpus run.
type Result struct {
	// Passed is the number of cases whose tree matched.
	Passed int
	// Failed is the number of cases whose tree did not match.
	Failed int
	// Skipped is the number of cases upstream itself does not run.
	Skipped int
	// Elsewhere is the number of cases a header hands to another grammar.
	Elsewhere int
	// Failures names each failing case, in corpus order.
	Failures []string
	// Details compares the expected and actual tree of the earliest failures.
	Details []Detail
}

// Detail is the comparison a failing case produced.
type Detail struct {
	// Name is the case, prefixed by its corpus file.
	Name string
	// Want is the expected tree.
	Want string
	// Got is the tree the parse produced.
	Got string
}

// detailLimit bounds the comparisons a report carries.
const detailLimit = 3

// Total is the number of cases that actually ran.
func (r Result) Total() int { return r.Passed + r.Failed }

// Run parses every case in a corpus directory and compares the tree it produces
// against the expected tree. It reports the counts and never repairs anything.
//
// grammar is the name this language answers to in a `:language(...)` header. A
// repository that ships several grammars shares a corpus between them, so a
// case naming another grammar belongs to that grammar's own run. An empty name
// runs every case, which suits a corpus carrying no such header.
func Run(t *testing.T, language *ts.Language, dir, grammar string) Result {
	t.Helper()
	cases, err := ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("no corpus cases under %s", dir)
	}

	parser := ts.NewParser()
	if !parser.SetLanguage(language) {
		t.Fatal("the parser rejected the language")
	}

	var result Result
	byFile := map[string]*Result{}
	for _, c := range cases {
		if c.Expect == ExpectSkip || !c.Platform {
			result.Skipped++
			continue
		}
		if grammar != "" && c.Language != "" && c.Language != grammar {
			result.Elsewhere++
			continue
		}
		file := byFile[c.File]
		if file == nil {
			file = &Result{}
			byFile[c.File] = file
		}
		got, ok := runCase(parser, c)
		if ok {
			result.Passed++
			file.Passed++
			continue
		}
		result.Failed++
		file.Failed++
		name := c.File + ": " + c.Name
		result.Failures = append(result.Failures, name)
		if len(result.Details) < detailLimit {
			result.Details = append(result.Details, Detail{Name: name, Want: c.Output, Got: got})
		}
	}

	report(t, result, byFile)
	return result
}

func runCase(parser *ts.Parser, c Case) (string, bool) {
	tree := parser.ParseString(nil, c.Input)
	if tree == nil {
		return "", false
	}
	root := tree.RootNode()
	if c.Expect == ExpectError {
		return "", root.HasError()
	}
	actual := root.String()
	if !c.HasFields {
		actual = StripFields(actual)
	}
	return actual, actual == c.Output
}

func report(t *testing.T, result Result, byFile map[string]*Result) {
	t.Helper()
	files := make([]string, 0, len(byFile))
	for name := range byFile {
		files = append(files, name)
	}
	sort.Strings(files)

	var b strings.Builder
	fmt.Fprintf(&b, "corpus: %d/%d cases pass, %d fail, %d skipped, %d for another grammar\n",
		result.Passed, result.Total(), result.Failed, result.Skipped, result.Elsewhere)
	for _, name := range files {
		r := byFile[name]
		fmt.Fprintf(&b, "  %-20s %d/%d\n", name, r.Passed, r.Passed+r.Failed)
	}
	if len(result.Failures) > 0 {
		b.WriteString("failing cases:\n")
		for _, name := range result.Failures {
			fmt.Fprintf(&b, "  %s\n", name)
		}
	}
	for _, d := range result.Details {
		fmt.Fprintf(&b, "\n%s\n  want: %s\n   got: %s\n", d.Name, d.Want, d.Got)
	}
	// stderr, so a passing package still shows the counts.
	fmt.Fprint(os.Stderr, "\n"+b.String())
}
