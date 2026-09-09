// Package tsx is TypeScript with JSX. Upstream ships it as its own grammar
// beside typescript, built from the same scanner, because JSX and a type
// assertion both want the angle bracket and no file can have both.
//
// The generate step reads the submodule the typescript package checks out.
package tsx

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/typescript"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package tsx -scanner github.com/wow-look-at-my/go-tree-sitter/grammars/typescript -out parser.go ../typescript/testdata/tree-sitter-typescript/tsx/src/parser.c

// Scanner returns the scanner this grammar shares with typescript.
func Scanner() ts.ExternalScanner { return typescript.Scanner() }

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the TSX grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("tsx: parser.go is missing. Run: go generate ./grammars/tsx")
	}
	return generated
}
