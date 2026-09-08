package treesitter

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
					child := subtreeChildren(errorTree)[j]
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
		subtreeSetExtra(mutableLookahead, true)
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

		p.stack.push(v, subtree{}, false, errorState)
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
