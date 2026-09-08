package treesitter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	toySymEnd        Symbol = 0
	toySymWord       Symbol = 1
	toySymSourceFile Symbol = 2
	toySymRepeat     Symbol = 3
)

func toyLex(lexer *Lexer, state StateID) bool {
	result := false
	skip := false
	eof := false
	var lookahead int32
	_, _ = eof, lookahead
	goto start
nextState:
	lexer.Advance(skip)
start:
	skip = false
	lookahead = lexer.Lookahead
	switch state {
	case 0:
		eof = lexer.EOF()
		if eof {
			state = 1
			goto nextState
		}
		if lookahead == ' ' || lookahead == '\n' || lookahead == '\t' || lookahead == '\r' {
			skip = true
			state = 0
			goto nextState
		}
		if lookahead >= 'a' && lookahead <= 'z' {
			state = 2
			goto nextState
		}
		return result
	case 1:
		result = true
		lexer.ResultSymbol = toySymEnd
		lexer.MarkEnd()
		return result
	case 2:
		result = true
		lexer.ResultSymbol = toySymWord
		lexer.MarkEnd()
		if lookahead >= 'a' && lookahead <= 'z' {
			state = 2
			goto nextState
		}
		return result
	default:
		return false
	}
}

func toyLanguage() *Language {
	entry := func(count uint8) ParseActionEntry {
		return ParseActionEntry{Count: count, Reusable: true}
	}
	action := func(a ParseAction) ParseActionEntry {
		return ParseActionEntry{Action: a}
	}
	return &Language{
		ABIVersion:          15,
		SymbolCount:         4,
		AliasCount:          0,
		TokenCount:          2,
		StateCount:          6,
		LargeStateCount:     6,
		ProductionIDCount:   1,
		FieldCount:          0,
		MaxAliasSequenceLen: 0,
		ParseTable: []uint16{
			1, 1, 0, 0,
			0, 3, 4, 3,
			5, 5, 0, 0,
			7, 11, 0, 0,
			9, 0, 0, 0,
			13, 13, 0, 0,
		},
		ParseActions: []ParseActionEntry{
			{},
			entry(1),
			action(ParseAction{Type: ParseActionTypeRecover}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeShift, State: 2}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeReduce, Symbol: toySymRepeat, ChildCount: 1}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeReduce, Symbol: toySymSourceFile, ChildCount: 1}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeAccept}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeShift, State: 5}),
			entry(1),
			action(ParseAction{Type: ParseActionTypeReduce, Symbol: toySymRepeat, ChildCount: 2}),
		},
		SymbolNames: []string{"end", "word", "source_file", "aux_source_file_repeat1"},
		FieldNames:  []string{""},
		SymbolMetadataTable: []SymbolMetadata{
			{Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{},
		},
		PublicSymbolMap: []Symbol{0, 1, 2, 3},
		AliasMap:        []uint16{0},
		LexModes: []LexerMode{
			{LexState: 0}, {LexState: 0}, {LexState: 0},
			{LexState: 0}, {LexState: 0}, {LexState: 0},
		},
		LexFn:           toyLex,
		PrimaryStateIDs: []StateID{0, 1, 2, 3, 4, 5},
		Name:            "toy",
	}
}

func parseToy(t *testing.T, source string) *Tree {
	t.Helper()
	parser := NewParser()
	require.True(t, parser.SetLanguage(toyLanguage()))
	tree := parser.ParseString(nil, []byte(source))
	require.NotNil(t, tree)
	return tree
}

func TestToyParsesWords(t *testing.T) {
	tree := parseToy(t, "hello world")
	assert.Equal(t, "(source_file (word) (word))", tree.String())

	root := tree.RootNode()
	assert.Equal(t, "source_file", root.Type())
	assert.Equal(t, uint32(2), root.NamedChildCount())
	assert.Equal(t, uint32(0), root.StartByte())
	assert.Equal(t, uint32(11), root.EndByte())
	assert.Equal(t, Point{Row: 0, Column: 11}, root.EndPoint())
	assert.False(t, root.HasError())

	first := root.NamedChild(0)
	assert.Equal(t, "word", first.Type())
	assert.Equal(t, uint32(0), first.StartByte())
	assert.Equal(t, uint32(5), first.EndByte())

	second := first.NextNamedSibling()
	require.False(t, second.IsNull())
	assert.Equal(t, uint32(6), second.StartByte())
	assert.True(t, second.PrevNamedSibling().Equal(first))
	assert.True(t, first.Parent().Equal(root))
	assert.True(t, root.Parent().IsNull())
}

func TestToySpansSeveralLines(t *testing.T) {
	tree := parseToy(t, "alpha\nbeta gamma\n")
	assert.Equal(t, "(source_file (word) (word) (word))", tree.String())

	root := tree.RootNode()
	beta := root.NamedChild(1)
	assert.Equal(t, Point{Row: 1, Column: 0}, beta.StartPoint())
	assert.Equal(t, Point{Row: 1, Column: 4}, beta.EndPoint())

	found := root.NamedDescendantForPointRange(Point{Row: 1, Column: 1}, Point{Row: 1, Column: 2})
	assert.Equal(t, "word", found.Type())
	assert.Equal(t, uint32(6), found.StartByte())

	byRange := root.DescendantForByteRange(11, 12)
	assert.Equal(t, uint32(11), byRange.StartByte())
}

func TestToyRecoversFromUnexpectedInput(t *testing.T) {
	tree := parseToy(t, "alpha ### beta")
	root := tree.RootNode()
	assert.True(t, root.HasError())
	assert.Contains(t, tree.String(), "ERROR")
	assert.Equal(t, "source_file", root.Type())
}

func TestToyWalksWithACursor(t *testing.T) {
	tree := parseToy(t, "one two three")
	cursor := tree.Walk()
	assert.Equal(t, "source_file", cursor.Node().Type())
	require.True(t, cursor.GotoFirstChild())
	assert.Equal(t, "word", cursor.Node().Type())
	assert.Equal(t, uint32(1), cursor.Depth())
	require.True(t, cursor.GotoNextSibling())
	assert.Equal(t, uint32(4), cursor.Node().StartByte())
	require.True(t, cursor.GotoPreviousSibling())
	assert.Equal(t, uint32(0), cursor.Node().StartByte())
	require.True(t, cursor.GotoParent())
	assert.Equal(t, "source_file", cursor.Node().Type())
	assert.False(t, cursor.GotoParent())

	copied := cursor.Copy()
	copied.Reset(tree.RootNode().NamedChild(2))
	assert.Equal(t, uint32(8), copied.Node().StartByte())
}

func TestToyReparsesAfterAnEdit(t *testing.T) {
	parser := NewParser()
	require.True(t, parser.SetLanguage(toyLanguage()))
	source := []byte("alpha beta")
	tree := parser.ParseString(nil, source)
	require.NotNil(t, tree)

	edited := []byte("alpha beta gamma")
	tree.Edit(InputEdit{
		StartByte:   10,
		OldEndByte:  10,
		NewEndByte:  16,
		StartPoint:  Point{Row: 0, Column: 10},
		OldEndPoint: Point{Row: 0, Column: 10},
		NewEndPoint: Point{Row: 0, Column: 16},
	})
	next := parser.ParseString(tree, edited)
	require.NotNil(t, next)
	assert.Equal(t, "(source_file (word) (word) (word))", next.String())
	assert.Equal(t, uint32(3), next.RootNode().NamedChildCount())
}

func TestToyReportsLanguageDetails(t *testing.T) {
	language := toyLanguage()
	assert.Equal(t, "toy", language.LanguageName())
	assert.Equal(t, uint32(4), language.SymbolCountTotal())
	assert.Equal(t, toySymWord, language.SymbolForName("word", true))
	assert.Equal(t, "ERROR", language.SymbolName(BuiltinSymError))
	assert.Equal(t, "_ERROR", language.SymbolName(builtinSymErrorRepeat))
	assert.True(t, language.StateIsPrimary(3))
	assert.Nil(t, language.Supertypes())
	assert.Nil(t, language.Subtypes(toySymWord))
	assert.Equal(t, []Symbol{toySymWord}, language.AliasesForSymbol(toySymWord))
	assert.Equal(t, Symbol(0), language.AliasAt(0, 0))
	assert.Equal(t, "", language.FieldNameForID(0))
	assert.Equal(t, FieldID(0), language.FieldIDForName("missing"))
}

func TestParserRejectsAnUnsupportedLanguage(t *testing.T) {
	parser := NewParser()
	assert.False(t, parser.SetLanguage(&Language{ABIVersion: 99}))
	assert.False(t, parser.SetLanguage(&Language{ABIVersion: 15}))
	assert.True(t, parser.SetLanguage(nil))
	assert.Nil(t, parser.ParseString(nil, []byte("x")))
}

func TestIncludedRangesLimitParsing(t *testing.T) {
	parser := NewParser()
	require.True(t, parser.SetLanguage(toyLanguage()))
	require.True(t, parser.SetIncludedRanges([]Range{{
		StartByte:  6,
		EndByte:    10,
		StartPoint: Point{Row: 0, Column: 6},
		EndPoint:   Point{Row: 0, Column: 10},
	}}))
	assert.Len(t, parser.IncludedRanges(), 1)
	tree := parser.ParseString(nil, []byte("alpha beta gamma"))
	require.NotNil(t, tree)
	assert.Equal(t, "(source_file (word))", tree.String())
	assert.Equal(t, uint32(6), tree.RootNode().StartByte())
	assert.False(t, parser.SetIncludedRanges([]Range{{StartByte: 5, EndByte: 1}}))
}

func TestTreeCopyKeepsTheSameShape(t *testing.T) {
	tree := parseToy(t, "alpha beta")
	other := tree.Copy()
	assert.Equal(t, tree.String(), other.String())
	assert.Equal(t, tree.Language(), other.Language())
	assert.Len(t, other.IncludedRanges(), 1)
}
