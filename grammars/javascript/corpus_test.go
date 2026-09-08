package javascript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/javascript"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs tree-sitter-javascript's own test corpus. Every case
// is a finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	result := corpus.Run(t, javascript.Language(), "testdata/tree-sitter-javascript/test/corpus", "javascript")
	assert.Zero(t, result.Failed, "corpus cases fail out of %d", result.Total())
}
