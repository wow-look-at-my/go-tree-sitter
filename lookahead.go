package treesitter

type lookaheadPhase int

const (
	lookaheadFresh lookaheadPhase = iota
	lookaheadPositioned
	lookaheadDone
)

type lookaheadIterator struct {
	language     *Language
	data         []uint16
	pos          int
	groupEndPos  int
	hasGroupEnd  bool
	tableValue   uint16
	groupCount   uint16
	isSmallState bool
	phase        lookaheadPhase

	actions     []ParseActionEntry
	symbol      Symbol
	nextState   StateID
	actionCount uint16
}

func (l *Language) lookaheads(state StateID) lookaheadIterator {
	isSmall := uint32(state) >= l.LargeStateCount
	it := lookaheadIterator{
		language:     l,
		isSmallState: isSmall,
		phase:        lookaheadFresh,
		symbol:       0xFFFF,
	}
	if isSmall {
		index := l.SmallParseTableMap[uint32(state)-l.LargeStateCount]
		it.data = l.SmallParseTable
		it.pos = int(index)
		it.groupEndPos = it.pos + 1
		it.hasGroupEnd = true
		it.groupCount = l.SmallParseTable[index]
	} else {
		it.data = l.ParseTable
		it.pos = int(uint32(state) * l.SymbolCount)
	}
	return it
}

func (it *lookaheadIterator) next() bool {
	if it.phase == lookaheadDone {
		return false
	}

	if it.isSmallState {
		it.pos++
		if it.pos == it.groupEndPos {
			if it.groupCount == 0 {
				it.phase = lookaheadDone
				return false
			}
			it.groupCount--
			it.tableValue = it.data[it.pos]
			it.pos++
			symbolCount := int(it.data[it.pos])
			it.pos++
			it.groupEndPos = it.pos + symbolCount
			it.symbol = it.data[it.pos]
		} else {
			it.symbol = it.data[it.pos]
			it.phase = lookaheadPositioned
			return true
		}
	} else {
		rowBase := it.pos
		var symbol uint32
		if it.phase != lookaheadFresh {
			symbol = uint32(it.symbol) + 1
		}
		for symbol < it.language.SymbolCount && it.data[rowBase+int(symbol)] == 0 {
			symbol++
		}
		if symbol >= it.language.SymbolCount {
			it.phase = lookaheadDone
			return false
		}
		it.symbol = Symbol(symbol)
		it.tableValue = it.data[rowBase+int(symbol)]
	}

	if uint32(it.symbol) < it.language.TokenCount {
		actions, count, _ := it.language.actionsAt(it.tableValue)
		it.actionCount = uint16(count)
		it.actions = actions
		it.nextState = 0
	} else {
		it.actionCount = 0
		it.nextState = it.tableValue
	}
	it.phase = lookaheadPositioned
	return true
}
