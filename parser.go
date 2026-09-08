package treesitter

const (
	maxVersionCount         = 6
	maxVersionCountOverflow = 4
	maxSummaryDepth         = 16
	maxCostDifference       = 18 * errorCostPerSkippedTree
	serializationBufferSize = 1024
)

type tokenCache struct {
	token             subtree
	lastExternalToken subtree
	byteIndex         uint32
}

type errorStatus struct {
	cost              uint32
	nodeCount         uint32
	dynamicPrecedence int32
	isInError         bool
}

type errorComparison int

const (
	errorComparisonTakeLeft errorComparison = iota
	errorComparisonPreferLeft
	errorComparisonNone
	errorComparisonPreferRight
	errorComparisonTakeRight
)

type reduceAction struct {
	count             uint32
	symbol            Symbol
	dynamicPrecedence int32
	productionID      uint16
}

func reduceActionSetAdd(self *[]reduceAction, newAction reduceAction) {
	for _, action := range *self {
		if action.symbol == newAction.symbol && action.count == newAction.count {
			return
		}
	}
	*self = append(*self, newAction)
}

// Parser turns source text into a syntax tree.
type Parser struct {
	lexer                     Lexer
	stack                     *parseStack
	language                  *Language
	reduceActions             []reduceAction
	finishedTree              subtree
	trailingExtras            []subtree
	trailingExtras2           []subtree
	scratchTrees              []subtree
	tokenCache                tokenCache
	reusableNode              reusableNode
	externalScannerPayload    any
	acceptCount               uint32
	oldTree                   subtree
	includedRangeDifferences  []Range
	includedRangeDiffIndex    uint32
	hasScannerError           bool
	hasError                  bool
	serializationBuffer       [serializationBufferSize]byte
	externalScannerStateBytes []byte
}

// NewParser creates a parser with no language assigned.
func NewParser() *Parser {
	self := &Parser{}
	self.lexer.init()
	self.stack = newParseStack()
	self.setCachedToken(0, nil, nil)
	return self
}

// SetLanguage assigns the grammar this parser uses.
func (p *Parser) SetLanguage(language *Language) bool {
	p.Reset()
	p.language = nil
	if language != nil {
		if language.ABIVersion > 15 || language.ABIVersion < 13 {
			return false
		}
		if language.LexFn == nil {
			return false
		}
	}
	p.language = language
	return true
}

// Language reports the grammar this parser uses.
func (p *Parser) Language() *Language { return p.language }

// SetIncludedRanges restricts parsing to the given regions.
func (p *Parser) SetIncludedRanges(ranges []Range) bool {
	return p.lexer.setIncludedRanges(ranges)
}

// IncludedRanges reports the regions the parser reads.
func (p *Parser) IncludedRanges() []Range { return p.lexer.includedRanges }

// Reset clears any parse state left over from a previous call.
func (p *Parser) Reset() {
	p.externalScannerDestroy()
	if p.oldTree != nil {
		subtreeRelease(p.oldTree)
		p.oldTree = nil
	}
	p.reusableNode.clear()
	p.lexer.reset(lengthZero())
	p.stack.clear()
	p.setCachedToken(0, nil, nil)
	if p.finishedTree != nil {
		subtreeRelease(p.finishedTree)
		p.finishedTree = nil
	}
	p.acceptCount = 0
	p.hasScannerError = false
	p.hasError = false
}

func (p *Parser) breakdownTopOfStack(version stackVersion) bool {
	didBreakDown := false
	pending := false

	for {
		pop := append([]stackSlice(nil), p.stack.popPending(version)...)
		if len(pop) == 0 {
			break
		}

		didBreakDown = true
		pending = false
		for _, slice := range pop {
			state := p.stack.state(slice.version)
			parent := slice.subtrees[0]

			for j := uint32(0); j < subtreeChildCount(parent); j++ {
				child := parent.children[j]
				pending = subtreeChildCount(child) > 0

				if subtreeIsError(child) {
					state = errorState
				} else if !subtreeExtra(child) {
					state = p.language.nextState(state, subtreeSymbol(child))
				}

				subtreeRetain(child)
				p.stack.push(slice.version, child, pending, state)
			}

			for j := 1; j < len(slice.subtrees); j++ {
				p.stack.push(slice.version, slice.subtrees[j], false, state)
			}

			subtreeRelease(parent)
		}

		if !pending {
			break
		}
	}

	return didBreakDown
}

func (p *Parser) breakdownLookahead(lookahead *subtree, state StateID, node *reusableNode) {
	didDescend := false
	tree := node.tree()
	for subtreeChildCount(tree) > 0 && subtreeParseState(tree) != state {
		node.descend()
		tree = node.tree()
		didDescend = true
	}

	if didDescend {
		subtreeRelease(*lookahead)
		*lookahead = tree
		subtreeRetain(*lookahead)
	}
}

func compareVersions(a, b errorStatus) errorComparison {
	if !a.isInError && b.isInError {
		if a.cost < b.cost {
			return errorComparisonTakeLeft
		}
		return errorComparisonPreferLeft
	}

	if a.isInError && !b.isInError {
		if b.cost < a.cost {
			return errorComparisonTakeRight
		}
		return errorComparisonPreferRight
	}

	if a.cost < b.cost {
		if (b.cost-a.cost)*(1+a.nodeCount) > maxCostDifference {
			return errorComparisonTakeLeft
		}
		return errorComparisonPreferLeft
	}

	if b.cost < a.cost {
		if (a.cost-b.cost)*(1+b.nodeCount) > maxCostDifference {
			return errorComparisonTakeRight
		}
		return errorComparisonPreferRight
	}

	if a.dynamicPrecedence > b.dynamicPrecedence {
		return errorComparisonPreferLeft
	}
	if b.dynamicPrecedence > a.dynamicPrecedence {
		return errorComparisonPreferRight
	}
	return errorComparisonNone
}

func (p *Parser) versionStatus(version stackVersion) errorStatus {
	cost := p.stack.errorCost(version)
	isPaused := p.stack.isPaused(version)
	if isPaused {
		cost += errorCostPerSkippedTree
	}
	return errorStatus{
		cost:              cost,
		nodeCount:         p.stack.nodeCountSinceError(version),
		dynamicPrecedence: p.stack.dynamicPrecedence(version),
		isInError:         isPaused || p.stack.state(version) == errorState,
	}
}

func (p *Parser) betterVersionExists(version stackVersion, isInError bool, cost uint32) bool {
	if p.finishedTree != nil && subtreeErrorCost(p.finishedTree) <= cost {
		return true
	}

	position := p.stack.position(version)
	status := errorStatus{
		cost:              cost,
		isInError:         isInError,
		dynamicPrecedence: p.stack.dynamicPrecedence(version),
		nodeCount:         p.stack.nodeCountSinceError(version),
	}

	for i, n := stackVersion(0), p.stack.versionCount(); i < n; i++ {
		if i == version || !p.stack.isActive(i) ||
			p.stack.position(i).Bytes < position.Bytes {
			continue
		}
		statusI := p.versionStatus(i)
		switch compareVersions(status, statusI) {
		case errorComparisonTakeRight:
			return true
		case errorComparisonPreferRight:
			if p.stack.canMerge(i, version) {
				return true
			}
		}
	}

	return false
}

func (p *Parser) externalScannerCreate() {
	if p.language != nil && p.language.Scanner != nil {
		p.externalScannerPayload = p.language.Scanner.Create()
	}
}

func (p *Parser) externalScannerDestroy() {
	if p.language != nil && p.externalScannerPayload != nil && p.language.Scanner != nil {
		p.language.Scanner.Destroy(p.externalScannerPayload)
	}
	p.externalScannerPayload = nil
}

func (p *Parser) externalScannerSerialize() uint32 {
	length := p.language.Scanner.Serialize(p.externalScannerPayload, p.serializationBuffer[:])
	if length > serializationBufferSize {
		length = serializationBufferSize
	}
	return length
}

func (p *Parser) externalScannerDeserialize(externalToken subtree) {
	var data []byte
	if externalToken != nil {
		data = subtreeExternalScannerState(externalToken).data
	}
	p.language.Scanner.Deserialize(p.externalScannerPayload, data)
}

func (p *Parser) externalScannerScan(externalLexState uint32) bool {
	validExternalTokens := p.language.enabledExternalTokens(externalLexState)
	return p.language.Scanner.Scan(p.externalScannerPayload, &p.lexer, validExternalTokens)
}

func (p *Parser) canReuseFirstLeaf(state StateID, tree subtree, entry *tableEntry) bool {
	leafSymbol := subtreeLeafSymbol(tree)
	leafState := subtreeLeafParseState(tree)
	currentLexMode := p.language.lexModeForState(state)
	leafLexMode := p.language.lexModeForState(leafState)

	if currentLexMode.LexState == 0xFFFF {
		return false
	}

	if entry.actionCount > 0 && leafLexMode == currentLexMode &&
		(leafSymbol != p.language.KeywordCaptureToken ||
			(!subtreeIsKeyword(tree) && subtreeParseState(tree) == state)) {
		return true
	}

	if subtreeSize(tree).Bytes == 0 && leafSymbol != BuiltinSymEnd {
		return false
	}

	return currentLexMode.ExternalLexState == 0 && entry.isReusable
}

func (p *Parser) lex(version stackVersion, parseState StateID) subtree {
	lexMode := p.language.lexModeForState(parseState)
	if lexMode.LexState == 0xFFFF {
		return nil
	}

	startPosition := p.stack.position(version)
	externalToken := p.stack.lastExternalToken(version)

	foundExternalToken := false
	errorMode := parseState == errorState
	skippedError := false
	calledGetColumn := false
	var firstErrorCharacter int32
	errorStartPosition := lengthZero()
	errorEndPosition := lengthZero()
	lookaheadEndByte := uint32(0)
	externalScannerStateLen := uint32(0)
	externalScannerStateChanged := false
	p.lexer.reset(startPosition)

	for {
		foundToken := false
		currentPosition := p.lexer.currentPosition
		savedColumnData := p.lexer.columnData

		if lexMode.ExternalLexState != 0 {
			p.lexer.start()
			p.externalScannerDeserialize(externalToken)
			foundToken = p.externalScannerScan(uint32(lexMode.ExternalLexState))
			if p.hasScannerError {
				return nil
			}
			p.lexer.finish(&lookaheadEndByte)

			if foundToken {
				externalScannerStateLen = p.externalScannerSerialize()
				externalScannerStateChanged = !subtreeExternalScannerState(externalToken).
					eq(p.serializationBuffer[:externalScannerStateLen])

				if p.lexer.tokenEndPosition.Bytes <= currentPosition.Bytes &&
					!externalScannerStateChanged {
					symbol := p.language.ExternalScannerSymbol[p.lexer.ResultSymbol]
					nextParseState := p.language.nextState(parseState, symbol)
					tokenIsExtra := nextParseState == parseState
					if errorMode || !p.stack.hasAdvancedSinceError(version) || tokenIsExtra {
						foundToken = false
					}
				}
			}

			if foundToken {
				foundExternalToken = true
				calledGetColumn = p.lexer.didGetColumn
				break
			}

			p.lexer.reset(currentPosition)
			p.lexer.columnData = savedColumnData
		}

		p.lexer.start()
		foundToken = p.language.LexFn(&p.lexer, lexMode.LexState)
		p.lexer.finish(&lookaheadEndByte)
		if foundToken {
			break
		}

		if !errorMode {
			errorMode = true
			lexMode = p.language.lexModeForState(errorState)
			p.lexer.reset(startPosition)
			continue
		}

		if !skippedError {
			skippedError = true
			errorStartPosition = p.lexer.tokenStartPosition
			errorEndPosition = p.lexer.tokenStartPosition
			firstErrorCharacter = p.lexer.Lookahead
		}

		if p.lexer.currentPosition.Bytes == errorEndPosition.Bytes {
			if p.lexer.EOF() {
				p.lexer.ResultSymbol = BuiltinSymError
				break
			}
			p.lexer.Advance(false)
		}

		errorEndPosition = p.lexer.currentPosition
	}

	var result subtree
	if skippedError {
		padding := lengthSub(errorStartPosition, startPosition)
		size := lengthSub(errorEndPosition, errorStartPosition)
		lookaheadBytes := lookaheadEndByte - errorEndPosition.Bytes
		result = newErrorSubtree(
			firstErrorCharacter, padding, size, lookaheadBytes, parseState, p.language,
		)
	} else {
		isKeyword := false
		symbol := p.lexer.ResultSymbol
		padding := lengthSub(p.lexer.tokenStartPosition, startPosition)
		size := lengthSub(p.lexer.tokenEndPosition, p.lexer.tokenStartPosition)
		lookaheadBytes := lookaheadEndByte - p.lexer.tokenEndPosition.Bytes

		if foundExternalToken {
			symbol = p.language.ExternalScannerSymbol[symbol]
		} else if symbol == p.language.KeywordCaptureToken && symbol != 0 {
			endByte := p.lexer.tokenEndPosition.Bytes
			p.lexer.reset(p.lexer.tokenStartPosition)
			p.lexer.start()

			isKeyword = p.language.KeywordLexFn(&p.lexer, 0)

			if isKeyword && p.lexer.tokenEndPosition.Bytes == endByte &&
				(p.language.hasActions(parseState, p.lexer.ResultSymbol) ||
					p.language.isReservedWord(parseState, p.lexer.ResultSymbol)) {
				symbol = p.lexer.ResultSymbol
			}
		}

		result = newLeafSubtree(
			symbol, padding, size, lookaheadBytes, parseState,
			foundExternalToken, calledGetColumn, isKeyword, p.language,
		)

		if foundExternalToken {
			result.scannerState.init(p.serializationBuffer[:externalScannerStateLen])
			result.hasExternalScannerStateChange = externalScannerStateChanged
		}
	}

	return result
}

func (p *Parser) getCachedToken(
	state StateID, position uint32, lastExternalToken subtree, entry *tableEntry,
) subtree {
	cache := &p.tokenCache
	if cache.token != nil && cache.byteIndex == position &&
		subtreeExternalScannerStateEq(cache.lastExternalToken, lastExternalToken) {
		p.language.tableEntry(state, subtreeSymbol(cache.token), entry)
		if p.canReuseFirstLeaf(state, cache.token, entry) {
			subtreeRetain(cache.token)
			return cache.token
		}
	}
	return nil
}

func (p *Parser) setCachedToken(byteIndex uint32, lastExternalToken, token subtree) {
	cache := &p.tokenCache
	if token != nil {
		subtreeRetain(token)
	}
	if lastExternalToken != nil {
		subtreeRetain(lastExternalToken)
	}
	if cache.token != nil {
		subtreeRelease(cache.token)
	}
	if cache.lastExternalToken != nil {
		subtreeRelease(cache.lastExternalToken)
	}
	cache.token = token
	cache.byteIndex = byteIndex
	cache.lastExternalToken = lastExternalToken
}

func (p *Parser) hasIncludedRangeDifference(startPosition, endPosition uint32) bool {
	return rangeArrayIntersects(
		p.includedRangeDifferences, p.includedRangeDiffIndex, startPosition, endPosition,
	)
}

func (p *Parser) reuseNode(
	version stackVersion, state *StateID, position uint32,
	lastExternalToken subtree, entry *tableEntry,
) subtree {
	for {
		result := p.reusableNode.tree()
		if result == nil {
			break
		}
		byteOffset := p.reusableNode.byteOffset()
		endByteOffset := byteOffset + subtreeTotalBytes(result)

		if subtreeIsEOF(result) {
			endByteOffset = ^uint32(0)
		}

		if byteOffset > position {
			break
		}

		if byteOffset < position {
			if endByteOffset <= position || !p.reusableNode.descend() {
				p.reusableNode.advance()
			}
			continue
		}

		if !subtreeExternalScannerStateEq(p.reusableNode.lastExternalToken, lastExternalToken) {
			p.reusableNode.advance()
			continue
		}

		blocked := subtreeHasChanges(result) || subtreeIsError(result) ||
			subtreeMissing(result) || subtreeIsFragile(result)
		if !blocked {
			end := endByteOffset
			if !subtreeIsEOF(result) {
				end = endByteOffset + subtreeLookaheadBytes(result)
			}
			blocked = p.hasIncludedRangeDifference(byteOffset, end)
		}

		if blocked {
			if !p.reusableNode.descend() {
				p.reusableNode.advance()
				p.breakdownTopOfStack(version)
				*state = p.stack.state(version)
			}
			continue
		}

		leafSymbol := subtreeLeafSymbol(result)
		p.language.tableEntry(*state, leafSymbol, entry)
		if !p.canReuseFirstLeaf(*state, result, entry) {
			p.reusableNode.advancePastLeaf()
			break
		}

		subtreeRetain(result)
		return result
	}

	return nil
}

func (p *Parser) selectTree(left, right subtree) bool {
	if left == nil {
		return true
	}
	if right == nil {
		return false
	}

	if subtreeErrorCost(right) < subtreeErrorCost(left) {
		return true
	}
	if subtreeErrorCost(left) < subtreeErrorCost(right) {
		return false
	}
	if subtreeDynamicPrecedence(right) > subtreeDynamicPrecedence(left) {
		return true
	}
	if subtreeDynamicPrecedence(left) > subtreeDynamicPrecedence(right) {
		return false
	}
	if subtreeErrorCost(left) > 0 {
		return true
	}

	switch subtreeCompare(left, right) {
	case -1:
		return false
	case 1:
		return true
	default:
		return false
	}
}

func (p *Parser) selectChildren(left subtree, children []subtree) bool {
	p.scratchTrees = append(p.scratchTrees[:0], children...)
	scratchTree := newNodeSubtree(subtreeSymbol(left), p.scratchTrees, 0, p.language)
	return p.selectTree(left, scratchTree)
}

func (p *Parser) shift(version stackVersion, state StateID, lookahead subtree, extra bool) {
	isLeaf := subtreeChildCount(lookahead) == 0
	subtreeToPush := lookahead
	if extra != subtreeExtra(lookahead) && isLeaf {
		result := subtreeMakeMut(lookahead)
		result.extra = extra
		subtreeToPush = result
	}

	p.stack.push(version, subtreeToPush, !isLeaf, state)
	if subtreeHasExternalTokens(subtreeToPush) {
		p.stack.setLastExternalToken(version, subtreeLastExternalToken(subtreeToPush))
	}
}

func (p *Parser) reduce(
	version stackVersion, symbol Symbol, count uint32, dynamicPrecedence int32,
	productionID uint16, isFragile, endOfNonTerminalExtra bool,
) stackVersion {
	initialVersionCount := p.stack.versionCount()

	pop := append([]stackSlice(nil), p.stack.popCount(version, count)...)
	removedVersionCount := uint32(0)
	haltedVersionCount := p.stack.haltedVersionCount()
	for i := 0; i < len(pop); i++ {
		slice := pop[i]
		sliceVersion := slice.version - removedVersionCount

		if sliceVersion > maxVersionCount+maxVersionCountOverflow+haltedVersionCount {
			p.stack.removeVersion(sliceVersion)
			subtreeArrayClear(&slice.subtrees)
			removedVersionCount++
			for i+1 < len(pop) {
				nextSlice := pop[i+1]
				if nextSlice.version != slice.version {
					break
				}
				subtreeArrayClear(&nextSlice.subtrees)
				i++
			}
			continue
		}

		children := slice.subtrees
		p.trailingExtras = subtreeArrayRemoveTrailingExtras(&children)

		parent := newNodeSubtree(symbol, children, productionID, p.language)

		for i+1 < len(pop) {
			nextSlice := pop[i+1]
			if nextSlice.version != slice.version {
				break
			}
			i++

			nextSliceChildren := nextSlice.subtrees
			p.trailingExtras2 = subtreeArrayRemoveTrailingExtras(&nextSliceChildren)

			if p.selectChildren(parent, nextSliceChildren) {
				subtreeArrayClear(&p.trailingExtras)
				subtreeRelease(parent)
				p.trailingExtras, p.trailingExtras2 = p.trailingExtras2, p.trailingExtras
				parent = newNodeSubtree(symbol, nextSliceChildren, productionID, p.language)
			} else {
				p.trailingExtras2 = p.trailingExtras2[:0]
				subtrees := nextSlice.subtrees
				subtreeArrayClear(&subtrees)
			}
		}

		state := p.stack.state(sliceVersion)
		nextState := p.language.nextState(state, symbol)
		if endOfNonTerminalExtra && nextState == state {
			parent.extra = true
		}
		if isFragile || len(pop) > 1 || initialVersionCount > 1 {
			parent.fragileLeft = true
			parent.fragileRight = true
			parent.parseState = treeStateNone
		} else {
			parent.parseState = state
		}
		parent.dynamicPrecedence += dynamicPrecedence

		p.stack.push(sliceVersion, parent, false, nextState)
		for _, extra := range p.trailingExtras {
			p.stack.push(sliceVersion, extra, false, nextState)
		}

		for j := stackVersion(0); j < sliceVersion; j++ {
			if j == version {
				continue
			}
			if p.stack.merge(j, sliceVersion) {
				removedVersionCount++
				break
			}
		}
	}

	if p.stack.versionCount() > initialVersionCount {
		return initialVersionCount
	}
	return stackVersionNone
}

func (p *Parser) accept(version stackVersion, lookahead subtree) {
	p.stack.push(version, lookahead, false, 1)

	pop := append([]stackSlice(nil), p.stack.popAll(version)...)
	for i := range pop {
		trees := pop[i].subtrees

		var root subtree
		for j := len(trees) - 1; j >= 0; j-- {
			tree := trees[j]
			if !subtreeExtra(tree) {
				childCount := subtreeChildCount(tree)
				children := tree.children
				for k := uint32(0); k < childCount; k++ {
					subtreeRetain(children[k])
				}
				spliced := make([]subtree, 0, len(trees)-1+int(childCount))
				spliced = append(spliced, trees[:j]...)
				spliced = append(spliced, children...)
				spliced = append(spliced, trees[j+1:]...)
				root = newNodeSubtree(
					subtreeSymbol(tree), spliced, tree.productionID, p.language,
				)
				subtreeRelease(tree)
				break
			}
		}

		p.acceptCount++

		if p.finishedTree != nil {
			if p.selectTree(p.finishedTree, root) {
				subtreeRelease(p.finishedTree)
				p.finishedTree = root
			} else {
				subtreeRelease(root)
			}
		} else {
			p.finishedTree = root
		}
	}

	p.stack.removeVersion(pop[0].version)
	p.stack.halt(version)
}

func (p *Parser) processCandidateRecoveryActions(actions []ParseActionEntry) bool {
	hasShiftAction := false
	for i := range actions {
		action := actions[i].Action
		switch action.Type {
		case ParseActionTypeShift, ParseActionTypeRecover:
			if !action.Extra && !action.Repetition {
				hasShiftAction = true
			}
		case ParseActionTypeReduce:
			if action.ChildCount > 0 {
				reduceActionSetAdd(&p.reduceActions, reduceAction{
					symbol:            action.Symbol,
					count:             uint32(action.ChildCount),
					dynamicPrecedence: int32(action.DynamicPrecedence),
					productionID:      action.ProductionID,
				})
			}
		}
	}
	return hasShiftAction
}

func (p *Parser) doAllPotentialReductions(
	startingVersion stackVersion, lookaheadSymbol Symbol,
) bool {
	initialVersionCount := p.stack.versionCount()

	canShiftLookaheadSymbol := false
	version := startingVersion
	for i := uint32(0); ; i++ {
		versionCount := p.stack.versionCount()
		if version >= versionCount {
			break
		}

		merged := false
		for j := initialVersionCount; j < version; j++ {
			if p.stack.merge(j, version) {
				merged = true
				break
			}
		}
		if merged {
			continue
		}

		state := p.stack.state(version)
		hasShiftAction := false
		p.reduceActions = p.reduceActions[:0]

		if lookaheadSymbol != 0 {
			var entry tableEntry
			p.language.tableEntry(state, lookaheadSymbol, &entry)
			hasShiftAction = p.processCandidateRecoveryActions(entry.actions)
		} else {
			iter := p.language.lookaheads(state)
			for iter.next() {
				if iter.symbol == BuiltinSymEnd || uint32(iter.symbol) >= p.language.TokenCount {
					continue
				}
				if p.processCandidateRecoveryActions(iter.actions) {
					hasShiftAction = true
				}
			}

			for j := 1; j < len(p.reduceActions); j++ {
				key := p.reduceActions[j]
				k := j - 1
				for k >= 0 && p.reduceActions[k].symbol < key.symbol {
					p.reduceActions[k+1] = p.reduceActions[k]
					k--
				}
				p.reduceActions[k+1] = key
			}
		}

		reductionVersion := stackVersionNone
		for _, action := range p.reduceActions {
			reductionVersion = p.reduce(
				version, action.symbol, action.count,
				action.dynamicPrecedence, action.productionID, true, false,
			)
		}

		if hasShiftAction {
			canShiftLookaheadSymbol = true
		} else if reductionVersion != stackVersionNone && i < maxVersionCount {
			p.stack.renumberVersion(reductionVersion, version)
			continue
		} else if lookaheadSymbol != 0 {
			p.stack.removeVersion(version)
		}

		if version == startingVersion {
			version = versionCount
		} else {
			version++
		}
	}

	return canShiftLookaheadSymbol
}

func (p *Parser) recoverToState(version stackVersion, depth uint32, goalState StateID) bool {
	pop := append([]stackSlice(nil), p.stack.popCount(version, depth)...)
	previousVersion := stackVersionNone

	for i := 0; i < len(pop); i++ {
		slice := pop[i]

		if slice.version == previousVersion {
			subtreeArrayClear(&slice.subtrees)
			pop = append(pop[:i], pop[i+1:]...)
			i--
			continue
		}

		if p.stack.state(slice.version) != goalState {
			p.stack.halt(slice.version)
			subtreeArrayClear(&slice.subtrees)
			pop = append(pop[:i], pop[i+1:]...)
			i--
			continue
		}

		errorTrees := p.stack.popError(slice.version)
		if len(errorTrees) > 0 {
			errorTree := errorTrees[0]
			errorChildCount := subtreeChildCount(errorTree)
			if errorChildCount > 0 {
				nested := make([]subtree, 0, errorChildCount)
				for j := uint32(0); j < errorChildCount; j++ {
					child := errorTree.children[j]
					subtreeRetain(child)
					nested = append(nested, child)
				}
				nestedError := newNodeSubtree(builtinSymErrorRepeat, nested, 0, p.language)
				slice.subtrees = append([]subtree{nestedError}, slice.subtrees...)
			}
			subtreeArrayClear(&errorTrees)
		}

		p.trailingExtras = subtreeArrayRemoveTrailingExtras(&slice.subtrees)

		if len(slice.subtrees) > 0 {
			errNode := newErrorNodeSubtree(slice.subtrees, true, p.language)
			p.stack.push(slice.version, errNode, false, goalState)
		}

		for _, tree := range p.trailingExtras {
			p.stack.push(slice.version, tree, false, goalState)
		}

		previousVersion = slice.version
	}

	return previousVersion != stackVersionNone
}

func (p *Parser) recover(version stackVersion, lookahead subtree) {
	didRecover := false
	previousVersionCount := p.stack.versionCount()
	position := p.stack.position(version)
	summary := p.stack.getSummary(version)
	nodeCountSinceError := p.stack.nodeCountSinceError(version)
	currentErrorCost := p.stack.errorCost(version)

	if summary != nil && !subtreeIsError(lookahead) {
		for _, entry := range *summary {
			if entry.state == errorState {
				continue
			}
			if entry.position.Bytes == position.Bytes {
				continue
			}
			depth := entry.depth
			if nodeCountSinceError > 0 {
				depth++
			}

			wouldMerge := false
			for j := stackVersion(0); j < previousVersionCount; j++ {
				if p.stack.state(j) == entry.state &&
					p.stack.position(j).Bytes == position.Bytes {
					wouldMerge = true
					break
				}
			}
			if wouldMerge {
				continue
			}

			newCost := currentErrorCost +
				entry.depth*errorCostPerSkippedTree +
				(position.Bytes-entry.position.Bytes)*errorCostPerSkippedChar +
				(position.Extent.Row-entry.position.Extent.Row)*errorCostPerSkippedLine
			if p.betterVersionExists(version, false, newCost) {
				break
			}

			if p.language.hasActions(entry.state, subtreeSymbol(lookahead)) {
				if p.recoverToState(version, depth, entry.state) {
					didRecover = true
					break
				}
			}
		}
	}

	for i := previousVersionCount; i < p.stack.versionCount(); i++ {
		if !p.stack.isActive(i) {
			p.stack.removeVersion(i)
			i--
		}
	}

	if subtreeIsEOF(lookahead) {
		parent := newErrorNodeSubtree(nil, false, p.language)
		p.stack.push(version, parent, false, 1)
		p.accept(version, lookahead)
		return
	}

	if didRecover && p.stack.versionCount() > maxVersionCount {
		p.stack.halt(version)
		subtreeRelease(lookahead)
		return
	}

	if didRecover && subtreeHasExternalScannerStateChange(lookahead) {
		p.stack.halt(version)
		subtreeRelease(lookahead)
		return
	}

	newCost := currentErrorCost + errorCostPerSkippedTree +
		subtreeTotalBytes(lookahead)*errorCostPerSkippedChar +
		subtreeTotalSize(lookahead).Extent.Row*errorCostPerSkippedLine
	if p.betterVersionExists(version, false, newCost) {
		p.stack.halt(version)
		subtreeRelease(lookahead)
		return
	}

	actions := p.language.actions(1, subtreeSymbol(lookahead))
	if n := len(actions); n > 0 && actions[n-1].Action.Type == ParseActionTypeShift &&
		actions[n-1].Action.Extra {
		mutableLookahead := subtreeMakeMut(lookahead)
		mutableLookahead.extra = true
		lookahead = mutableLookahead
	}

	children := []subtree{lookahead}
	errorRepeat := newNodeSubtree(builtinSymErrorRepeat, children, 0, p.language)

	if nodeCountSinceError > 0 {
		pop := append([]stackSlice(nil), p.stack.popCount(version, 1)...)

		if len(pop) > 1 {
			for i := 1; i < len(pop); i++ {
				subtreeArrayClear(&pop[i].subtrees)
			}
			for p.stack.versionCount() > pop[0].version+1 {
				p.stack.removeVersion(pop[0].version + 1)
			}
		}

		p.stack.renumberVersion(pop[0].version, version)
		merged := append(pop[0].subtrees, errorRepeat)
		errorRepeat = newNodeSubtree(builtinSymErrorRepeat, merged, 0, p.language)
	}

	p.stack.push(version, errorRepeat, false, errorState)
	if subtreeHasExternalTokens(lookahead) {
		p.stack.setLastExternalToken(version, subtreeLastExternalToken(lookahead))
	}

	hasError := true
	for i := stackVersion(0); i < p.stack.versionCount(); i++ {
		status := p.versionStatus(i)
		if !status.isInError {
			hasError = false
			break
		}
	}
	p.hasError = hasError
}

func (p *Parser) handleError(version stackVersion, lookahead subtree) {
	previousVersionCount := p.stack.versionCount()

	p.doAllPotentialReductions(version, 0)
	versionCount := p.stack.versionCount()
	position := p.stack.position(version)

	didInsertMissingToken := false
	for v := version; v < versionCount; {
		if !didInsertMissingToken {
			state := p.stack.state(v)
			for missingSymbol := Symbol(1); uint32(missingSymbol) < p.language.TokenCount; missingSymbol++ {
				stateAfterMissingSymbol := p.language.nextState(state, missingSymbol)
				if stateAfterMissingSymbol == 0 || stateAfterMissingSymbol == state {
					continue
				}

				if p.language.hasReduceAction(stateAfterMissingSymbol, subtreeLeafSymbol(lookahead)) {
					p.lexer.reset(position)
					p.lexer.MarkEnd()
					padding := lengthSub(p.lexer.tokenEndPosition, position)
					lookaheadBytes := subtreeTotalBytes(lookahead) + subtreeLookaheadBytes(lookahead)

					versionWithMissingTree := p.stack.copyVersion(v)
					missingTree := newMissingLeafSubtree(
						missingSymbol, state, padding, lookaheadBytes, p.language,
					)
					p.stack.push(
						versionWithMissingTree, missingTree, false, stateAfterMissingSymbol,
					)

					if p.doAllPotentialReductions(
						versionWithMissingTree, subtreeLeafSymbol(lookahead),
					) {
						didInsertMissingToken = true
						break
					}
				}
			}
		}

		p.stack.push(v, nil, false, errorState)
		if v == version {
			v = previousVersionCount
		} else {
			v++
		}
	}

	for i := previousVersionCount; i < versionCount; i++ {
		p.stack.merge(version, previousVersionCount)
	}

	p.stack.recordSummary(version, maxSummaryDepth)

	if subtreeChildCount(lookahead) > 0 {
		p.breakdownLookahead(&lookahead, errorState, &p.reusableNode)
	}
	p.recover(version, lookahead)
}

func (p *Parser) advance(version stackVersion, allowNodeReuse bool) bool {
	state := p.stack.state(version)
	position := p.stack.position(version).Bytes
	lastExternalToken := p.stack.lastExternalToken(version)

	didReuse := true
	var lookahead subtree
	var entry tableEntry

	if allowNodeReuse {
		lookahead = p.reuseNode(version, &state, position, lastExternalToken, &entry)
	}

	if lookahead == nil {
		didReuse = false
		lookahead = p.getCachedToken(state, position, lastExternalToken, &entry)
	}

	needsLex := lookahead == nil
	for {
		if needsLex {
			needsLex = false
			lookahead = p.lex(version, state)
			if p.hasScannerError {
				return false
			}

			if lookahead != nil {
				p.setCachedToken(position, lastExternalToken, lookahead)
				p.language.tableEntry(state, subtreeSymbol(lookahead), &entry)
			} else {
				p.language.tableEntry(state, BuiltinSymEnd, &entry)
			}
		}

		didReduce := false
		lastReductionVersion := stackVersionNone
		for i := uint32(0); i < entry.actionCount; i++ {
			action := entry.actions[i].Action

			switch action.Type {
			case ParseActionTypeShift:
				if action.Repetition {
					break
				}
				var nextState StateID
				if action.Extra {
					nextState = state
				} else {
					nextState = action.State
				}

				if subtreeChildCount(lookahead) > 0 {
					p.breakdownLookahead(&lookahead, state, &p.reusableNode)
					nextState = p.language.nextState(state, subtreeSymbol(lookahead))
				}

				p.shift(version, nextState, lookahead, action.Extra)
				if didReuse {
					p.reusableNode.advance()
				}
				return true

			case ParseActionTypeReduce:
				isFragile := entry.actionCount > 1
				endOfNonTerminalExtra := lookahead == nil
				reductionVersion := p.reduce(
					version, action.Symbol, uint32(action.ChildCount),
					int32(action.DynamicPrecedence), action.ProductionID,
					isFragile, endOfNonTerminalExtra,
				)
				didReduce = true
				if reductionVersion != stackVersionNone {
					lastReductionVersion = reductionVersion
				}

			case ParseActionTypeAccept:
				p.accept(version, lookahead)
				return true

			case ParseActionTypeRecover:
				if subtreeChildCount(lookahead) > 0 {
					p.breakdownLookahead(&lookahead, errorState, &p.reusableNode)
				}
				p.recover(version, lookahead)
				if didReuse {
					p.reusableNode.advance()
				}
				return true
			}
		}

		if lastReductionVersion != stackVersionNone {
			p.stack.renumberVersion(lastReductionVersion, version)
			state = p.stack.state(version)

			if lookahead == nil {
				needsLex = true
			} else {
				p.language.tableEntry(state, subtreeLeafSymbol(lookahead), &entry)
			}
			continue
		}

		if didReduce {
			if lookahead != nil {
				subtreeRelease(lookahead)
			}
			p.stack.halt(version)
			return true
		}

		if subtreeIsKeyword(lookahead) &&
			subtreeSymbol(lookahead) != p.language.KeywordCaptureToken &&
			!p.language.isReservedWord(state, subtreeSymbol(lookahead)) {
			p.language.tableEntry(state, p.language.KeywordCaptureToken, &entry)
			if entry.actionCount > 0 {
				mutableLookahead := subtreeMakeMut(lookahead)
				subtreeSetSymbol(mutableLookahead, p.language.KeywordCaptureToken, p.language)
				lookahead = mutableLookahead
				continue
			}
		}

		if state == errorState {
			p.recover(version, lookahead)
			return true
		}

		if p.breakdownTopOfStack(version) {
			state = p.stack.state(version)
			subtreeRelease(lookahead)
			needsLex = true
			continue
		}

		p.stack.pause(version, lookahead)
		return true
	}
}

func (p *Parser) condenseStack() uint32 {
	minErrorCost := ^uint32(0)
	for i := stackVersion(0); i < p.stack.versionCount(); i++ {
		if p.stack.isHalted(i) {
			p.stack.removeVersion(i)
			i--
			continue
		}

		statusI := p.versionStatus(i)
		if !statusI.isInError && statusI.cost < minErrorCost {
			minErrorCost = statusI.cost
		}

		for j := stackVersion(0); j < i; j++ {
			statusJ := p.versionStatus(j)

			switch compareVersions(statusJ, statusI) {
			case errorComparisonTakeLeft:
				p.stack.removeVersion(i)
				i--
				j = i
			case errorComparisonPreferLeft, errorComparisonNone:
				if p.stack.merge(j, i) {
					i--
					j = i
				}
			case errorComparisonPreferRight:
				if p.stack.merge(j, i) {
					i--
					j = i
				} else {
					p.stack.swapVersions(i, j)
				}
			case errorComparisonTakeRight:
				p.stack.removeVersion(j)
				i--
				j--
			}
		}
	}

	for p.stack.versionCount() > maxVersionCount {
		p.stack.removeVersion(maxVersionCount)
	}

	if p.stack.versionCount() > 0 {
		hasUnpausedVersion := false
		for i, n := stackVersion(0), p.stack.versionCount(); i < n; i++ {
			if p.stack.isPaused(i) {
				if !hasUnpausedVersion && p.acceptCount < maxVersionCount {
					minErrorCost = p.stack.errorCost(i)
					lookahead := p.stack.resume(i)
					p.handleError(i, lookahead)
					hasUnpausedVersion = true
				} else {
					p.stack.removeVersion(i)
					i--
					n--
				}
			} else {
				hasUnpausedVersion = true
			}
		}
	}

	return minErrorCost
}

func (p *Parser) balanceSubtree() {
	finishedTree := p.finishedTree
	var treeStack []subtree
	if subtreeChildCount(finishedTree) > 0 && finishedTree.refCount == 1 {
		treeStack = append(treeStack, finishedTree)
	}

	for len(treeStack) > 0 {
		tree := treeStack[len(treeStack)-1]

		if tree.repeatDepth > 0 {
			child1 := tree.children[0]
			child2 := tree.children[len(tree.children)-1]
			repeatDelta := int64(subtreeRepeatDepth(child1)) - int64(subtreeRepeatDepth(child2))
			if repeatDelta > 0 {
				n := uint32(repeatDelta)
				for i := n / 2; i > 0; i /= 2 {
					subtreeCompress(tree, i, p.language, &treeStack)
					n -= i
				}
			}
		}

		treeStack = treeStack[:len(treeStack)-1]

		for _, child := range tree.children {
			if subtreeChildCount(child) > 0 && child.refCount == 1 {
				treeStack = append(treeStack, child)
			}
		}
	}
}

func (p *Parser) hasOutstandingParse() bool {
	return p.externalScannerPayload != nil ||
		p.stack.state(0) != 1 ||
		p.stack.nodeCountSinceError(0) != 0
}

// Parse builds a syntax tree from the given input.
func (p *Parser) Parse(oldTree *Tree, input Input) *Tree {
	if p.language == nil || input.Read == nil {
		return nil
	}
	if oldTree != nil && oldTree.language != p.language {
		return nil
	}

	p.lexer.setInput(input)
	p.includedRangeDifferences = p.includedRangeDifferences[:0]
	p.includedRangeDiffIndex = 0

	if !p.hasOutstandingParse() {
		p.externalScannerCreate()
		if p.hasScannerError {
			p.Reset()
			return nil
		}

		if oldTree != nil {
			subtreeRetain(oldTree.root)
			p.oldTree = oldTree.root
			rangeArrayGetChangedRanges(
				oldTree.includedRanges, p.lexer.includedRanges, &p.includedRangeDifferences,
			)
			p.reusableNode.reset(oldTree.root)
		} else {
			p.reusableNode.clear()
		}
	}

	position := uint32(0)
	lastPosition := uint32(0)
	versionCount := uint32(0)
	for {
		for version := stackVersion(0); ; version++ {
			versionCount = p.stack.versionCount()
			if version >= versionCount {
				break
			}
			allowNodeReuse := versionCount == 1
			for p.stack.isActive(version) {
				if !p.advance(version, allowNodeReuse) {
					p.Reset()
					return nil
				}

				position = p.stack.position(version).Bytes
				if position > lastPosition || (version > 0 && position == lastPosition) {
					lastPosition = position
					break
				}
			}
		}

		minErrorCost := p.condenseStack()

		if p.finishedTree != nil && subtreeErrorCost(p.finishedTree) < minErrorCost {
			p.stack.clear()
			break
		}

		for p.includedRangeDiffIndex < uint32(len(p.includedRangeDifferences)) {
			r := p.includedRangeDifferences[p.includedRangeDiffIndex]
			if r.EndByte <= position {
				p.includedRangeDiffIndex++
			} else {
				break
			}
		}

		if versionCount == 0 {
			break
		}
	}

	if p.finishedTree == nil {
		p.Reset()
		return nil
	}

	p.balanceSubtree()

	result := newTree(p.finishedTree, p.language, p.lexer.includedRanges)
	p.finishedTree = nil

	p.Reset()
	return result
}

type stringInput struct {
	data []byte
}

func (s *stringInput) read(byteOffset uint32, _ Point) []byte {
	if byteOffset >= uint32(len(s.data)) {
		return nil
	}
	return s.data[byteOffset:]
}

// ParseString builds a syntax tree from a source string.
func (p *Parser) ParseString(oldTree *Tree, source []byte) *Tree {
	in := &stringInput{data: source}
	return p.Parse(oldTree, Input{Read: in.read, Encoding: EncodingUTF8})
}
