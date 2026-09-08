package treesitter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeUTF8ReadsWellFormedSequences(t *testing.T) {
	cases := []struct {
		input []byte
		want  int32
		size  uint32
	}{
		{[]byte("a"), 'a', 1},
		{[]byte("\x00"), 0, 1},
		{[]byte("\x7f"), 0x7F, 1},
		{[]byte("é"), 0xE9, 2},
		{[]byte("€"), 0x20AC, 3},
		{[]byte("\U0001F600"), 0x1F600, 4},
		{[]byte{0xEF, 0xBB, 0xBF}, 0xFEFF, 3},
	}
	for _, c := range cases {
		got, size := DecodeUTF8(c.input)
		assert.Equal(t, c.want, got, "input %q", c.input)
		assert.Equal(t, c.size, size, "input %q", c.input)
	}
}

func TestDecodeUTF8RejectsIllFormedSequences(t *testing.T) {
	bad := [][]byte{
		{},
		{0x80},
		{0xC0, 0x80},
		{0xC1, 0xBF},
		{0xE0, 0x80, 0x80},
		{0xED, 0xA0, 0x80},
		{0xF5, 0x80, 0x80, 0x80},
		{0xE2, 0x82},
		{0xF0},
	}
	for _, input := range bad {
		got, _ := DecodeUTF8(input)
		assert.Equal(t, int32(-1), got, "input %x", input)
	}
}

func TestDecodeUTF16ReadsBothByteOrders(t *testing.T) {
	value, size := DecodeUTF16LE([]byte{0x41, 0x00})
	assert.Equal(t, 'A', value)
	assert.Equal(t, uint32(2), size)

	value, size = DecodeUTF16BE([]byte{0x00, 0x41})
	assert.Equal(t, 'A', value)
	assert.Equal(t, uint32(2), size)

	value, size = DecodeUTF16LE([]byte{0x3D, 0xD8, 0x00, 0xDE})
	assert.Equal(t, int32(0x1F600), value)
	assert.Equal(t, uint32(4), size)

	value, size = DecodeUTF16BE([]byte{0xD8, 0x3D, 0xDE, 0x00})
	assert.Equal(t, int32(0x1F600), value)
	assert.Equal(t, uint32(4), size)

	value, _ = DecodeUTF16LE([]byte{0x41})
	assert.Equal(t, int32(-1), value)
	value, _ = DecodeUTF16BE([]byte{0x41})
	assert.Equal(t, int32(-1), value)
}

func TestSetContainsSearchesSortedRanges(t *testing.T) {
	ranges := []CharacterRange{
		{Start: 'A', End: 'Z'},
		{Start: '_', End: '_'},
		{Start: 'a', End: 'z'},
		{Start: 0x100, End: 0x200},
	}
	assert.True(t, SetContains(ranges, 'A'))
	assert.True(t, SetContains(ranges, 'Q'))
	assert.True(t, SetContains(ranges, '_'))
	assert.True(t, SetContains(ranges, 'z'))
	assert.True(t, SetContains(ranges, 0x180))
	assert.False(t, SetContains(ranges, '0'))
	assert.False(t, SetContains(ranges, '['))
	assert.False(t, SetContains(ranges, 0x300))
}

func TestPointArithmeticMatchesTheReference(t *testing.T) {
	assert.Equal(t, Point{Row: 1, Column: 3}, pointAdd(Point{Row: 0, Column: 5}, Point{Row: 1, Column: 3}))
	assert.Equal(t, Point{Row: 0, Column: 8}, pointAdd(Point{Row: 0, Column: 5}, Point{Row: 0, Column: 3}))
	assert.Equal(t, Point{Row: 1, Column: 5}, pointSub(Point{Row: 2, Column: 5}, Point{Row: 1, Column: 9}))
	assert.Equal(t, Point{Row: 0, Column: 2}, pointSub(Point{Row: 1, Column: 5}, Point{Row: 1, Column: 3}))
	assert.Equal(t, Point{}, pointSub(Point{Row: 1, Column: 1}, Point{Row: 1, Column: 3}))
	assert.True(t, pointLt(Point{Row: 0, Column: 1}, Point{Row: 1, Column: 0}))
	assert.True(t, pointLte(Point{Row: 1, Column: 0}, Point{Row: 1, Column: 0}))
	assert.True(t, pointGt(Point{Row: 1, Column: 1}, Point{Row: 1, Column: 0}))
	assert.True(t, pointEq(Point{Row: 1, Column: 1}, Point{Row: 1, Column: 1}))
}

func TestLengthArithmeticSaturates(t *testing.T) {
	a := length{Bytes: 10, Extent: Point{Row: 1, Column: 4}}
	b := length{Bytes: 4, Extent: Point{Row: 0, Column: 2}}
	assert.Equal(t, uint32(14), lengthAdd(a, b).Bytes)
	assert.Equal(t, uint32(6), lengthSub(a, b).Bytes)
	assert.Equal(t, uint32(0), lengthSub(b, a).Bytes)
	assert.Equal(t, uint32(0), lengthSaturatingSub(b, a).Bytes)
	assert.Equal(t, uint32(6), lengthSaturatingSub(a, b).Bytes)
	assert.True(t, lengthIsUndefined(lengthUndefined))
	assert.False(t, lengthIsUndefined(lengthZero()))
}

func TestRangeArrayReportsDifferences(t *testing.T) {
	old := []Range{{StartByte: 0, EndByte: 10, EndPoint: Point{Column: 10}}}
	next := []Range{{StartByte: 0, EndByte: 4, EndPoint: Point{Column: 4}}}
	var differences []Range
	rangeArrayGetChangedRanges(old, next, &differences)
	assert.Len(t, differences, 1)
	assert.Equal(t, uint32(4), differences[0].StartByte)
	assert.Equal(t, uint32(10), differences[0].EndByte)

	assert.True(t, rangeArrayIntersects(differences, 0, 5, 6))
	assert.False(t, rangeArrayIntersects(differences, 0, 0, 3))
	assert.False(t, rangeArrayIntersects(differences, 1, 5, 6))
}
