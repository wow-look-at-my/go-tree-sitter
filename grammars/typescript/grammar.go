package typescript

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-fetch -repo tree-sitter/tree-sitter-typescript -rev 75b3874edb2dc714fb1fd77a32013d0f8699989f -dir testdata/tree-sitter-typescript
//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package typescript -out parser.go testdata/tree-sitter-typescript/typescript/src/parser.c

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the TypeScript grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
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
