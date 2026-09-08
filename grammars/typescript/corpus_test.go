package typescript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/typescript"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs tree-sitter-typescript's own test corpus. Every case
// is a finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	dir := "testdata/tree-sitter-typescript/test/corpus"
	result := corpus.Run(t, typescript.Language(), dir, "typescript")
	assert.Zero(t, result.Failed, "corpus cases fail out of %d", result.Total())
}
