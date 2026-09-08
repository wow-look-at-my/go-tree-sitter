package treesitter

// The inline arm of a subtree, and every accessor that has to ask which arm it
// is looking at.
//
// Upstream packs these fields into the same word as the heap pointer and tells
// the arms apart by the pointer's low bit. Go cannot do that, so the fields sit
// beside the pointer and flagInline is the discriminant. The field widths are
// upstream's, because they decide which leaves qualify: a leaf that does not
// fit any one of them goes to the heap instead.

const (
	flagInline uint8 = 1 << iota
	flagVisible
	flagNamed
	flagExtra
	flagHasChanges
	flagIsMissing
	flagIsKeyword
)

// maxInlineLength bounds every byte-width field of an inline leaf. Upstream
// compares strictly, so the largest value that fits is one below it.
const maxInlineLength = 255

type subtreeInline struct {
	paddingColumns uint8
	paddingRows    uint8
	lookaheadBytes uint8
	paddingBytes   uint8
	sizeBytes      uint8
	symbol         uint8
	flags          uint8
	parseState     StateID
}

func (s subtree) isInline() bool { return s.inline.flags&flagInline != 0 }

func (s subtree) flag(bit uint8) bool { return s.inline.flags&bit != 0 }

// canInline reports a leaf whose extent fits the inline field widths. A token
// that spans a row cannot: the inline arm stores no size extent and rebuilds it
// as a single row, so only a token that ends on the row it started can be
// rebuilt exactly.
func canInline(padding, size length, lookaheadBytes uint32) bool {
	return padding.Bytes < maxInlineLength &&
		padding.Extent.Row < 16 &&
		padding.Extent.Column < maxInlineLength &&
		size.Bytes < maxInlineLength &&
		size.Extent.Row == 0 &&
		size.Extent.Column < maxInlineLength &&
		lookaheadBytes < 16
}

func inlineLeaf(
	symbol Symbol, padding, size length, lookaheadBytes uint32, parseState StateID,
	visible, named, extra, isKeyword bool,
) subtree {
	flags := flagInline
	for bit, on := range map[uint8]bool{
		flagVisible:   visible,
		flagNamed:     named,
		flagExtra:     extra,
		flagIsKeyword: isKeyword,
	} {
		if on {
			flags |= bit
		}
	}
	return subtree{inline: subtreeInline{
		paddingColumns: uint8(padding.Extent.Column),
		paddingRows:    uint8(padding.Extent.Row),
		lookaheadBytes: uint8(lookaheadBytes),
		paddingBytes:   uint8(padding.Bytes),
		sizeBytes:      uint8(size.Bytes),
		symbol:         uint8(symbol),
		flags:          flags,
		parseState:     parseState,
	}}
}

func subtreeSymbol(self subtree) Symbol {
	if self.isInline() {
		return Symbol(self.inline.symbol)
	}
	return self.heap.symbol
}

func subtreeVisible(self subtree) bool {
	if self.isInline() {
		return self.flag(flagVisible)
	}
	return self.heap.visible
}

func subtreeNamed(self subtree) bool {
	if self.isInline() {
		return self.flag(flagNamed)
	}
	return self.heap.named
}

func subtreeExtra(self subtree) bool {
	if self.isInline() {
		return self.flag(flagExtra)
	}
	return self.heap.extra
}

func subtreeHasChanges(self subtree) bool {
	if self.isInline() {
		return self.flag(flagHasChanges)
	}
	return self.heap.hasChanges
}

func subtreeMissing(self subtree) bool {
	if self.isInline() {
		return self.flag(flagIsMissing)
	}
	return self.heap.isMissing
}

func subtreeIsKeyword(self subtree) bool {
	if self.isInline() {
		return self.flag(flagIsKeyword)
	}
	return self.heap.isKeyword
}

func subtreeParseState(self subtree) StateID {
	if self.isInline() {
		return self.inline.parseState
	}
	return self.heap.parseState
}

func subtreeLookaheadBytes(self subtree) uint32 {
	if self.isInline() {
		return uint32(self.inline.lookaheadBytes)
	}
	return self.heap.lookaheadBytes
}

// subtreeChildren answers the empty slice for an inline leaf, which has none.
func subtreeChildren(self subtree) []subtree {
	if self.isInline() {
		return nil
	}
	return self.heap.children
}

func subtreeChildCount(self subtree) uint32 {
	if self.isInline() || self.isNil() {
		return 0
	}
	return uint32(len(self.heap.children))
}

func subtreeLeafSymbol(self subtree) Symbol {
	if self.isInline() {
		return Symbol(self.inline.symbol)
	}
	if len(self.heap.children) == 0 {
		return self.heap.symbol
	}
	return self.heap.firstLeafSymbol
}

func subtreeLeafParseState(self subtree) StateID {
	if self.isInline() {
		return self.inline.parseState
	}
	if len(self.heap.children) == 0 {
		return self.heap.parseState
	}
	return self.heap.firstLeafParseState
}

func subtreePadding(self subtree) length {
	if self.isInline() {
		return length{
			Bytes: uint32(self.inline.paddingBytes),
			Extent: Point{
				Row:    uint32(self.inline.paddingRows),
				Column: uint32(self.inline.paddingColumns),
			},
		}
	}
	return self.heap.padding
}

// subtreeSize rebuilds an inline extent as a single row, which is exactly what
// canInline admits and nothing wider.
func subtreeSize(self subtree) length {
	if self.isInline() {
		return length{
			Bytes:  uint32(self.inline.sizeBytes),
			Extent: Point{Row: 0, Column: uint32(self.inline.sizeBytes)},
		}
	}
	return self.heap.size
}

func subtreeTotalSize(self subtree) length {
	return lengthAdd(subtreePadding(self), subtreeSize(self))
}

func subtreeTotalBytes(self subtree) uint32 { return subtreeTotalSize(self).Bytes }

func subtreeRepeatDepth(self subtree) uint32 {
	if self.isInline() {
		return 0
	}
	return uint32(self.heap.repeatDepth)
}

func subtreeVisibleChildCount(self subtree) uint32 {
	if subtreeChildCount(self) > 0 {
		return self.heap.visibleChildCount
	}
	return 0
}

func subtreeNamedChildCount(self subtree) uint32 {
	if subtreeChildCount(self) > 0 {
		return self.heap.namedChildCount
	}
	return 0
}

func subtreeProductionID(self subtree) uint16 {
	if subtreeChildCount(self) > 0 {
		return self.heap.productionID
	}
	return 0
}

func subtreeVisibleDescendantCount(self subtree) uint32 {
	if subtreeChildCount(self) == 0 {
		return 0
	}
	return self.heap.visibleDescendantCount
}

func subtreeErrorCost(self subtree) uint32 {
	if subtreeMissing(self) {
		return errorCostPerMissingTree + errorCostPerRecovery
	}
	if self.isInline() {
		return 0
	}
	return self.heap.errorCost
}

func subtreeDynamicPrecedence(self subtree) int32 {
	if subtreeChildCount(self) == 0 {
		return 0
	}
	return self.heap.dynamicPrecedence
}

// An inline leaf carries no fragility, no external token and no column
// dependence: canInline and newLeafSubtree between them keep every subtree that
// would need one on the heap.
func subtreeFragileLeft(self subtree) bool {
	return !self.isInline() && self.heap.fragileLeft
}

func subtreeFragileRight(self subtree) bool {
	return !self.isInline() && self.heap.fragileRight
}

func subtreeIsFragile(self subtree) bool {
	return !self.isInline() && (self.heap.fragileLeft || self.heap.fragileRight)
}

func subtreeHasExternalTokens(self subtree) bool {
	return !self.isInline() && self.heap.hasExternalTokens
}

func subtreeHasExternalScannerStateChange(self subtree) bool {
	return !self.isInline() && self.heap.hasExternalScannerStateChange
}

func subtreeDependsOnColumn(self subtree) bool {
	return !self.isInline() && self.heap.dependsOnColumn
}

func subtreeLookaheadChar(self subtree) int32 {
	if self.isInline() {
		return 0
	}
	return self.heap.lookaheadChar
}

// subtreeRefCount answers one for an inline leaf: it is held by value, so the
// holder is its only owner and it is always safe to mutate in place.
func subtreeRefCount(self subtree) uint32 {
	if self.isInline() {
		return 1
	}
	return self.heap.refCount
}

func subtreeIsError(self subtree) bool { return subtreeSymbol(self) == BuiltinSymError }
func subtreeIsEOF(self subtree) bool   { return subtreeSymbol(self) == BuiltinSymEnd }

func subtreeSetExtra(self *subtree, v bool) {
	if self.isInline() {
		self.inline.setFlag(flagExtra, v)
		return
	}
	self.heap.extra = v
}

func subtreeSetHasChanges(self *subtree, v bool) {
	if self.isInline() {
		self.inline.setFlag(flagHasChanges, v)
		return
	}
	self.heap.hasChanges = v
}

func (i *subtreeInline) setFlag(bit uint8, v bool) {
	if v {
		i.flags |= bit
		return
	}
	i.flags &^= bit
}

// subtreeResize records an edit's new padding and size. An inline leaf that no
// longer fits the inline widths moves to the heap rather than being truncated
// into them.
func subtreeResize(self *subtree, padding, size length, lookaheadBytes uint32) {
	if !self.isInline() {
		self.heap.padding = padding
		self.heap.size = size
		return
	}
	if canInline(padding, size, lookaheadBytes) {
		self.inline.paddingBytes = uint8(padding.Bytes)
		self.inline.paddingRows = uint8(padding.Extent.Row)
		self.inline.paddingColumns = uint8(padding.Extent.Column)
		self.inline.sizeBytes = uint8(size.Bytes)
		return
	}
	*self = heapSubtree(&subtreeData{
		refCount:       1,
		padding:        padding,
		size:           size,
		lookaheadBytes: lookaheadBytes,
		symbol:         Symbol(self.inline.symbol),
		parseState:     self.inline.parseState,
		visible:        self.flag(flagVisible),
		named:          self.flag(flagNamed),
		extra:          self.flag(flagExtra),
		isMissing:      self.flag(flagIsMissing),
		isKeyword:      self.flag(flagIsKeyword),
	})
}
