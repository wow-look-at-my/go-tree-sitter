// query_analyze.go holds the pattern analysis pass of query.c (v0.27.2). It
// walks the parse table to find patterns that can never match, and to mark
// the steps at which a pattern is sure to finish.

package treesitter

// analyzePatterns is ts_query__analyze_patterns. It reports the byte offset
// of an impossible pattern, and whether every pattern is possible.
func (q *Query) analyzePatterns() (uint32, bool) {
	var errorOffset uint32

	var nonRootedPatternStartSteps []uint16
	for i, pattern := range q.patternMap {
		if !pattern.isRooted {
			if q.steps[pattern.stepIndex].symbol != wildcardSymbol {
				nonRootedPatternStartSteps = append(nonRootedPatternStartSteps, uint16(i))
			}
		}
	}

	// Mark the steps that contain captures, and record the steps that have children.
	var parentStepIndices []uint32
	for i := 0; i < len(q.steps); i++ {
		step := &q.steps[i]
		if step.depth == patternDoneMarker {
			step.parentPatternGuaranteed = true
			step.rootPatternGuaranteed = true
			continue
		}

		hasChildren := false
		isWildcard := step.symbol == wildcardSymbol
		step.containsCaptures = step.captureIDs[0] != noneIndex
		for j := i + 1; j < len(q.steps); j++ {
			nextStep := &q.steps[j]
			if nextStep.depth == patternDoneMarker || nextStep.depth <= step.depth {
				break
			}
			if nextStep.captureIDs[0] != noneIndex {
				step.containsCaptures = true
			}
			if !isWildcard {
				nextStep.rootPatternGuaranteed = true
				nextStep.parentPatternGuaranteed = true
			}
			hasChildren = true
		}

		if hasChildren {
			if !isWildcard {
				parentStepIndices = append(parentStepIndices, uint32(i))
			} else if step.supertypeSymbol != 0 && q.language.ABIVersion >= languageVersionWithQueries {
				// Every direct child must be a subtype of the supertype.
				subtypes := q.language.Subtypes(step.supertypeSymbol)
				for j := i + 1; j < len(q.steps); j++ {
					childStep := &q.steps[j]
					if childStep.depth == patternDoneMarker || childStep.depth <= step.depth {
						break
					}
					if childStep.depth == step.depth+1 && childStep.symbol != wildcardSymbol {
						isValidSubtype := false
						for _, sub := range subtypes {
							if childStep.symbol == sub {
								isValidSubtype = true
								break
							}
						}
						if !isValidSubtype {
							for _, so := range q.stepOffsets {
								if int(so.stepIndex) >= j {
									return so.byteOffset, false
								}
							}
						}
					}
				}
			}
		}
	}

	// A subgraph per parent symbol in the query, and per hidden symbol in the grammar.
	var subgraphs []analysisSubgraph
	for _, parentStepIndex := range parentStepIndices {
		insertSubgraph(&subgraphs, q.steps[parentStepIndex].symbol)
	}
	for sym := Symbol(q.language.TokenCount); uint32(sym) < q.language.SymbolCount; sym++ {
		if !q.language.symbolMetadata(sym).Visible {
			insertSubgraph(&subgraphs, sym)
		}
	}

	// Scan the parse table for the states where these symbols start and end, and for each state's predecessors.
	predecessorMap := newStatePredecessorMap(q.language)
	for state := StateID(1); uint32(state) < q.language.StateCount; state++ {
		it := q.language.lookaheads(state)
		for it.next() {
			if it.actionCount > 0 {
				for i := uint16(0); i < it.actionCount; i++ {
					action := it.actions[i].Action
					if action.Type == ParseActionTypeReduce {
						for _, symbol := range q.language.AliasesForSymbol(action.Symbol) {
							if subgraphIndex, exists := searchSubgraphs(subgraphs, symbol); exists {
								subgraph := &subgraphs[subgraphIndex]
								if len(subgraph.nodes) == 0 || subgraph.nodes[len(subgraph.nodes)-1].state != state {
									subgraph.nodes = append(subgraph.nodes, analysisSubgraphNode{
										state:        state,
										productionID: action.ProductionID,
										childIndex:   action.ChildCount,
										done:         true,
									})
								}
							}
						}
					} else if action.Type == ParseActionTypeShift && !action.Extra {
						predecessorMap.add(action.State, state)
					}
				}
			} else if it.nextState != 0 {
				if it.nextState != state {
					predecessorMap.add(it.nextState, state)
				}
				if q.language.StateIsPrimary(state) {
					for _, symbol := range q.language.AliasesForSymbol(it.symbol) {
						if subgraphIndex, exists := searchSubgraphs(subgraphs, symbol); exists {
							subgraph := &subgraphs[subgraphIndex]
							if len(subgraph.startStates) == 0 ||
								subgraph.startStates[len(subgraph.startStates)-1] != state {
								subgraph.startStates = append(subgraph.startStates, state)
							}
						}
					}
				}
			}
		}
	}

	// Walk backward from each subgraph's end states through the predecessors.
	var nextNodes []analysisSubgraphNode
	for i := 0; i < len(subgraphs); i++ {
		subgraph := &subgraphs[i]
		if len(subgraph.nodes) == 0 {
			subgraphs = append(subgraphs[:i], subgraphs[i+1:]...)
			i--
			continue
		}
		nextNodes = append(nextNodes[:0], subgraph.nodes...)
		for len(nextNodes) > 0 {
			node := nextNodes[len(nextNodes)-1]
			nextNodes = nextNodes[:len(nextNodes)-1]
			if node.childIndex > 1 {
				for _, predecessor := range predecessorMap.get(node.state) {
					predecessorNode := analysisSubgraphNode{
						state:        predecessor,
						childIndex:   node.childIndex - 1,
						productionID: node.productionID,
						done:         false,
					}
					index, exists := searchSubgraphNodes(subgraph.nodes, &predecessorNode)
					if !exists {
						subgraph.nodes = append(subgraph.nodes, analysisSubgraphNode{})
						copy(subgraph.nodes[index+1:], subgraph.nodes[index:])
						subgraph.nodes[index] = predecessorNode
						nextNodes = append(nextNodes, predecessorNode)
					}
				}
			}
		}
	}

	// For each non-terminal pattern, find whether it can match, and the children at which matching can fail.
	var analysis queryAnalysis
	allPatternsAreValid := true
	for _, parentStepIndex := range parentStepIndices {
		parentDepth := q.steps[parentStepIndex].depth
		parentSymbol := q.steps[parentStepIndex].symbol
		if parentSymbol == BuiltinSymError {
			continue
		}

		// A parent symbol with no subgraph is a terminal: an error.
		subgraphIndex, exists := searchSubgraphs(subgraphs, parentSymbol)
		if !exists {
			firstChildStepIndex := uint16(parentStepIndex + 1)
			j, _ := searchSorted(len(q.stepOffsets), func(i int) int {
				return cmpUint(q.stepOffsets[i].stepIndex, firstChildStepIndex)
			})
			errorOffset = q.stepOffsets[j].byteOffset
			allPatternsAreValid = false
			break
		}

		subgraph := &subgraphs[subgraphIndex]
		analysis.states.clear(&analysis.statePool)
		analysis.deeperStates.clear(&analysis.statePool)
		for _, parseState := range subgraph.startStates {
			state := analysisState{
				stepIndex:  uint16(parentStepIndex + 1),
				depth:      1,
				rootSymbol: parentSymbol,
			}
			state.stack[0] = analysisStateEntry{parseState: parseState, parentSymbol: parentSymbol}
			analysis.states.push(&analysis.statePool, &state)
		}

		analysis.didAbort = false
		q.performAnalysis(subgraphs, &analysis)

		// A pattern that could not be analyzed is fallible at every step.
		if analysis.didAbort {
			for j := parentStepIndex + 1; j < uint32(len(q.steps)); j++ {
				step := &q.steps[j]
				if step.depth <= parentDepth || step.depth == patternDoneMarker {
					break
				}
				if !step.isDeadEnd {
					step.parentPatternGuaranteed = false
					step.rootPatternGuaranteed = false
				}
			}
			continue
		}

		// A pattern that cannot match: report where.
		if len(analysis.finishedParentSymbols) == 0 {
			var impossibleStepIndex uint16
			if len(analysis.finalStepIndices) > 0 {
				impossibleStepIndex = analysis.finalStepIndices[len(analysis.finalStepIndices)-1]
			} else {
				impossibleStepIndex = uint16(parentStepIndex)
			}
			j, _ := searchSorted(len(q.stepOffsets), func(i int) int {
				return cmpUint(q.stepOffsets[i].stepIndex, impossibleStepIndex)
			})
			if j >= len(q.stepOffsets) {
				j = len(q.stepOffsets) - 1
			}
			errorOffset = q.stepOffsets[j].byteOffset
			allPatternsAreValid = false
			break
		}

		// A step where a match terminated is fallible.
		for _, finalStepIndex := range analysis.finalStepIndices {
			step := &q.steps[finalStepIndex]
			if step.depth != patternDoneMarker && step.depth > parentDepth && !step.isDeadEnd {
				step.parentPatternGuaranteed = false
				step.rootPatternGuaranteed = false
			}
		}
	}

	// A step whose capture a predicate reads is indefinite.
	var predicateCaptureIDs []uint16
	for _, pattern := range q.patterns {
		predicateCaptureIDs = predicateCaptureIDs[:0]
		start := pattern.predicateStepsOffset
		for j := start; j < start+pattern.predicateStepsLength; j++ {
			step := q.predicateSteps[j]
			if step.Type == PredicateStepCapture {
				insertSortedUint16(&predicateCaptureIDs, uint16(step.ValueID))
			}
		}
		start = pattern.stepsOffset
		for j := start; j < start+pattern.stepsLength; j++ {
			step := &q.steps[j]
			for _, captureID := range step.captureIDs {
				if captureID == noneIndex {
					break
				}
				if _, exists := searchSortedUint16(predicateCaptureIDs, captureID); exists {
					step.rootPatternGuaranteed = false
					break
				}
			}
		}
	}

	// A pattern fallible at a step is fallible at every earlier step.
	done := len(q.steps) == 0
	for !done {
		done = true
		for i := len(q.steps) - 1; i > 0; i-- {
			step := &q.steps[i]
			if step.depth == patternDoneMarker {
				continue
			}

			parentPatternGuaranteed := false
			for {
				if step.rootPatternGuaranteed {
					parentPatternGuaranteed = true
					break
				}
				if step.alternativeIndex == noneIndex || int(step.alternativeIndex) < i {
					break
				}
				step = &q.steps[step.alternativeIndex]
			}

			if !parentPatternGuaranteed {
				prevStep := &q.steps[i-1]
				if !prevStep.isDeadEnd && prevStep.depth != patternDoneMarker && prevStep.rootPatternGuaranteed {
					prevStep.rootPatternGuaranteed = false
					done = false
				}
			}
		}
	}

	if !allPatternsAreValid {
		return errorOffset, false
	}

	// The repetition symbols in which a non-rooted pattern can match.
	analysis.didAbort = false
	for _, patternEntryIndex := range nonRootedPatternStartSteps {
		entry := q.patternMap[patternEntryIndex]

		analysis.states.clear(&analysis.statePool)
		analysis.deeperStates.clear(&analysis.statePool)
		for j := range subgraphs {
			subgraph := &subgraphs[j]
			metadata := q.language.symbolMetadata(subgraph.symbol)
			if metadata.Visible || metadata.Named {
				continue
			}
			for _, parseState := range subgraph.startStates {
				state := analysisState{
					stepIndex:  entry.stepIndex,
					depth:      1,
					rootSymbol: subgraph.symbol,
				}
				state.stack[0] = analysisStateEntry{parseState: parseState, parentSymbol: subgraph.symbol}
				analysis.states.push(&analysis.statePool, &state)
			}
		}

		q.performAnalysis(subgraphs, &analysis)

		if len(analysis.finishedParentSymbols) > 0 {
			q.patterns[entry.patternIndex].isNonLocal = true
		}
		for _, symbol := range analysis.finishedParentSymbols {
			index, exists := searchSorted(len(q.repeatSymbolsWithRootlessPatterns), func(i int) int {
				return cmpUint(q.repeatSymbolsWithRootlessPatterns[i], Symbol(symbol))
			})
			if !exists {
				q.repeatSymbolsWithRootlessPatterns = append(q.repeatSymbolsWithRootlessPatterns, 0)
				copy(q.repeatSymbolsWithRootlessPatterns[index+1:], q.repeatSymbolsWithRootlessPatterns[index:])
				q.repeatSymbolsWithRootlessPatterns[index] = Symbol(symbol)
			}
		}
	}

	return 0, true
}
