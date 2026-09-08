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
	// Failures names each failing case, in corpus order.
	Failures []string
}

// Total is the number of cases that actually ran.
func (r Result) Total() int { return r.Passed + r.Failed }

// Run parses every case in a corpus directory and compares the tree it produces
// against the expected tree. It reports the counts and never repairs anything.
func Run(t *testing.T, language *ts.Language, dir string) Result {
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
		file := byFile[c.File]
		if file == nil {
			file = &Result{}
			byFile[c.File] = file
		}
		if runCase(parser, c) {
			result.Passed++
			file.Passed++
			continue
		}
		result.Failed++
		file.Failed++
		result.Failures = append(result.Failures, c.File+": "+c.Name)
	}

	report(t, result, byFile)
	return result
}

func runCase(parser *ts.Parser, c Case) bool {
	tree := parser.ParseString(nil, c.Input)
	if tree == nil {
		return false
	}
	root := tree.RootNode()
	if c.Expect == ExpectError {
		return root.HasError()
	}
	actual := root.String()
	if !c.HasFields {
		actual = StripFields(actual)
	}
	return actual == c.Output
}

func report(t *testing.T, result Result, byFile map[string]*Result) {
	t.Helper()
	files := make([]string, 0, len(byFile))
	for name := range byFile {
		files = append(files, name)
	}
	sort.Strings(files)

	var b strings.Builder
	fmt.Fprintf(&b, "corpus: %d/%d cases pass, %d fail, %d skipped\n",
		result.Passed, result.Total(), result.Failed, result.Skipped)
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
	// stderr, so a passing package still shows the counts.
	fmt.Fprint(os.Stderr, "\n"+b.String())
}
