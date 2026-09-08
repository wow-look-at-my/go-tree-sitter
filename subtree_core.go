package treesitter

import "bytes"

const (
	errorState              = 0
	errorCostPerRecovery    = 500
	errorCostPerMissingTree = 110
	errorCostPerSkippedTree = 100
	errorCostPerSkippedLine = 30
	errorCostPerSkippedChar = 1
)

type externalScannerState struct {
	data []byte
}

func (e *externalScannerState) init(data []byte) {
	e.data = append([]byte(nil), data...)
}

func (e *externalScannerState) copy() externalScannerState {
	return externalScannerState{data: append([]byte(nil), e.data...)}
}

func (e *externalScannerState) eq(other []byte) bool {
	return bytes.Equal(e.data, other)
}

type subtreeData struct {
	refCount uint32

	padding        length
	size           length
	lookaheadBytes uint32
	errorCost      uint32
	symbol         Symbol
	parseState     StateID

	visible                       bool
	named                         bool
	extra                         bool
	fragileLeft                   bool
	fragileRight                  bool
	hasChanges                    bool
	hasExternalTokens             bool
	hasExternalScannerStateChange bool
	dependsOnColumn               bool
	isMissing                     bool
	isKeyword                     bool

	visibleChildCount      uint32
	namedChildCount        uint32
	visibleDescendantCount uint32
	dynamicPrecedence      int32
	repeatDepth            uint16
	productionID           uint16
	firstLeafSymbol        Symbol
	firstLeafParseState    StateID

	scannerState  externalScannerState
	lookaheadChar int32

	children []subtree
}

// subtree is upstream's tagged union. Upstream keeps a small leaf token in the
// parent's child array by value and reaches the heap only for the rest, telling
// the arms apart by the low bit of the pointer. Go cannot overlay an integer on
// a pointer, so the arms are separate fields and an empty heap arm is the null
// subtree.
//
// The field is named rather than embedded on purpose. Embedding promotes every
// heap field back onto the value, so a read that belongs on an accessor still
// compiles and faults only when it meets a leaf that is not on the heap. A
// named field makes the compiler list the work instead.
type subtree struct {
	heap *subtreeData
}

func heapSubtree(data *subtreeData) subtree { return subtree{heap: data} }

// isNil reports the null subtree, which callers used to spell as a nil pointer.
func (s subtree) isNil() bool { return s.heap == nil }

func subtreeChildCount(self subtree) uint32 {
	if self.isNil() {
		return 0
	}
	return uint32(len(self.heap.children))
}

func subtreeChildren(self subtree) []subtree { return self.heap.children }

func subtreeSymbol(self subtree) Symbol         { return self.heap.symbol }
func subtreeVisible(self subtree) bool          { return self.heap.visible }
func subtreeNamed(self subtree) bool            { return self.heap.named }
func subtreeExtra(self subtree) bool            { return self.heap.extra }
func subtreeHasChanges(self subtree) bool       { return self.heap.hasChanges }
func subtreeMissing(self subtree) bool          { return self.heap.isMissing }
func subtreeIsKeyword(self subtree) bool        { return self.heap.isKeyword }
func subtreeParseState(self subtree) StateID    { return self.heap.parseState }
func subtreeLookaheadBytes(self subtree) uint32 { return self.heap.lookaheadBytes }
func subtreeLookaheadChar(self subtree) int32   { return self.heap.lookaheadChar }
func subtreeRefCount(self subtree) uint32       { return self.heap.refCount }

func subtreeVisibleChildCount(self subtree) uint32 { return self.heap.visibleChildCount }
func subtreeNamedChildCount(self subtree) uint32   { return self.heap.namedChildCount }
func subtreeProductionID(self subtree) uint16      { return self.heap.productionID }

func subtreeSetHasChanges(self subtree, v bool)         { self.heap.hasChanges = v }
func subtreeSetExtra(self subtree, v bool)              { self.heap.extra = v }
func subtreeSetFragileLeft(self subtree, v bool)        { self.heap.fragileLeft = v }
func subtreeSetFragileRight(self subtree, v bool)       { self.heap.fragileRight = v }
func subtreeSetParseState(self subtree, s StateID)      { self.heap.parseState = s }
func subtreeAddDynamicPrecedence(self subtree, d int32) { self.heap.dynamicPrecedence += d }
func subtreeSetPadding(self subtree, l length)          { self.heap.padding = l }
func subtreeSetSize(self subtree, l length)             { self.heap.size = l }

func subtreeLeafSymbol(self subtree) Symbol {
	if len(self.heap.children) == 0 {
		return self.heap.symbol
	}
	return self.heap.firstLeafSymbol
}

func subtreeLeafParseState(self subtree) StateID {
	if len(self.heap.children) == 0 {
		return self.heap.parseState
	}
	return self.heap.firstLeafParseState
}

func subtreePadding(self subtree) length   { return self.heap.padding }
func subtreeSize(self subtree) length      { return self.heap.size }
func subtreeTotalSize(self subtree) length { return lengthAdd(self.heap.padding, self.heap.size) }
func subtreeTotalBytes(self subtree) uint32 {
	return subtreeTotalSize(self).Bytes
}

func subtreeRepeatDepth(self subtree) uint32 { return uint32(self.heap.repeatDepth) }

func subtreeVisibleDescendantCount(self subtree) uint32 {
	if len(self.heap.children) == 0 {
		return 0
	}
	return self.heap.visibleDescendantCount
}

func subtreeErrorCost(self subtree) uint32 {
	if self.heap.isMissing {
		return errorCostPerMissingTree + errorCostPerRecovery
	}
	return self.heap.errorCost
}

func subtreeDynamicPrecedence(self subtree) int32 {
	if len(self.heap.children) == 0 {
		return 0
	}
	return self.heap.dynamicPrecedence
}

func subtreeFragileLeft(self subtree) bool  { return self.heap.fragileLeft }
func subtreeFragileRight(self subtree) bool { return self.heap.fragileRight }
func subtreeHasExternalTokens(self subtree) bool {
	return self.heap.hasExternalTokens
}

func subtreeHasExternalScannerStateChange(self subtree) bool {
	return self.heap.hasExternalScannerStateChange
}

func subtreeDependsOnColumn(self subtree) bool { return self.heap.dependsOnColumn }

func subtreeIsFragile(self subtree) bool {
	return self.heap.fragileLeft || self.heap.fragileRight
}

func subtreeIsError(self subtree) bool { return self.heap.symbol == BuiltinSymError }
func subtreeIsEOF(self subtree) bool   { return self.heap.symbol == BuiltinSymEnd }

func subtreeRetain(self subtree) {
	if self.isNil() {
		return
	}
	self.heap.refCount++
}

func subtreeRelease(self subtree) {
	if self.isNil() {
		return
	}
	var stack []*subtreeData
	self.heap.refCount--
	if self.heap.refCount == 0 {
		stack = append(stack, self.heap)
	}
	for len(stack) > 0 {
		tree := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range tree.children {
			child.heap.refCount--
			if child.heap.refCount == 0 {
				stack = append(stack, child.heap)
			}
		}
	}
}

func subtreeClone(self subtree) subtree {
	result := new(subtreeData)
	*result = *self.heap
	if len(self.heap.children) > 0 {
		result.children = append([]subtree(nil), self.heap.children...)
		for _, child := range result.children {
			subtreeRetain(child)
		}
	} else if self.heap.hasExternalTokens {
		result.scannerState = self.heap.scannerState.copy()
	}
	result.refCount = 1
	return heapSubtree(result)
}

func subtreeMakeMut(self subtree) subtree {
	if self.heap.refCount == 1 {
		return self
	}
	result := subtreeClone(self)
	subtreeRelease(self)
	return result
}

func subtreeArrayCopy(self []subtree) []subtree {
	dest := append([]subtree(nil), self...)
	for _, t := range dest {
		subtreeRetain(t)
	}
	return dest
}

func subtreeArrayClear(self *[]subtree) {
	for _, t := range *self {
		subtreeRelease(t)
	}
	*self = (*self)[:0]
}

func subtreeArrayRemoveTrailingExtras(self *[]subtree) []subtree {
	var destination []subtree
	for len(*self) > 0 {
		last := (*self)[len(*self)-1]
		if subtreeExtra(last) {
			*self = (*self)[:len(*self)-1]
			destination = append(destination, last)
		} else {
			break
		}
	}
	subtreeArrayReverse(destination)
	return destination
}

func subtreeArrayReverse(self []subtree) {
	for i, limit := 0, len(self)/2; i < limit; i++ {
		j := len(self) - 1 - i
		self[i], self[j] = self[j], self[i]
	}
}

func newLeafSubtree(
	symbol Symbol, padding, size length, lookaheadBytes uint32, parseState StateID,
	hasExternalTokens, dependsOnColumn, isKeyword bool, language *Language,
) subtree {
	metadata := language.symbolMetadata(symbol)
	extra := symbol == BuiltinSymEnd
	return heapSubtree(&subtreeData{
		refCount:          1,
		padding:           padding,
		size:              size,
		lookaheadBytes:    lookaheadBytes,
		symbol:            symbol,
		parseState:        parseState,
		visible:           metadata.Visible,
		named:             metadata.Named,
		extra:             extra,
		hasExternalTokens: hasExternalTokens,
		dependsOnColumn:   dependsOnColumn,
		isKeyword:         isKeyword,
	})
}

func subtreeSetSymbol(self subtree, symbol Symbol, language *Language) {
	metadata := language.symbolMetadata(symbol)
	self.heap.symbol = symbol
	self.heap.named = metadata.Named
	self.heap.visible = metadata.Visible
}

func newErrorSubtree(
	lookaheadChar int32, padding, size length, bytesScanned uint32,
	parseState StateID, language *Language,
) subtree {
	result := newLeafSubtree(
		BuiltinSymError, padding, size, bytesScanned, parseState,
		false, false, false, language,
	)
	result.heap.fragileLeft = true
	result.heap.fragileRight = true
	result.heap.lookaheadChar = lookaheadChar
	return result
}

func subtreeErrorExtentCost(size length) uint32 {
	return errorCostPerRecovery +
		errorCostPerSkippedChar*size.Bytes +
		errorCostPerSkippedLine*size.Extent.Row
}

// subtreeSummarizeChildren runs only on a node that has children, which is
// always on the heap, so it takes that arm rather than the union.
func subtreeSummarizeChildren(self *subtreeData, language *Language) {
	self.namedChildCount = 0
	self.visibleChildCount = 0
	self.errorCost = 0
	self.repeatDepth = 0
	self.visibleDescendantCount = 0
	self.hasExternalTokens = false
	self.dependsOnColumn = false
	self.hasExternalScannerStateChange = false
	self.dynamicPrecedence = 0

	structuralIndex := uint32(0)
	aliasSequence := language.aliasSequence(uint32(self.productionID))
	lookaheadEndByte := uint32(0)

	children := self.children
	for i, child := range children {
		if self.size.Extent.Row == 0 && subtreeDependsOnColumn(child) {
			self.dependsOnColumn = true
		}
		if subtreeHasExternalScannerStateChange(child) {
			self.hasExternalScannerStateChange = true
		}
		if i == 0 {
			self.padding = subtreePadding(child)
			self.size = subtreeSize(child)
		} else {
			self.size = lengthAdd(self.size, subtreeTotalSize(child))
		}

		childLookaheadEndByte := self.padding.Bytes + self.size.Bytes + subtreeLookaheadBytes(child)
		if childLookaheadEndByte > lookaheadEndByte {
			lookaheadEndByte = childLookaheadEndByte
		}

		grandchildCount := subtreeChildCount(child)
		if subtreeSymbol(child) == builtinSymErrorRepeat {
			extentCost := subtreeErrorExtentCost(subtreeSize(child))
			self.errorCost += subtreeErrorCost(child) - extentCost
		} else {
			self.errorCost += subtreeErrorCost(child)
			if self.symbol == BuiltinSymError || self.symbol == builtinSymErrorRepeat {
				if !subtreeExtra(child) && !(subtreeIsError(child) && grandchildCount == 0) {
					if subtreeVisible(child) {
						self.errorCost += errorCostPerSkippedTree
					} else if grandchildCount > 0 {
						self.errorCost += errorCostPerSkippedTree * subtreeVisibleChildCount(child)
					}
				}
			}
		}

		self.dynamicPrecedence += subtreeDynamicPrecedence(child)
		self.visibleDescendantCount += subtreeVisibleDescendantCount(child)

		if !subtreeExtra(child) && subtreeSymbol(child) != 0 &&
			aliasSequence != nil && aliasSequence[structuralIndex] != 0 {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if language.symbolMetadata(aliasSequence[structuralIndex]).Named {
				self.namedChildCount++
			}
		} else if subtreeVisible(child) {
			self.visibleDescendantCount++
			self.visibleChildCount++
			if subtreeNamed(child) {
				self.namedChildCount++
			}
		} else if grandchildCount > 0 {
			self.visibleChildCount += subtreeVisibleChildCount(child)
			self.namedChildCount += subtreeNamedChildCount(child)
		}

		if subtreeHasExternalTokens(child) {
			self.hasExternalTokens = true
		}

		if subtreeIsError(child) {
			self.fragileLeft = true
			self.fragileRight = true
			self.parseState = treeStateNone
		}

		if !subtreeExtra(child) {
			structuralIndex++
		}
	}

	self.lookaheadBytes = lookaheadEndByte - self.size.Bytes - self.padding.Bytes

	if self.symbol == BuiltinSymError || self.symbol == builtinSymErrorRepeat {
		self.errorCost += subtreeErrorExtentCost(self.size)
	}

	if len(children) > 0 {
		firstChild := children[0]
		lastChild := children[len(children)-1]

		self.firstLeafSymbol = subtreeLeafSymbol(firstChild)
		self.firstLeafParseState = subtreeLeafParseState(firstChild)

		if subtreeFragileLeft(firstChild) {
			self.fragileLeft = true
		}
		if subtreeFragileRight(lastChild) {
			self.fragileRight = true
		}

		if len(children) >= 2 && !self.visible && !self.named &&
			subtreeSymbol(firstChild) == self.symbol {
			if subtreeRepeatDepth(firstChild) > subtreeRepeatDepth(lastChild) {
				self.repeatDepth = uint16(subtreeRepeatDepth(firstChild) + 1)
			} else {
				self.repeatDepth = uint16(subtreeRepeatDepth(lastChild) + 1)
			}
		}
	}
}

func newNodeSubtree(symbol Symbol, children []subtree, productionID uint16, language *Language) subtree {
	metadata := language.symbolMetadata(symbol)
	fragile := symbol == BuiltinSymError || symbol == builtinSymErrorRepeat
	data := &subtreeData{
		refCount:     1,
		symbol:       symbol,
		children:     children,
		visible:      metadata.Visible,
		named:        metadata.Named,
		fragileLeft:  fragile,
		fragileRight: fragile,
		productionID: productionID,
	}
	subtreeSummarizeChildren(data, language)
	return heapSubtree(data)
}

func newErrorNodeSubtree(children []subtree, extra bool, language *Language) subtree {
	result := newNodeSubtree(BuiltinSymError, children, 0, language)
	result.heap.extra = extra
	return result
}

func newMissingLeafSubtree(
	symbol Symbol, state StateID, padding length, lookaheadBytes uint32, language *Language,
) subtree {
	result := newLeafSubtree(
		symbol, padding, lengthZero(), lookaheadBytes, state,
		false, false, false, language,
	)
	result.heap.isMissing = true
	return result
}

// subtreeCompress walks a chain of nodes that all have children, so every link
// in it is on the heap and the scratch stack carries that arm.
func subtreeCompress(self *subtreeData, count uint32, language *Language, stack *[]*subtreeData) {
	initialStackSize := len(*stack)

	tree := self
	symbol := tree.symbol
	for i := uint32(0); i < count; i++ {
		if tree.refCount > 1 || len(tree.children) < 2 {
			break
		}
		child := tree.children[0].heap
		if len(child.children) < 2 || child.refCount > 1 || child.symbol != symbol {
			break
		}
		grandchild := child.children[0].heap
		if len(grandchild.children) < 2 || grandchild.refCount > 1 || grandchild.symbol != symbol {
			break
		}

		tree.children[0] = heapSubtree(grandchild)
		child.children[0] = grandchild.children[len(grandchild.children)-1]
		grandchild.children[len(grandchild.children)-1] = heapSubtree(child)
		*stack = append(*stack, tree)
		tree = grandchild
	}

	for len(*stack) > initialStackSize {
		tree = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		child := tree.children[0].heap
		grandchild := child.children[len(child.children)-1].heap
		subtreeSummarizeChildren(grandchild, language)
		subtreeSummarizeChildren(child, language)
		subtreeSummarizeChildren(tree, language)
	}
}

func subtreeCompare(left, right subtree) int {
	stack := []subtree{left, right}
	for len(stack) > 0 {
		right = stack[len(stack)-1]
		left = stack[len(stack)-2]
		stack = stack[:len(stack)-2]

		result := 0
		switch {
		case subtreeSymbol(left) < subtreeSymbol(right):
			result = -1
		case subtreeSymbol(right) < subtreeSymbol(left):
			result = 1
		case subtreeChildCount(left) < subtreeChildCount(right):
			result = -1
		case subtreeChildCount(right) < subtreeChildCount(left):
			result = 1
		}
		if result != 0 {
			return result
		}

		for i := subtreeChildCount(left); i > 0; i-- {
			stack = append(stack, subtreeChildren(left)[i-1], subtreeChildren(right)[i-1])
		}
	}
	return 0
}

func subtreeLastExternalToken(tree subtree) subtree {
	if tree.isNil() || !subtreeHasExternalTokens(tree) {
		return subtree{}
	}
	for len(subtreeChildren(tree)) > 0 {
		children := subtreeChildren(tree)
		for i := len(children) - 1; i >= 0; i-- {
			child := children[i]
			if subtreeHasExternalTokens(child) {
				tree = child
				break
			}
		}
	}
	return tree
}

var emptyScannerState externalScannerState

func subtreeExternalScannerState(self subtree) *externalScannerState {
	if !self.isNil() && self.heap.hasExternalTokens && len(self.heap.children) == 0 {
		return &self.heap.scannerState
	}
	return &emptyScannerState
}

func subtreeExternalScannerStateEq(self, other subtree) bool {
	return subtreeExternalScannerState(self).eq(subtreeExternalScannerState(other).data)
}
