package golang

import (
	_ "embed"
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -out tables.zst testdata/tree-sitter-go/src/parser.c

//go:embed tables.zst
var tables []byte

var language = sync.OnceValue(func() *ts.Language {
	return ts.LoadGrammar("golang", tables, nil)
})

// Language returns the Go grammar, decoding its tables when a parse needs them.
func Language() *ts.Language { return language() }
