package treesitter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLexer(source string) *Lexer {
	in := &stringInput{data: []byte(source)}
	lexer := &Lexer{}
	lexer.init()
	lexer.setInput(Input{Read: in.read, Encoding: EncodingUTF8})
	return lexer
}

func TestLexerReportsTheColumnAfterAJump(t *testing.T) {
	lexer := newTestLexer("alpha\nbeta gamma\n")
	lexer.start()
	assert.Equal(t, uint32(0), lexer.GetColumn())

	for i := 0; i < 8; i++ {
		lexer.Advance(false)
	}
	assert.Equal(t, uint32(2), lexer.GetColumn())
	assert.Equal(t, Point{Row: 1, Column: 2}, lexer.currentPosition.Extent)

	lexer.gotoPosition(length{Bytes: 12, Extent: Point{Row: 1, Column: 6}})
	assert.False(t, lexer.columnData.valid)
	assert.Equal(t, uint32(6), lexer.GetColumn())
}

func TestLexerWalksToTheEndOfInput(t *testing.T) {
	lexer := newTestLexer("ab")
	lexer.start()
	assert.False(t, lexer.EOF())
	assert.Equal(t, 'a', lexer.Lookahead)

	lexer.Advance(false)
	assert.Equal(t, 'b', lexer.Lookahead)
	lexer.MarkEnd()
	assert.Equal(t, uint32(1), lexer.tokenEndPosition.Bytes)

	lexer.Advance(false)
	assert.True(t, lexer.EOF())
	assert.Equal(t, int32(0), lexer.Lookahead)

	lookaheadEnd := uint32(0)
	lexer.finish(&lookaheadEnd)
	assert.Equal(t, uint32(3), lookaheadEnd)
}

func TestLexerDecodesMultipleByteCharacters(t *testing.T) {
	lexer := newTestLexer("é€")
	lexer.start()
	assert.Equal(t, int32(0xE9), lexer.Lookahead)
	lexer.Advance(false)
	assert.Equal(t, int32(0x20AC), lexer.Lookahead)
	assert.Equal(t, uint32(2), lexer.currentPosition.Bytes)
}

func TestLexerSkipsAByteOrderMark(t *testing.T) {
	lexer := newTestLexer(string([]byte{0xEF, 0xBB, 0xBF}) + "hi")
	lexer.start()
	assert.Equal(t, 'h', lexer.Lookahead)
	assert.Equal(t, uint32(0), lexer.GetColumn())
}

func TestLexerHonoursIncludedRanges(t *testing.T) {
	lexer := newTestLexer("alpha beta gamma")
	require.True(t, lexer.setIncludedRanges([]Range{
		{StartByte: 0, EndByte: 5, EndPoint: Point{Column: 5}},
		{StartByte: 11, EndByte: 16, StartPoint: Point{Column: 11}, EndPoint: Point{Column: 16}},
	}))
	lexer.start()
	assert.True(t, lexer.IsAtIncludedRangeStart())
	for i := 0; i < 5; i++ {
		lexer.Advance(false)
	}
	assert.Equal(t, uint32(11), lexer.currentPosition.Bytes)
	assert.True(t, lexer.IsAtIncludedRangeStart())
	assert.Equal(t, 'g', lexer.Lookahead)

	lexer.MarkEnd()
	assert.Equal(t, uint32(5), lexer.tokenEndPosition.Bytes)

	for i := 0; i < 5; i++ {
		lexer.Advance(false)
	}
	assert.True(t, lexer.EOF())
	assert.False(t, lexer.IsAtIncludedRangeStart())
}

func TestLexerRejectsRangesOutOfOrder(t *testing.T) {
	lexer := newTestLexer("abcdef")
	assert.False(t, lexer.setIncludedRanges([]Range{
		{StartByte: 4, EndByte: 6},
		{StartByte: 0, EndByte: 2},
	}))
}
