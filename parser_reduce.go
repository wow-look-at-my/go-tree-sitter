package treesitter

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
				discarded := nextSlice.subtrees
				subtreeArrayClear(&discarded)
			}
		}

		state := p.stack.state(sliceVersion)
		nextState := p.language.nextState(state, symbol)
		if endOfNonTerminalExtra && nextState == state {
			subtreeSetExtra(&parent, true)
		}
		if isFragile || len(pop) > 1 || initialVersionCount > 1 {
			subtreeSetFragileLeft(parent.heap, true)
			subtreeSetFragileRight(parent.heap, true)
			subtreeSetParseState(parent.heap, treeStateNone)
		} else {
			subtreeSetParseState(parent.heap, state)
		}
		subtreeAddDynamicPrecedence(parent.heap, dynamicPrecedence)

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
				children := subtreeChildren(tree)
				for k := uint32(0); k < childCount; k++ {
					subtreeRetain(children[k])
				}
				spliced := make([]subtree, 0, len(trees)-1+int(childCount))
				spliced = append(spliced, trees[:j]...)
				spliced = append(spliced, children...)
				spliced = append(spliced, trees[j+1:]...)
				root = newNodeSubtree(
					subtreeSymbol(tree), spliced, subtreeProductionID(tree), p.language,
				)
				subtreeRelease(tree)
				break
			}
		}

		p.acceptCount++

		if !p.finishedTree.isNil() {
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
