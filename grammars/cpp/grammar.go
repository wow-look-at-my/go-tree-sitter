package cpp

import ts "github.com/wow-look-at-my/go-tree-sitter"

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package cpp -out parser.go testdata/tree-sitter-cpp/src/parser.c

// generated is set by parser.go, which the generate step writes and the
// repository does not carry.
var generated *ts.Language

// Language returns the C++ grammar. It panics when the table is missing, rather
// than hand back a language that parses nothing.
func Language() *ts.Language {
	if generated == nil {
		panic("cpp: parser.go is missing. Run: go generate ./grammars/cpp")
	}
	return generated
}
