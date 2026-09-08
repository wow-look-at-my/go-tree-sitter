package treesitter

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

	if lookahead.isNil() {
		didReuse = false
		lookahead = p.getCachedToken(state, position, lastExternalToken, &entry)
	}

	needsLex := lookahead.isNil()
	for {
		if needsLex {
			needsLex = false
			lookahead = p.lex(version, state)
			if p.hasScannerError {
				return false
			}

			if !lookahead.isNil() {
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
				endOfNonTerminalExtra := lookahead.isNil()
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

			if lookahead.isNil() {
				needsLex = true
			} else {
				p.language.tableEntry(state, subtreeLeafSymbol(lookahead), &entry)
			}
			continue
		}

		if didReduce {
			if !lookahead.isNil() {
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
	// Only a node that has children is ever balanced, and such a node is always
	// on the heap, so the walk carries that arm rather than the union.
	var treeStack []*subtreeData
	if subtreeChildCount(finishedTree) > 0 && subtreeRefCount(finishedTree) == 1 {
		treeStack = append(treeStack, finishedTree.heap)
	}

	for len(treeStack) > 0 {
		tree := treeStack[len(treeStack)-1]

		if tree.repeatDepth > 0 {
			first := tree.children[0]
			last := tree.children[len(tree.children)-1]
			repeatDelta := int64(subtreeRepeatDepth(first)) - int64(subtreeRepeatDepth(last))
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
			if subtreeChildCount(child) > 0 && subtreeRefCount(child) == 1 {
				treeStack = append(treeStack, child.heap)
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

		if !p.finishedTree.isNil() && subtreeErrorCost(p.finishedTree) < minErrorCost {
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

	if p.finishedTree.isNil() {
		p.Reset()
		return nil
	}

	p.balanceSubtree()

	result := newTree(p.finishedTree, p.language, p.lexer.includedRanges)
	p.finishedTree = subtree{}

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
