package clang_test

import (
	"testing"

	"github.com/wow-look-at-my/go-tree-sitter/grammars/clang"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

func BenchmarkDecodeTables(b *testing.B) { corpus.BenchmarkDecode(b, "tables.zst") }

// The grammar's own generated parser.c: a large, real C file that is already in
// the tree.
func BenchmarkParse(b *testing.B) {
	corpus.BenchmarkParse(b, clang.Language(), "testdata/tree-sitter-c/src/parser.c")
}
