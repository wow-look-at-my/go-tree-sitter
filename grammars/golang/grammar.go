package golang

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-fetch -repo tree-sitter/tree-sitter-go -rev 2346a3ab1bb3857b48b29d779a1ef9799a248cd7 -dir testdata/tree-sitter-go
//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package golang -out parser.go testdata/tree-sitter-go/src/parser.c

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the Go grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("golang: parser.go is missing. Run: go generate ./grammars/golang")
	}
	return generated
}
