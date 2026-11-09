// query_analysis_walk.go holds ts_query__perform_analysis of query.c (v0.27.2): the parse table walk behind the analysis pass.

package treesitter

// performAnalysis is ts_query__perform_analysis. It walks the subgraph for a
// non-terminal and records every step at which the pattern can stop.
func (q *Query) performAnalysis(subgraphs []analysisSubgraph, analysis *queryAnalysis) {
	recursionDepthLimit := uint(0)
	prevFinalStepCount := 0
	analysis.finalStepIndices = analysis.finalStepIndices[:0]
	analysis.finishedParentSymbols = analysis.finishedParentSymbols[:0]

	for iteration := 0; ; iteration++ {
		if iteration == maxAnalysisIterationCount {
			analysis.didAbort = true
			break
		}

		// With no progress at this recursion depth, allow one more level,
		// but only when the last increase found new final steps.
		if len(analysis.states) == 0 {
			if len(analysis.deeperStates) > 0 && len(analysis.finalStepIndices) > prevFinalStepCount {
				prevFinalStepCount = len(analysis.finalStepIndices)
				recursionDepthLimit++
				analysis.states, analysis.deeperStates = analysis.deeperStates, analysis.states
				continue
			}
			break
		}

		analysis.nextStates.clear(&analysis.statePool)
		for j := 0; j < len(analysis.states); j++ {
			state := analysis.states[j]

			// Advance the states that made the least progress first, and
			// never advance a state past one that made more.
			if len(analysis.nextStates) > 0 {
				comparison := compareAnalysisStates(state, analysis.nextStates[len(analysis.nextStates)-1])
				if comparison == 0 {
					analysis.nextStates.insertSorted(&analysis.statePool, state)
					continue
				} else if comparison > 0 {
					for j < len(analysis.states) {
						analysis.nextStates.push(&analysis.statePool, analysis.states[j])
						j++
					}
					break
				}
			}

			parseState := state.top().parseState
			parentSymbol := state.top().parentSymbol
			parentFieldID := state.top().fieldID
			childIndex := uint(state.top().childIndex)
			step := &q.steps[state.stepIndex]

			subgraphIndex, exists := searchSubgraphs(subgraphs, parentSymbol)
			if !exists {
				continue
			}
			subgraph := &subgraphs[subgraphIndex]

			// Follow every path in the parse table that stays in the subgraph.
			it := q.language.lookaheads(parseState)
			for it.next() {
				sym := it.symbol

				successor := analysisSubgraphNode{
					state:      parseState,
					childIndex: uint8(childIndex),
				}
				if it.actionCount > 0 {
					action := it.actions[it.actionCount-1].Action
					if action.Type == ParseActionTypeShift {
						if !action.Extra {
							successor.state = action.State
							successor.childIndex++
						}
					} else {
						continue
					}
				} else if it.nextState != 0 {
					successor.state = it.nextState
					successor.childIndex++
				} else {
					continue
				}

				nodeIndex, _ := searchSubgraphNodes(subgraph.nodes, &successor)
				for nodeIndex < len(subgraph.nodes) {
					node := &subgraph.nodes[nodeIndex]
					nodeIndex++
					if node.state != successor.state || node.childIndex != successor.childIndex {
						break
					}

					// The alias and field this child would take.
					alias := q.language.AliasAt(uint32(node.productionID), uint32(childIndex))
					var visibleSymbol Symbol
					switch {
					case alias != 0:
						visibleSymbol = alias
					case q.language.symbolMetadata(sym).Visible:
						visibleSymbol = q.language.publicSymbol(sym)
					}
					fieldID := parentFieldID
					if fieldID == 0 {
						for _, entry := range q.language.fieldMap(uint32(node.productionID)) {
							if !entry.Inherited && uint(entry.ChildIndex) == childIndex {
								fieldID = entry.FieldID
								break
							}
						}
					}

					// A state that has advanced past this hypothetical subtree.
					nextState := *state
					nextStateTop := nextState.top()
					nextStateTop.childIndex = uint16(successor.childIndex)
					nextStateTop.parseState = successor.state
					if node.done {
						nextStateTop.done = true
					}

					doesMatch := false
					switch {
					case step.symbol == BuiltinSymError:
						// An ERROR node can appear anywhere.
						doesMatch = true
					case visibleSymbol != 0:
						doesMatch = true
						if step.symbol == wildcardSymbol {
							if step.isNamed && !q.language.symbolMetadata(visibleSymbol).Named {
								doesMatch = false
							}
						} else if step.symbol != visibleSymbol {
							doesMatch = false
						}
						if step.field != 0 && step.field != fieldID {
							doesMatch = false
						}
						if step.supertypeSymbol != 0 && !state.hasSupertype(step.supertypeSymbol) {
							doesMatch = false
						}
					case uint32(sym) >= q.language.TokenCount:
						// A hidden child: descend into it. Replace a finished
						// top entry, or push a new one.
						if !nextStateTop.done {
							if int(nextState.depth)+1 >= maxAnalysisStateDepth {
								analysis.didAbort = true
								continue
							}
							nextState.depth++
							nextStateTop = nextState.top()
						}
						*nextStateTop = analysisStateEntry{
							parseState:   parseState,
							parentSymbol: sym,
							childIndex:   0,
							fieldID:      fieldID,
							done:         false,
						}
						if nextState.recursionDepth() > recursionDepthLimit {
							analysis.deeperStates.insertSorted(&analysis.statePool, &nextState)
							continue
						}
					}

					// Pop finished syntax nodes.
					for nextState.depth > 0 && nextStateTop.done {
						nextState.depth--
						nextStateTop = nextState.top()
					}

					// On a match, advance to the next step at this depth, past the descendant steps of the matched child.
					nextStep := step
					if doesMatch {
						for {
							nextState.stepIndex++
							nextStep = &q.steps[nextState.stepIndex]
							if nextStep.depth == patternDoneMarker || nextStep.depth <= step.depth {
								break
							}
						}
					} else if successor.state == parseState {
						continue
					}

					for {
						// Analysis needs no repetitions, so skip pass-through steps.
						if nextStep.isPassThrough {
							nextState.stepIndex++
							nextStep = &q.steps[nextState.stepIndex]
							continue
						}

						if !nextStep.isDeadEnd {
							didFinishPattern := q.steps[nextState.stepIndex].depth != step.depth
							switch {
							case didFinishPattern:
								insertSortedUint16(&analysis.finishedParentSymbols, state.rootSymbol)
							case nextState.depth == 0:
								insertSortedUint16(&analysis.finalStepIndices, nextState.stepIndex)
							default:
								analysis.nextStates.insertSorted(&analysis.statePool, &nextState)
							}
						}

						// Follow a forward alternative with another state.
						if doesMatch &&
							nextStep.alternativeIndex != noneIndex &&
							nextStep.alternativeIndex > nextState.stepIndex {
							nextState.stepIndex = nextStep.alternativeIndex
							nextStep = &q.steps[nextState.stepIndex]
						} else {
							break
						}
					}
				}
			}
		}

		analysis.states, analysis.nextStates = analysis.nextStates, analysis.states
	}
}
