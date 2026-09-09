package rust

import (
	_ "embed"
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -out tables.zst testdata/tree-sitter-rust/src/parser.c

//go:embed tables.zst
var tables []byte

// Scanner returns the hand written external scanner.
func Scanner() ts.ExternalScanner { return scanner{} }

var language = sync.OnceValue(func() *ts.Language {
	return ts.LoadGrammar("rust", tables, scanner{})
})

// Language returns the Rust grammar, decoding its tables when a parse needs them.
func Language() *ts.Language { return language() }
