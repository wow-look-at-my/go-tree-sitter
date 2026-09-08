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
	lexer                    Lexer
	stack                    *parseStack
	language                 *Language
	reduceActions            []reduceAction
	finishedTree             subtree
	trailingExtras           []subtree
	trailingExtras2          []subtree
	scratchTrees             []subtree
	tokenCache               tokenCache
	reusableNode             reusableNode
	externalScannerPayload   any
	acceptCount              uint32
	oldTree                  subtree
	includedRangeDifferences []Range
	includedRangeDiffIndex   uint32
	hasScannerError          bool
	hasError                 bool
	serializationBuffer      [serializationBufferSize]byte
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
	size := p.language.Scanner.Serialize(p.externalScannerPayload, p.serializationBuffer[:])
	if size > serializationBufferSize {
		size = serializationBufferSize
	}
	return size
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
