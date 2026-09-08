package clang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/clang"
	"github.com/wow-look-at-my/go-tree-sitter/internal/goldens"
)

func TestGoldenTrees(t *testing.T) {
	goldens.Run(t, clang.Language(), "testdata")
}

func TestFieldsAreReachable(t *testing.T) {
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(clang.Language()))
	tree := parser.ParseString(nil, []byte("int add(int a, int b) { return a + b; }\n"))
	require.NotNil(t, tree)

	root := tree.RootNode()
	function := root.NamedChild(0)
	assert.Equal(t, "function_definition", function.Type())

	returnType := function.ChildByFieldName("type")
	require.False(t, returnType.IsNull())
	assert.Equal(t, "primitive_type", returnType.Type())

	declarator := function.ChildByFieldName("declarator")
	require.False(t, declarator.IsNull())
	assert.Equal(t, "function_declarator", declarator.Type())

	body := function.ChildByFieldName("body")
	require.False(t, body.IsNull())
	assert.Equal(t, "compound_statement", body.Type())

	assert.Equal(t, "type", function.FieldNameForNamedChild(0))
	assert.Equal(t, "", function.ChildByFieldName("nonexistent").Type())

	statement := body.NamedChild(0)
	assert.Equal(t, "return_statement", statement.Type())
	sum := statement.NamedChild(0)
	assert.Equal(t, "binary_expression", sum.Type())
	assert.Equal(t, "left", sum.FieldNameForNamedChild(0))
	assert.Equal(t, "right", sum.FieldNameForNamedChild(1))

	source := []byte("int add(int a, int b) { return a + b; }\n")
	left := sum.ChildByFieldName("left")
	assert.Equal(t, "a", string(source[left.StartByte():left.EndByte()]))
	right := sum.ChildByFieldName("right")
	assert.Equal(t, "b", string(source[right.StartByte():right.EndByte()]))
	assert.Equal(t, "left", sum.FieldNameForChild(0))
}

func TestRecoveryProducesAnErrorNode(t *testing.T) {
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(clang.Language()))
	tree := parser.ParseString(nil, []byte("int main(void) { return @@@; }\n"))
	require.NotNil(t, tree)
	assert.True(t, tree.RootNode().HasError())
	assert.Contains(t, tree.String(), "ERROR")
}

func TestWalkVisitsEveryNamedNode(t *testing.T) {
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(clang.Language()))
	tree := parser.ParseString(nil, []byte("int x = 1;\nint y = 2;\n"))
	require.NotNil(t, tree)

	cursor := tree.Walk()
	seen := 0
	var visit func()
	visit = func() {
		seen++
		if cursor.GotoFirstChild() {
			for {
				visit()
				if !cursor.GotoNextSibling() {
					break
				}
			}
			cursor.GotoParent()
		}
	}
	visit()
	assert.Greater(t, seen, 8)
	assert.Equal(t, "translation_unit", cursor.Node().Type())
}
