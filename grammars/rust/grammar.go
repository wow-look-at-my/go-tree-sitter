package rust

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-fetch -repo tree-sitter/tree-sitter-rust -rev 77a3747266f4d621d0757825e6b11edcbf991ca5 -dir testdata/tree-sitter-rust
//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package rust -out parser.go testdata/tree-sitter-rust/src/parser.c

// Scanner returns the hand written external scanner.
func Scanner() ts.ExternalScanner { return scanner{} }

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the Rust grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("rust: parser.go is missing. Run: go generate ./grammars/rust")
	}
	return generated
}
