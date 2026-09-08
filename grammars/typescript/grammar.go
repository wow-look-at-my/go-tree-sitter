package typescript

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package typescript -out parser.go testdata/tree-sitter-typescript/typescript/src/parser.c

// load is set by the parser.go that the generate step writes.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the TypeScript grammar, decoding its tables when a parse
// needs them. It panics when the table is missing, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("typescript: parser.go is missing. Run: go generate ./grammars/typescript")
	}
	return generated
}
