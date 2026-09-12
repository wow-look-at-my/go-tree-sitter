package javascript

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-fetch -repo tree-sitter/tree-sitter-javascript -rev 58404d8cf191d69f2674a8fd507bd5776f46cb11 -dir testdata/tree-sitter-javascript
//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package javascript -out parser.go testdata/tree-sitter-javascript/src/parser.c

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the JavaScript grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("javascript: parser.go is missing. Run: go generate ./grammars/javascript")
	}
	return generated
}
