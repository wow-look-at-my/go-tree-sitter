package rust

import ts "github.com/wow-look-at-my/go-tree-sitter"

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package rust -out parser.go testdata/tree-sitter-rust/src/parser.c

// generated is set by the parser.go that the generate step writes.
var generated *ts.Language

// Language returns the Rust grammar. It panics when the table is missing, rather
// than hand back a language that parses nothing.
func Language() *ts.Language {
	if generated == nil {
		panic("rust: parser.go is missing. Run: go generate ./grammars/rust")
	}
	return generated
}
