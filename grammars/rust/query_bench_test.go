package rust_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/rust"
)

const rustHighlightsPath = "testdata/tree-sitter-rust/queries/highlights.scm"

// BenchmarkCompileHighlights compiles the grammar's highlights query.
func BenchmarkCompileHighlights(b *testing.B) {
	source, err := os.ReadFile(rustHighlightsPath)
	require.NoError(b, err)
	language := rust.Language()
	b.ReportAllocs()
	for b.Loop() {
		_, err := ts.NewQuery(language, string(source))
		require.NoError(b, err)
	}
}

// BenchmarkHighlightCaptures runs the highlights query over the large Rust
// file and drains its captures.
func BenchmarkHighlightCaptures(b *testing.B) {
	querySource, err := os.ReadFile(rustHighlightsPath)
	require.NoError(b, err)
	document, err := os.ReadFile("testdata/large_goal_support.rs.txt")
	require.NoError(b, err)
	language := rust.Language()
	query, err := ts.NewQuery(language, string(querySource))
	require.NoError(b, err)
	parser := ts.NewParser()
	require.True(b, parser.SetLanguage(language))
	tree := parser.ParseString(nil, document)
	require.NotNil(b, tree)

	cursor := ts.NewQueryCursor()
	b.SetBytes(int64(len(document)))
	b.ReportAllocs()
	for b.Loop() {
		count := 0
		for range cursor.Captures(query, tree.RootNode(), document) {
			count++
		}
		require.NotZero(b, count)
	}
}
