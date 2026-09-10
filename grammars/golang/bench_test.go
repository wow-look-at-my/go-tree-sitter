package golang_test

import (
	"testing"

	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

func BenchmarkDecodeTables(b *testing.B) { corpus.BenchmarkDecode(b, "tables.zst") }

// The runtime's own table codec: real Go, and a real Go file from the package
// this grammar is for.
func BenchmarkParse(b *testing.B) {
	corpus.BenchmarkParse(b, golang.Language(), "../../lexer.go")
}
