package clang_test

import (
	"testing"

	"github.com/wow-look-at-my/go-tree-sitter/grammars/clang"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs tree-sitter-c's own test corpus. Every case is a
// finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	result := corpus.Run(t, clang.Language(), "testdata/corpus")
	if result.Failed > 0 {
		t.Errorf("%d of %d upstream corpus cases fail", result.Failed, result.Total())
	}
}
