package golang

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package golang -out parser.go testdata/tree-sitter-go/src/parser.c

// load is set by the parser.go that the generate step writes.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the Go grammar, decoding its tables on the first call. It
// panics when the table is missing, rather than hand back a language that
// parses nothing.
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
