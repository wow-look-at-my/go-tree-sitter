package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// language builds the emitter state a TSLanguage initializer produces, with a
// named field set.
func language(field string, value int64) *emitter {
	return &emitter{file: &cFile{lang: map[string]langValue{
		field: {isNum: true, num: value},
	}}}
}

// The newest ABI renamed this field. An older grammar spells it "version", and
// the reader that knew only the new name left such a grammar unversioned.
func TestBothSpellingsOfTheABIFieldAreRead(t *testing.T) {
	assert.Equal(t, uint32(ts.MaxABIVersion), language("abi_version", ts.MaxABIVersion).abiVersion())
	assert.Equal(t, uint32(ts.MaxABIVersion-1), language("version", ts.MaxABIVersion-1).abiVersion())
	assert.Zero(t, (&emitter{file: &cFile{lang: map[string]langValue{}}}).abiVersion())
}

// A grammar the runtime rejects must fail the build. The package it would
// otherwise write compiles, links and decodes, then parses nothing.
func TestAnABIThisRuntimeCannotParseWithIsRefused(t *testing.T) {
	for v := ts.MinABIVersion; v <= ts.MaxABIVersion; v++ {
		assert.NoError(t, checkABI(uint32(v)), "ABI %d", v)
	}

	err := checkABI(0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no ABI version")

	for _, v := range []uint32{ts.MinABIVersion - 1, ts.MaxABIVersion + 1} {
		err := checkABI(v)
		require.Error(t, err, "ABI %d", v)
		assert.Contains(t, err.Error(), "this runtime parses ABI")
	}
}

// The runtime and the translator must agree on the range, or a table lands that
// the runtime then refuses.
func TestTheTranslatorAcceptsExactlyWhatTheRuntimeDoes(t *testing.T) {
	parser := ts.NewParser()
	for v := uint32(0); v <= ts.MaxABIVersion+2; v++ {
		accepted := parser.SetLanguage(&ts.Language{ABIVersion: v, LexFn: lexNothing})
		assert.Equal(t, accepted, checkABI(v) == nil, "ABI %d", v)
	}
}

func lexNothing(*ts.Lexer, ts.StateID) bool { return false }
