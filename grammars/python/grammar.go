package python

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package python -out parser.go testdata/tree-sitter-python/src/parser.c

// Scanner returns the hand written external scanner.
func Scanner() ts.ExternalScanner { return scanner{} }

// load is set by the parser.go that the generate step writes.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the Python grammar, decoding its tables when a parse needs
// them. It panics when the table is missing, rather than hand back a language
// that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("python: parser.go is missing. Run: go generate ./grammars/python")
	}
	return generated
}
