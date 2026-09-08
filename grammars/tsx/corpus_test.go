package tsx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/tsx"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// TestUpstreamCorpus runs the tsx cases of tree-sitter-typescript's own test
// corpus. Every case is a finding: none is deleted and none is skipped here.
func TestUpstreamCorpus(t *testing.T) {
	dir := "../typescript/testdata/tree-sitter-typescript/test/corpus"
	result := corpus.Run(t, tsx.Language(), dir, "tsx")
	assert.Zero(t, result.Failed, "corpus cases fail out of %d", result.Total())
}
