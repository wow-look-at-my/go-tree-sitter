package bash

import (
	"sync"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-fetch -repo tree-sitter/tree-sitter-bash -rev a06c2e4415e9bc0346c6b86d401879ffb44058f7 -dir testdata/tree-sitter-bash
//go:generate go run github.com/wow-look-at-my/go-tree-sitter/cmd/ts-translate -package bash -out parser.go testdata/tree-sitter-bash/src/parser.c

// Scanner returns the hand written external scanner.
func Scanner() ts.ExternalScanner { return scanner{} }

// load is set by the parser.go that the generate step writes. The tables are
// data and are generated at build time, so this half compiles without them.
var (
	load      func() *ts.Language
	loadOnce  sync.Once
	generated *ts.Language
)

// Language returns the Bash grammar, decoding its tables when a parse needs
// them. It panics when the generate step has not run, rather than hand back a
// language that parses nothing.
func Language() *ts.Language {
	loadOnce.Do(func() {
		if load != nil {
			generated = load()
		}
	})
	if generated == nil {
		panic("bash: parser.go is missing. Run: go generate ./grammars/bash")
	}
	return generated
}
