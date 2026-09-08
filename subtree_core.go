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

type subtree = *subtreeData

func subtreeChildCount(self subtree) uint32 {
	if self == nil {
		return 0
	}
	return uint32(len(self.children))
}

func subtreeSymbol(self subtree) Symbol         { return self.symbol }
func subtreeVisible(self subtree) bool          { return self.visible }
func subtreeNamed(self subtree) bool            { return self.named }
func subtreeExtra(self subtree) bool            { return self.extra }
func subtreeHasChanges(self subtree) bool       { return self.hasChanges }
func subtreeMissing(self subtree) bool          { return self.isMissing }
func subtreeIsKeyword(self subtree) bool        { return self.isKeyword }
func subtreeParseState(self subtree) StateID    { return self.parseState }
func subtreeLookaheadBytes(self subtree) uint32 { return self.lookaheadBytes }

func subtreeLeafSymbol(self subtree) Symbol {
	if len(self.children) == 0 {
		return self.symbol
	}
	return self.firstLeafSymbol
}

func subtreeLeafParseState(self subtree) StateID {
	if len(self.children) == 0 {
		return self.parseState
	}
	return self.firstLeafParseState
}

func subtreePadding(self subtree) length   { return self.padding }
func subtreeSize(self subtree) length      { return self.size }
func subtreeTotalSize(self subtree) length { return lengthAdd(self.padding, self.size) }
func subtreeTotalBytes(self subtree) uint32 {
	return subtreeTotalSize(self).Bytes
}

func subtreeRepeatDepth(self subtree) uint32 { return uint32(self.repeatDepth) }

func subtreeVisibleDescendantCount(self subtree) uint32 {
	if len(self.children) == 0 {
		return 0
	}
	return self.visibleDescendantCount
}

func subtreeErrorCost(self subtree) uint32 {
	if self.isMissing {
		return errorCostPerMissingTree + errorCostPerRecovery
	}
	return self.errorCost
}

func subtreeDynamicPrecedence(self subtree) int32 {
	if len(self.children) == 0 {
		return 0
	}
	return self.dynamicPrecedence
}

func subtreeFragileLeft(self subtree) bool  { return self.fragileLeft }
func subtreeFragileRight(self subtree) bool { return self.fragileRight }
func subtreeHasExternalTokens(self subtree) bool {
	return self.hasExternalTokens
}

func subtreeHasExternalScannerStateChange(self subtree) bool {
	return self.hasExternalScannerStateChange
}

func subtreeDependsOnColumn(self subtree) bool { return self.dependsOnColumn }

func subtreeIsFragile(self subtree) bool {
	return self.fragileLeft || self.fragileRight
}

func subtreeIsError(self subtree) bool { return self.symbol == BuiltinSymError }
func subtreeIsEOF(self subtree) bool   { return self.symbol == BuiltinSymEnd }

func subtreeRetain(self subtree) {
	if self == nil {
		return
	}
	self.refCount++
}

func subtreeRelease(self subtree) {
	if self == nil {
		return
	}
	var stack []subtree
	self.refCount--
	if self.refCount == 0 {
		stack = append(stack, self)
	}
	for len(stack) > 0 {
		tree := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, child := range tree.children {
			child.refCount--
			if child.refCount == 0 {
				stack = append(stack, child)
			}
		}
	}
}

func subtreeClone(self subtree) subtree {
	result := new(subtreeData)
	*result = *self
	if len(self.children) > 0 {
		result.children = append([]subtree(nil), self.children...)
		for _, child := range result.children {
			subtreeRetain(child)
		}
	} else if self.hasExternalTokens {
		result.scannerState = self.scannerState.copy()
	}
	result.refCount = 1
	return result
}

func subtreeMakeMut(self subtree) subtree {
	if self.refCount == 1 {
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
	return &subtreeData{
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
	}
}

func subtreeSetSymbol(self subtree, symbol Symbol, language *Language) {
	metadata := language.symbolMetadata(symbol)
	self.symbol = symbol
	self.named = metadata.Named
	self.visible = metadata.Visible
}

func newErrorSubtree(
	lookaheadChar int32, padding, size length, bytesScanned uint32,
	parseState StateID, language *Language,
) subtree {
	result := newLeafSubtree(
		BuiltinSymError, padding, size, bytesScanned, parseState,
		false, false, false, language,
	)
	result.fragileLeft = true
	result.fragileRight = true
	result.lookaheadChar = lookaheadChar
	return result
}

func subtreeErrorExtentCost(size length) uint32 {
	return errorCostPerRecovery +
		errorCostPerSkippedChar*size.Bytes +
		errorCostPerSkippedLine*size.Extent.Row
}

func subtreeSummarizeChildren(self subtree, language *Language) {
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
						self.errorCost += errorCostPerSkippedTree * child.visibleChildCount
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
			self.visibleChildCount += child.visibleChildCount
			self.namedChildCount += child.namedChildCount
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
	return data
}

func newErrorNodeSubtree(children []subtree, extra bool, language *Language) subtree {
	result := newNodeSubtree(BuiltinSymError, children, 0, language)
	result.extra = extra
	return result
}

func newMissingLeafSubtree(
	symbol Symbol, state StateID, padding length, lookaheadBytes uint32, language *Language,
) subtree {
	result := newLeafSubtree(
		symbol, padding, lengthZero(), lookaheadBytes, state,
		false, false, false, language,
	)
	result.isMissing = true
	return result
}

func subtreeCompress(self subtree, count uint32, language *Language, stack *[]subtree) {
	initialStackSize := len(*stack)

	tree := self
	symbol := tree.symbol
	for i := uint32(0); i < count; i++ {
		if tree.refCount > 1 || len(tree.children) < 2 {
			break
		}
		child := tree.children[0]
		if len(child.children) < 2 || child.refCount > 1 || child.symbol != symbol {
			break
		}
		grandchild := child.children[0]
		if len(grandchild.children) < 2 || grandchild.refCount > 1 || grandchild.symbol != symbol {
			break
		}

		tree.children[0] = grandchild
		child.children[0] = grandchild.children[len(grandchild.children)-1]
		grandchild.children[len(grandchild.children)-1] = child
		*stack = append(*stack, tree)
		tree = grandchild
	}

	for len(*stack) > initialStackSize {
		tree = (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		child := tree.children[0]
		grandchild := child.children[len(child.children)-1]
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
			stack = append(stack, left.children[i-1], right.children[i-1])
		}
	}
	return 0
}

func subtreeLastExternalToken(tree subtree) subtree {
	if tree == nil || !subtreeHasExternalTokens(tree) {
		return nil
	}
	for len(tree.children) > 0 {
		for i := len(tree.children) - 1; i >= 0; i-- {
			child := tree.children[i]
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
	if self != nil && self.hasExternalTokens && len(self.children) == 0 {
		return &self.scannerState
	}
	return &emptyScannerState
}

func subtreeExternalScannerStateEq(self, other subtree) bool {
	return subtreeExternalScannerState(self).eq(subtreeExternalScannerState(other).data)
}
