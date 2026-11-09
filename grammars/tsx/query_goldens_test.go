package tsx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/tsx"
	"github.com/wow-look-at-my/go-tree-sitter/internal/querygoldens"
)

// TestQueryGoldens compares every bundled query over every input with the
// output of the pinned tree-sitter CLI, byte for byte.
func TestQueryGoldens(t *testing.T) {
	stats := querygoldens.Run(t, tsx.Language(), "tsx")
	assert.Zero(t, stats.Failed, "query goldens fail out of %d", stats.Compared)
}

// TestQueryCompileParity checks that Go compiles exactly the queries the CLI
// compiled, with the same error otherwise.
func TestQueryCompileParity(t *testing.T) {
	querygoldens.RunCompile(t, tsx.Language(), "tsx")
}
