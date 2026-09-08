package golang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
	"github.com/wow-look-at-my/go-tree-sitter/internal/goldens"
)

func TestGoldenTrees(t *testing.T) {
	goldens.Run(t, golang.Language(), "testdata")
}

func TestLanguageDetails(t *testing.T) {
	language := golang.Language()
	assert.Equal(t, "go", language.LanguageName())
	assert.Equal(t, uint32(15), language.ABIVersion)

	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(language))
	tree := parser.ParseString(nil, []byte("package main\n"))
	require.NotNil(t, tree)

	root := tree.RootNode()
	assert.Equal(t, "source_file", root.Type())
	clause := root.NamedChild(0)
	assert.Equal(t, "package_clause", clause.Type())
	assert.False(t, root.HasError())
}
