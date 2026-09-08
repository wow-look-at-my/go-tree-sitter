package cpp_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/cpp"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs tree-sitter-cpp's own test corpus. Every case is a
// finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	result := corpus.Run(t, cpp.Language(), "testdata/tree-sitter-cpp/test/corpus", "cpp")
	assert.Zero(t, result.Failed, "corpus cases fail out of %d", result.Total())
}
