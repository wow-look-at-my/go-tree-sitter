package clang_test

import (
	"testing"

	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

func BenchmarkDecodeTables(b *testing.B) { corpus.BenchmarkDecode(b, "tables.zst") }
