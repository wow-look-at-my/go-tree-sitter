package grammarsrc

import (
	"path/filepath"
	"testing"
)

// A caller names its parser.c, and that path already says which directory the
// submodule occupies. Deriving it is what lets one directive fetch and
// translate, so a caller cannot write the fetch and the translate against two
// different directories.
func TestDirForReadsTheSubmoduleOutOfTheParserPath(test *testing.T) {
	for _, row := range []struct {
		name   string
		parser string
		repo   string
		want   string
	}{
		{
			name:   "the parser sits directly under the submodule",
			parser: "testdata/tree-sitter-bash/src/parser.c",
			repo:   "tree-sitter/tree-sitter-bash",
			want:   "testdata/tree-sitter-bash",
		},
		{
			name:   "one repository holding several grammars",
			parser: "testdata/tree-sitter-typescript/typescript/src/parser.c",
			repo:   "tree-sitter/tree-sitter-typescript",
			want:   "testdata/tree-sitter-typescript",
		},
		{
			name:   "a sibling package reading the same submodule",
			parser: "../typescript/testdata/tree-sitter-typescript/tsx/src/parser.c",
			repo:   "tree-sitter/tree-sitter-typescript",
			want:   "../typescript/testdata/tree-sitter-typescript",
		},
		{
			name:   "a name that is a prefix of another grammar's",
			parser: "testdata/tree-sitter-c/src/parser.c",
			repo:   "tree-sitter/tree-sitter-c",
			want:   "testdata/tree-sitter-c",
		},
	} {
		test.Run(row.name, func(test *testing.T) {
			got, err := DirFor(row.parser, row.repo)
			if err != nil {
				test.Fatalf("DirFor(%q, %q): %v", row.parser, row.repo, err)
			}
			if got != filepath.FromSlash(row.want) {
				test.Errorf("DirFor(%q, %q) = %q, want %q", row.parser, row.repo, got, row.want)
			}
		})
	}
}

// A wrong answer here fetches into the wrong directory and leaves the real one
// empty, so the failure has to be loud rather than a guess.
func TestDirForRefusesWhatItCannotDerive(test *testing.T) {
	for _, row := range []struct {
		name   string
		parser string
		repo   string
	}{
		{"the repository is not owner/name", "testdata/tree-sitter-bash/src/parser.c", "tree-sitter-bash"},
		{"the repository has no name", "testdata/tree-sitter-bash/src/parser.c", "tree-sitter/"},
		{"no directory above the parser carries that name", "testdata/grammar/src/parser.c", "tree-sitter/tree-sitter-bash"},
	} {
		test.Run(row.name, func(test *testing.T) {
			if got, err := DirFor(row.parser, row.repo); err == nil {
				test.Errorf("DirFor(%q, %q) = %q, want an error", row.parser, row.repo, got)
			}
		})
	}
}
