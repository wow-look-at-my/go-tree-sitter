package treesitter_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

// A first query over a Go file: captures come back in document order with
// the names the pattern gave them.
func TestQueryMatchesAGoFunctionName(t *testing.T) {
	language := golang.Language()
	query, err := ts.NewQuery(language, `(function_declaration name: (identifier) @name) @fn`)
	require.NoError(t, err)

	source := []byte("package main\n\nfunc a() {}\nfunc b() {}\n")
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(language))
	tree := parser.ParseString(nil, source)

	cursor := ts.NewQueryCursor()
	var got []string
	for m := range cursor.Matches(query, tree.RootNode(), source) {
		for _, c := range m.Captures {
			got = append(got, query.CaptureNameForID(c.Index)+"="+string(source[c.Node.StartByte():c.Node.EndByte()]))
		}
	}
	require.Equal(t, []string{"fn=func a() {}", "name=a", "fn=func b() {}", "name=b"}, got)

	var names []string
	for m, index := range cursor.Captures(query, tree.RootNode(), source) {
		c := m.Captures[index]
		names = append(names, query.CaptureNameForID(c.Index))
	}
	require.Equal(t, []string{"fn", "name", "fn", "name"}, names)
}

// An unknown node type is reported with the offset of its name.
func TestQueryReportsAnUnknownNodeType(t *testing.T) {
	_, err := ts.NewQuery(golang.Language(), "(function_declaration (nonsense))")
	var qerr *ts.QueryError
	require.ErrorAs(t, err, &qerr)
	require.Equal(t, ts.QueryErrorNodeType, qerr.Kind)
	require.Equal(t, 23, qerr.Offset)
	require.Equal(t, `"nonsense"`, qerr.Message)
}
