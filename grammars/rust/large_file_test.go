package rust_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/rust"
)

// This real file a single time crashed subtreeCompress on an inline earliest child.
func TestParsesALargeRealRustFile(t *testing.T) {
	src, err := os.ReadFile("testdata/large_goal_support.rs.txt")
	require.NoError(t, err)
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(rust.Language()))
	tree := parser.ParseString(nil, src)
	require.NotNil(t, tree)
	root := tree.RootNode()
	require.False(t, root.HasError(), "the file is valid Rust")
}
