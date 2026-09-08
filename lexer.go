package treesitter

// Range is a contiguous region of a document.
type Range struct {
	StartPoint Point
	EndPoint   Point
	StartByte  uint32
	EndByte    uint32
}

// InputEncoding selects how the source bytes are decoded.
type InputEncoding int

// Supported source encodings.
const (
	EncodingUTF8 InputEncoding = iota
	EncodingUTF16LE
	EncodingUTF16BE
	EncodingCustom
)

// Input supplies source text to the parser in chunks.
type Input struct {
	Read     func(byteOffset uint32, position Point) []byte
	Encoding InputEncoding
	Decode   func([]byte) (int32, uint32)
}

const byteOrderMark int32 = 0xFEFF

var defaultRange = Range{
	StartPoint: Point{},
	EndPoint:   Point{Row: ^uint32(0), Column: ^uint32(0)},
	StartByte:  0,
	EndByte:    ^uint32(0),
}

type columnData struct {
	value uint32
	valid bool
}

// Lexer scans the source text on behalf of a grammar's lex functions.
type Lexer struct {
	// Lookahead is the code point at the current position.
	Lookahead int32
	// ResultSymbol is set by a lex function when it accepts a token.
	ResultSymbol Symbol

	currentPosition    length
	tokenStartPosition length
	tokenEndPosition   length

	includedRanges            []Range
	chunk                     []byte
	input                     Input
	currentIncludedRangeIndex uint32
	chunkStart                uint32
	lookaheadSize             uint32
	didGetColumn              bool
	columnData                columnData
}

func (l *Lexer) decodeFn() func([]byte) (int32, uint32) {
	switch l.input.Encoding {
	case EncodingUTF8:
		return DecodeUTF8
	case EncodingUTF16LE:
		return DecodeUTF16LE
	case EncodingUTF16BE:
		return DecodeUTF16BE
	default:
		return l.input.Decode
	}
}

func (l *Lexer) setColumnData(v uint32) {
	l.columnData.valid = true
	l.columnData.value = v
}

func (l *Lexer) incrementColumnData() {
	if l.columnData.valid {
		l.columnData.value++
	}
}

func (l *Lexer) invalidateColumnData() {
	l.columnData.valid = false
	l.columnData.value = 0
}

// EOF reports whether the lexer consumed all included ranges.
func (l *Lexer) EOF() bool {
	return l.currentIncludedRangeIndex == uint32(len(l.includedRanges))
}

func (l *Lexer) clearChunk() {
	l.chunk = nil
	l.chunkStart = 0
}

func (l *Lexer) getChunk() {
	l.chunkStart = l.currentPosition.Bytes
	l.chunk = l.input.Read(l.currentPosition.Bytes, l.currentPosition.Extent)
	if len(l.chunk) == 0 {
		l.currentIncludedRangeIndex = uint32(len(l.includedRanges))
		l.chunk = nil
	}
}

func (l *Lexer) getLookahead() {
	positionInChunk := l.currentPosition.Bytes - l.chunkStart
	size := uint32(len(l.chunk)) - positionInChunk

	if size == 0 {
		l.lookaheadSize = 1
		l.Lookahead = 0
		return
	}

	chunk := l.chunk[positionInChunk:]

	if l.input.Encoding == EncodingUTF8 && chunk[0] < 0x80 {
		l.Lookahead = int32(chunk[0])
		l.lookaheadSize = 1
		return
	}

	decode := l.decodeFn()
	l.Lookahead, l.lookaheadSize = decode(chunk)

	if l.Lookahead == decodeError && size < 4 {
		l.getChunk()
		l.Lookahead, l.lookaheadSize = decode(l.chunk)
	}

	if l.Lookahead == decodeError {
		l.lookaheadSize = 1
	}
}

func (l *Lexer) gotoPosition(position length) {
	if position.Bytes != l.currentPosition.Bytes {
		l.invalidateColumnData()
	}

	l.currentPosition = position

	foundIncludedRange := false
	for i := range l.includedRanges {
		r := &l.includedRanges[i]
		if r.EndByte > l.currentPosition.Bytes && r.EndByte > r.StartByte {
			if r.StartByte >= l.currentPosition.Bytes {
				l.currentPosition = length{Bytes: r.StartByte, Extent: r.StartPoint}
			}
			l.currentIncludedRangeIndex = uint32(i)
			foundIncludedRange = true
			break
		}
	}

	if foundIncludedRange {
		if l.chunk != nil && (l.currentPosition.Bytes < l.chunkStart ||
			l.currentPosition.Bytes >= l.chunkStart+uint32(len(l.chunk))) {
			l.clearChunk()
		}
		l.lookaheadSize = 0
		l.Lookahead = 0
	} else {
		l.currentIncludedRangeIndex = uint32(len(l.includedRanges))
		last := l.includedRanges[len(l.includedRanges)-1]
		l.currentPosition = length{Bytes: last.EndByte, Extent: last.EndPoint}
		l.clearChunk()
		l.lookaheadSize = 1
		l.Lookahead = 0
	}
}

func (l *Lexer) doAdvance(skip bool) {
	if l.lookaheadSize != 0 {
		if l.Lookahead == '\n' {
			l.currentPosition.Extent.Row++
			l.currentPosition.Extent.Column = 0
			l.setColumnData(0)
		} else {
			isBOM := l.currentPosition.Bytes == 0 && l.Lookahead == byteOrderMark
			if !isBOM {
				l.incrementColumnData()
			}
			l.currentPosition.Extent.Column += l.lookaheadSize
		}
		l.currentPosition.Bytes += l.lookaheadSize
	}

	hasRange := true
	idx := l.currentIncludedRangeIndex
	for {
		if idx >= uint32(len(l.includedRanges)) {
			hasRange = false
			break
		}
		r := l.includedRanges[idx]
		if !(l.currentPosition.Bytes >= r.EndByte || r.EndByte == r.StartByte) {
			break
		}
		if l.currentIncludedRangeIndex < uint32(len(l.includedRanges)) {
			l.currentIncludedRangeIndex++
		}
		if l.currentIncludedRangeIndex < uint32(len(l.includedRanges)) {
			idx = l.currentIncludedRangeIndex
			next := l.includedRanges[idx]
			l.currentPosition = length{Bytes: next.StartByte, Extent: next.StartPoint}
		} else {
			hasRange = false
			break
		}
	}

	if skip {
		l.tokenStartPosition = l.currentPosition
	}

	if hasRange {
		if l.currentPosition.Bytes < l.chunkStart ||
			l.currentPosition.Bytes >= l.chunkStart+uint32(len(l.chunk)) {
			l.getChunk()
		}
		l.getLookahead()
	} else {
		l.clearChunk()
		l.Lookahead = 0
		l.lookaheadSize = 1
	}
}

// Advance consumes the current code point.
func (l *Lexer) Advance(skip bool) {
	if l.chunk == nil {
		return
	}

	nextPosition := l.currentPosition.Bytes + 1
	currentRangeEnd := l.includedRanges[l.currentIncludedRangeIndex].EndByte
	if l.input.Encoding == EncodingUTF8 && l.lookaheadSize == 1 &&
		l.Lookahead != '\n' && nextPosition < currentRangeEnd &&
		nextPosition < l.chunkStart+uint32(len(l.chunk)) {
		nextByte := l.chunk[nextPosition-l.chunkStart]
		if nextByte < 0x80 {
			l.incrementColumnData()
			l.currentPosition.Bytes++
			l.currentPosition.Extent.Column++
			if skip {
				l.tokenStartPosition = l.currentPosition
			}
			l.Lookahead = int32(nextByte)
			return
		}
	}

	l.doAdvance(skip)
}

// MarkEnd records the end of the token being scanned.
func (l *Lexer) MarkEnd() {
	if !l.EOF() {
		current := l.includedRanges[l.currentIncludedRangeIndex]
		if l.currentIncludedRangeIndex > 0 && l.currentPosition.Bytes == current.StartByte {
			previous := l.includedRanges[l.currentIncludedRangeIndex-1]
			l.tokenEndPosition = length{Bytes: previous.EndByte, Extent: previous.EndPoint}
			return
		}
	}
	l.tokenEndPosition = l.currentPosition
}

// GetColumn reports the column of the current position.
func (l *Lexer) GetColumn() uint32 {
	l.didGetColumn = true

	if !l.columnData.valid {
		goalByte := l.currentPosition.Bytes

		startOfCol := length{
			Bytes:  l.currentPosition.Bytes - l.currentPosition.Extent.Column,
			Extent: Point{Row: l.currentPosition.Extent.Row, Column: 0},
		}
		l.gotoPosition(startOfCol)
		l.setColumnData(0)
		l.getChunk()

		if !l.EOF() {
			l.getLookahead()
			for l.currentPosition.Bytes < goalByte && !l.EOF() && l.chunk != nil {
				l.doAdvance(false)
				if l.EOF() {
					break
				}
			}
		}
	}

	return l.columnData.value
}

// IsAtIncludedRangeStart reports whether the lexer sits on a range boundary.
func (l *Lexer) IsAtIncludedRangeStart() bool {
	if l.currentIncludedRangeIndex < uint32(len(l.includedRanges)) {
		return l.currentPosition.Bytes == l.includedRanges[l.currentIncludedRangeIndex].StartByte
	}
	return false
}

func (l *Lexer) init() {
	*l = Lexer{}
	l.setIncludedRanges(nil)
}

func (l *Lexer) setInput(input Input) {
	l.input = input
	l.clearChunk()
	l.gotoPosition(l.currentPosition)
}

func (l *Lexer) reset(position length) {
	if position.Bytes != l.currentPosition.Bytes {
		l.gotoPosition(position)
	}
}

func (l *Lexer) start() {
	l.tokenStartPosition = l.currentPosition
	l.tokenEndPosition = lengthUndefined
	l.ResultSymbol = 0
	l.didGetColumn = false
	if !l.EOF() {
		if len(l.chunk) == 0 {
			l.getChunk()
		}
		if l.lookaheadSize == 0 {
			l.getLookahead()
		}
		if l.currentPosition.Bytes == 0 {
			if l.Lookahead == byteOrderMark {
				l.Advance(true)
			}
			l.setColumnData(0)
		}
	}
}

func (l *Lexer) finish(lookaheadEndByte *uint32) {
	if lengthIsUndefined(l.tokenEndPosition) {
		l.MarkEnd()
	}

	if l.tokenEndPosition.Bytes < l.tokenStartPosition.Bytes {
		l.tokenStartPosition = l.tokenEndPosition
	}

	currentLookaheadEndByte := l.currentPosition.Bytes + 1
	if l.Lookahead == decodeError {
		currentLookaheadEndByte += 4
	}
	if currentLookaheadEndByte > *lookaheadEndByte {
		*lookaheadEndByte = currentLookaheadEndByte
	}
}

func (l *Lexer) setIncludedRanges(ranges []Range) bool {
	if len(ranges) == 0 {
		ranges = []Range{defaultRange}
	} else {
		previousByte := uint32(0)
		for i := range ranges {
			r := ranges[i]
			if r.StartByte < previousByte || r.EndByte < r.StartByte {
				return false
			}
			previousByte = r.EndByte
		}
	}
	l.includedRanges = append([]Range(nil), ranges...)
	l.gotoPosition(l.currentPosition)
	return true
}
