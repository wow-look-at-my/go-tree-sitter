package bash_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/bash"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs tree-sitter-bash's own test corpus. Every case is a
// finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	result := corpus.Run(t, bash.Language(), "testdata/tree-sitter-bash/test/corpus", "bash")
	assert.Zero(t, result.Failed, "corpus cases fail out of %d", result.Total())
}
