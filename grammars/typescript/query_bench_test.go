package typescript_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/typescript"
)

const typescriptHighlightsPath = "testdata/tree-sitter-typescript/queries/highlights.scm"

// BenchmarkCompileHighlights compiles the grammar's highlights query.
func BenchmarkCompileHighlights(b *testing.B) {
	source, err := os.ReadFile(typescriptHighlightsPath)
	require.NoError(b, err)
	language := typescript.Language()
	b.ReportAllocs()
	for b.Loop() {
		_, err := ts.NewQuery(language, string(source))
		require.NoError(b, err)
	}
}

// BenchmarkHighlightCaptures runs the highlights query over the grammar's
// large example file and drains its captures.
func BenchmarkHighlightCaptures(b *testing.B) {
	querySource, err := os.ReadFile(typescriptHighlightsPath)
	require.NoError(b, err)
	document, err := os.ReadFile("testdata/tree-sitter-typescript/examples/parser.ts")
	require.NoError(b, err)
	language := typescript.Language()
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
