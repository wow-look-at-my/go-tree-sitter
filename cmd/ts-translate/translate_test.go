package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// The pinned grammars this test translates. Bash carries an external scanner
// and C carries none, so both shapes of grammar are exercised.
var pinnedGrammars = map[string]string{
	"bash":  "../../grammars/bash/testdata/tree-sitter-bash/src/parser.c",
	"clang": "../../grammars/clang/testdata/tree-sitter-c/src/parser.c",
}

// The generator has to survive a real parser.c, not a fixture shaped to suit
// it. This runs the whole path: tokenize, parse, build the tables and the lexer
// program, encode them, and decode them back.
func TestARealGrammarTranslatesAndDecodes(t *testing.T) {
	for pkg, path := range pinnedGrammars {
		t.Run(pkg, func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err, "the grammar submodule is not checked out")

			e := &emitter{file: parseFile(tokenize(src))}
			tables := e.buildTables()
			require.NoError(t, checkABI(tables.Language.ABIVersion))

			assert.NotZero(t, tables.Language.SymbolCount)
			assert.NotZero(t, tables.Language.StateCount)
			assert.NotEmpty(t, tables.Language.SymbolNames)
			assert.NotEmpty(t, tables.Language.ParseActions)
			assert.NotEmpty(t, tables.CharacterSets)

			require.NotNil(t, tables.Lex)
			assert.NotEmpty(t, tables.Lex.Code)
			assert.NotEmpty(t, tables.Lex.States)

			blob, err := ts.EncodeTables(tables)
			require.NoError(t, err)
			decoded, err := ts.DecodeTables(blob)
			require.NoError(t, err)
			assert.Equal(t, tables.Language.SymbolCount, decoded.Language.SymbolCount)
			assert.Equal(t, tables.Language.ABIVersion, decoded.Language.ABIVersion)
			assert.Equal(t, tables.Language.SymbolNames, decoded.Language.SymbolNames)
			assert.Equal(t, tables.Lex, decoded.Lex)
			assert.Equal(t, tables.KeywordLex, decoded.KeywordLex)
		})
	}
}

// Every state the lexer can enter needs an entry point, and every jump has to
// land inside the program. A dangling entry point or jump is a lexer that walks
// off its own instruction stream at parse time.
func TestTheLexProgramIsInternallyConsistent(t *testing.T) {
	src, err := os.ReadFile(pinnedGrammars["bash"])
	require.NoError(t, err)

	e := &emitter{file: parseFile(tokenize(src))}
	p := e.buildTables().Lex
	require.NotNil(t, p)

	for state, pc := range p.States {
		assert.Less(t, int(pc), len(p.Code), "state %d enters past the program", state)
	}
	for pc, ins := range p.Code {
		switch ins.Op {
		case ts.OpJumpIfFalse:
			assert.LessOrEqual(t, int(ins.Arg), len(p.Code),
				"the jump at %d lands past the program", pc)
		case ts.OpAdvanceMap:
			assert.Less(t, int(ins.Arg), len(p.Maps),
				"the advance map at %d names a map that is not there", pc)
		case ts.OpPushSet:
			assert.Less(t, int(ins.Arg), len(e.setIndex),
				"the set test at %d names a set that is not there", pc)
		}
	}
}
