// Package tsx is TypeScript with JSX. Upstream ships it as its own grammar
// beside typescript, built from the same scanner, because JSX and a type
// assertion both want the angle bracket and no file can have both.
//
// The generate step reads the submodule the typescript package checks out.
package tsx

import (
	_ "embed"
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/typescript"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -out tables.zst ../typescript/testdata/tree-sitter-typescript/tsx/src/parser.c

//go:embed tables.zst
var tables []byte

// Scanner returns the scanner this grammar shares with typescript.
func Scanner() ts.ExternalScanner { return typescript.Scanner() }

var language = sync.OnceValue(func() *ts.Language {
	return ts.LoadGrammar("tsx", tables, typescript.Scanner())
})

// Language returns the TSX grammar, decoding its tables when a parse needs them.
func Language() *ts.Language { return language() }
