package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// The pinned grammars this test translates. Bash carries an external scanner
// and C carries none, so both halves of the emitted package are exercised.
var pinnedGrammars = map[string]string{
	"bash":  "../../grammars/bash/testdata/tree-sitter-bash/src/parser.c",
	"clang": "../../grammars/clang/testdata/tree-sitter-c/src/parser.c",
}

// The generator has to survive a real parser.c, not a fixture shaped to suit
// it. This runs the whole path: tokenize, parse, build the tables, encode them,
// decode them back, and emit the package.
func TestARealGrammarTranslatesAndDecodes(t *testing.T) {
	for pkg, path := range pinnedGrammars {
		t.Run(pkg, func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err, "the grammar submodule is not checked out")

			e := &emitter{file: parseFile(tokenize(src)), sb: &strings.Builder{}, pkg: pkg}
			tables := e.buildTables()
			require.NoError(t, checkABI(tables.Language.ABIVersion))

			assert.NotZero(t, tables.Language.SymbolCount)
			assert.NotZero(t, tables.Language.StateCount)
			assert.NotEmpty(t, tables.Language.SymbolNames)
			assert.NotEmpty(t, tables.Language.ParseActions)
			assert.NotEmpty(t, tables.CharacterSets)

			blob, err := ts.EncodeTables(tables)
			require.NoError(t, err)
			decoded, err := ts.DecodeTables(blob)
			require.NoError(t, err)
			assert.Equal(t, tables.Language.SymbolCount, decoded.Language.SymbolCount)
			assert.Equal(t, tables.Language.ABIVersion, decoded.Language.ABIVersion)
			assert.Equal(t, tables.Language.SymbolNames, decoded.Language.SymbolNames)

			e.emitPackage()
			out := e.sb.String()
			assert.Contains(t, out, "package "+pkg)
			assert.Contains(t, out, "func tsLex(")
			assert.Contains(t, out, "func loadTables()")
		})
	}
}

// A grammar with an external scanner has to reach it. A package that omits the
// wiring parses without the tokens the scanner alone produces.
func TestAGrammarWithAScannerWiresOneUp(t *testing.T) {
	src, err := os.ReadFile(pinnedGrammars["bash"])
	require.NoError(t, err)

	e := &emitter{
		file: parseFile(tokenize(src)), sb: &strings.Builder{}, pkg: "bash",
		scannerPkg: "example.com/grammar",
	}
	require.True(t, e.file.hasScanner)
	e.buildTables()
	e.emitPackage()
	assert.Contains(t, e.sb.String(), "language.Scanner = grammar.Scanner()")
}

// The standalone form supplies the accessor a package with no hand written half
// would otherwise lack.
func TestTheStandaloneFormEmitsItsOwnAccessor(t *testing.T) {
	src, err := os.ReadFile(pinnedGrammars["clang"])
	require.NoError(t, err)

	e := &emitter{
		file: parseFile(tokenize(src)), sb: &strings.Builder{},
		pkg: "clang", standalone: true,
	}
	e.buildTables()
	e.emitPackage()
	out := e.sb.String()
	assert.Contains(t, out, "func Language() *ts.Language")
	assert.NotContains(t, out, "func init() { load = loadTables }")
}
