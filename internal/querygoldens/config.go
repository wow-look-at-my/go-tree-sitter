// Package querygoldens compares query results against files the reference
// tree-sitter command line tool wrote with `tree-sitter query`.
//
// A grammar package keeps its goldens under testdata/queries:
//
//	<stem>/compile.golden              "accepted\n", or the CLI's compile error
//	<stem>/matches/<input>.golden      stdout of `tree-sitter query`
//	<stem>/captures/<input>.golden     stdout of `tree-sitter query -c`
//
// <stem> is the .scm file name without its extension. <input> is one of
// samples/<file>, examples/<file> or corpus/<corpus file>. A golden holds one
// section per source file: the file's name on a line of its own, then the
// indented result lines. A corpus golden holds one section per case, named
// corpus/<corpus file>/<case index>.
package querygoldens

import "fmt"

// GoldensDir is where a grammar package keeps its query goldens.
const GoldensDir = "testdata/queries"

// Grammar says where the queries and inputs of one grammar live. Every path is
// relative to the grammar package directory.
type Grammar struct {
	// Name is the grammar name a corpus `:language(...)` header uses.
	Name string
	// Package is the grammar package directory, relative to the repository root.
	Package string
	// CLIGrammar is the grammar directory `tree-sitter query -p` loads.
	CLIGrammar string
	// Queries are the directories whose .scm files run.
	Queries []string
	// Samples is a directory of *.txt sources, or empty.
	Samples string
	// Examples is the upstream examples directory, or empty.
	Examples string
	// Corpus is the upstream test/corpus directory.
	Corpus string
}

// MaxExampleBytes bounds the examples that run.
const MaxExampleBytes = 16 << 10

// Grammars lists every bundled grammar.
var Grammars = []Grammar{
	{
		Name:       "go",
		Package:    "grammars/golang",
		CLIGrammar: "testdata/tree-sitter-go",
		Queries:    []string{"testdata/tree-sitter-go/queries", "testdata/invalid-queries"},
		Samples:    "testdata",
		Examples:   "testdata/tree-sitter-go/examples",
		Corpus:     "testdata/tree-sitter-go/test/corpus",
	},
	{
		Name:       "c",
		Package:    "grammars/clang",
		CLIGrammar: "testdata/tree-sitter-c",
		Queries:    []string{"testdata/tree-sitter-c/queries"},
		Samples:    "testdata",
		Examples:   "testdata/tree-sitter-c/examples",
		Corpus:     "testdata/tree-sitter-c/test/corpus",
	},
	{
		Name:       "cpp",
		Package:    "grammars/cpp",
		CLIGrammar: "testdata/tree-sitter-cpp",
		Queries:    []string{"testdata/tree-sitter-cpp/queries"},
		Examples:   "testdata/tree-sitter-cpp/examples",
		Corpus:     "testdata/tree-sitter-cpp/test/corpus",
	},
	{
		Name:       "rust",
		Package:    "grammars/rust",
		CLIGrammar: "testdata/tree-sitter-rust",
		Queries:    []string{"testdata/tree-sitter-rust/queries"},
		Samples:    "testdata",
		Examples:   "testdata/tree-sitter-rust/examples",
		Corpus:     "testdata/tree-sitter-rust/test/corpus",
	},
	{
		Name:       "bash",
		Package:    "grammars/bash",
		CLIGrammar: "testdata/tree-sitter-bash",
		Queries:    []string{"testdata/tree-sitter-bash/queries"},
		Examples:   "testdata/tree-sitter-bash/examples",
		Corpus:     "testdata/tree-sitter-bash/test/corpus",
	},
	{
		Name:       "javascript",
		Package:    "grammars/javascript",
		CLIGrammar: "testdata/tree-sitter-javascript",
		Queries:    []string{"testdata/tree-sitter-javascript/queries"},
		Examples:   "testdata/tree-sitter-javascript/examples",
		Corpus:     "testdata/tree-sitter-javascript/test/corpus",
	},
	{
		Name:       "typescript",
		Package:    "grammars/typescript",
		CLIGrammar: "testdata/tree-sitter-typescript/typescript",
		Queries:    []string{"testdata/tree-sitter-typescript/queries"},
		Examples:   "testdata/tree-sitter-typescript/examples",
		Corpus:     "testdata/tree-sitter-typescript/test/corpus",
	},
	{
		Name:       "tsx",
		Package:    "grammars/tsx",
		CLIGrammar: "../typescript/testdata/tree-sitter-typescript/tsx",
		Queries:    []string{"../typescript/testdata/tree-sitter-typescript/queries"},
		Examples:   "../typescript/testdata/tree-sitter-typescript/examples",
		Corpus:     "../typescript/testdata/tree-sitter-typescript/test/corpus",
	},
}

// Find returns the grammar with the given name.
func Find(name string) (Grammar, error) {
	for _, g := range Grammars {
		if g.Name == name {
			return g, nil
		}
	}
	return Grammar{}, fmt.Errorf("no grammar named %q", name)
}
