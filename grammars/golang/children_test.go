package golang_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

// sameNode reports nodes of one tree at one place with one type.
func sameNode(a, b ts.Node) bool {
	return a.Equal(b) && a.Type() == b.Type() && a.IsNamed() == b.IsNamed()
}

// Children, NamedChildren and a cursor must each answer as the index lookups
// do, for every node of real Go sources. The grammar hides nodes, so the
// descent through a hidden child is covered.
func TestChildrenAnswerAsTheIndexLookups(t *testing.T) {
	paths, err := filepath.Glob("testdata/*.go.txt")
	require.NoError(t, err)
	more, err := filepath.Glob("*.go")
	require.NoError(t, err)
	paths = append(paths, more...)
	require.NotEmpty(t, paths)

	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(golang.Language()))
	for _, path := range paths {
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		if len(src) > 256<<10 {
			continue
		}
		tree := parser.ParseString(nil, src)
		require.NotNil(t, tree, path)

		var walk func(n ts.Node, cursor *ts.TreeCursor)
		walk = func(n ts.Node, cursor *ts.TreeCursor) {
			all, named := n.Children(), n.NamedChildren()
			require.Len(t, all, int(n.ChildCount()), path)
			require.Len(t, named, int(n.NamedChildCount()), path)
			for i, child := range named {
				require.True(t, sameNode(child, n.NamedChild(uint32(i))), "%s: named child %d of %s", path, i, n.Type())
			}
			if !cursor.GotoFirstChild() {
				require.Empty(t, all, path)
				return
			}
			for i, child := range all {
				require.True(t, sameNode(child, n.Child(uint32(i))), "%s: child %d of %s", path, i, n.Type())
				require.True(t, sameNode(child, cursor.Node()), "%s: cursor child %d of %s", path, i, n.Type())
				walk(child, cursor)
				require.Equal(t, i+1 < len(all), cursor.GotoNextSibling(), path)
			}
			for i := len(all) - 2; i >= 0; i-- {
				require.True(t, cursor.GotoPreviousSibling(), path)
				require.True(t, sameNode(all[i], cursor.Node()), path)
			}
			require.False(t, cursor.GotoPreviousSibling(), path)
			require.True(t, cursor.GotoParent(), path)
		}
		root := tree.RootNode()
		walk(root, root.Walk())
	}
}
