// query_cursor_states.go holds the in-progress state bookkeeping of query.c (v0.27.2).

package treesitter

// firstInProgressCapture is ts_query_cursor__first_in_progress_capture. It
// finds the in-progress state whose next capture comes earliest.
func (c *QueryCursor) firstInProgressCapture(wantDefinite bool) (stateIndex, byteOffset, patternIndex uint32, isDefinite, found bool) {
	stateIndex = ^uint32(0)
	byteOffset = ^uint32(0)
	patternIndex = ^uint32(0)
	for i := 0; i < len(c.states); i++ {
		state := &c.states[i]
		if state.dead {
			continue
		}
		captures := c.captureListPool.get(state.captureListID)
		if int(state.consumedCaptureCount) >= len(captures) {
			continue
		}
		node := captures[state.consumedCaptureCount].Node
		if node.EndByte() <= c.includedRange.StartByte ||
			pointLte(node.EndPoint(), c.includedRange.StartPoint) {
			state.consumedCaptureCount++
			i--
			continue
		}
		nodeStartByte := node.StartByte()
		if !found || nodeStartByte < byteOffset ||
			(nodeStartByte == byteOffset && uint32(state.patternIndex) < patternIndex) {
			step := &c.query.steps[state.stepIndex]
			if wantDefinite {
				// The next step must not be immediate: the capture could still be discarded.
				isDefinite = step.rootPatternGuaranteed && !step.isImmediate
			} else if step.rootPatternGuaranteed {
				continue
			}
			found = true
			stateIndex = uint32(i)
			byteOffset = nodeStartByte
			patternIndex = uint32(state.patternIndex)
		}
	}
	return
}

// compareNodes is ts_query_cursor__compare_nodes: depth-first order.
func compareNodes(left, right Node) int {
	if left.id != right.id {
		leftStart := left.StartByte()
		rightStart := right.StartByte()
		if leftStart < rightStart {
			return -1
		}
		if leftStart > rightStart {
			return 1
		}
		leftEnd := left.EndByte()
		rightEnd := right.EndByte()
		if leftEnd > rightEnd {
			return -1
		}
		if leftEnd < rightEnd {
			return 1
		}
	}
	return 0
}

// compareCaptures is ts_query_cursor__compare_captures: whether either
// state's captures are a superset of the other's.
func (c *QueryCursor) compareCaptures(leftState, rightState *queryState) (leftContainsRight, rightContainsLeft bool) {
	leftCaptures := c.captureListPool.get(leftState.captureListID)
	rightCaptures := c.captureListPool.get(rightState.captureListID)
	leftContainsRight = true
	rightContainsLeft = true
	i, j := 0, 0
	for {
		if i < len(leftCaptures) {
			if j < len(rightCaptures) {
				left := &leftCaptures[i]
				right := &rightCaptures[j]
				if left.Node.id == right.Node.id && left.Index == right.Index {
					i++
					j++
				} else {
					switch compareNodes(left.Node, right.Node) {
					case -1:
						rightContainsLeft = false
						i++
					case 1:
						leftContainsRight = false
						j++
					default:
						rightContainsLeft = false
						leftContainsRight = false
						i++
						j++
					}
				}
			} else {
				rightContainsLeft = false
				break
			}
		} else {
			if j < len(rightCaptures) {
				leftContainsRight = false
			}
			break
		}
	}
	return
}

// statePrecedes is ts_query_cursor__state_precedes: the order the dedup pass
// wants. Within a start depth and pattern, states without captures come first.
func (c *QueryCursor) statePrecedes(a, b *queryState) bool {
	if a.startDepth != b.startDepth {
		return a.startDepth < b.startDepth
	}
	if a.patternIndex != b.patternIndex {
		return a.patternIndex < b.patternIndex
	}
	aCaps := c.captureListPool.get(a.captureListID)
	bCaps := c.captureListPool.get(b.captureListID)
	if (len(aCaps) == 0) != (len(bCaps) == 0) {
		return len(aCaps) == 0
	}
	if len(aCaps) == 0 {
		return false
	}
	return aCaps[0].Node.StartByte() < bCaps[0].Node.StartByte()
}

// sortStatesByCapture is ts_query_cursor__sort_states_by_capture: a stable
// insertion sort, run once per node.
func (c *QueryCursor) sortStatesByCapture() {
	states := c.states
	for i := 1; i < len(states); i++ {
		if !c.statePrecedes(&states[i], &states[i-1]) {
			continue
		}
		key := states[i]
		j := i
		for {
			states[j] = states[j-1]
			j--
			if !(j > 0 && c.statePrecedes(&key, &states[j-1])) {
				break
			}
		}
		states[j] = key
	}
}

// insertState inserts a state at an index, keeping the order of the rest.
func (c *QueryCursor) insertState(index int, state queryState) {
	c.states = append(c.states, queryState{})
	copy(c.states[index+1:], c.states[index:])
	c.states[index] = state
}

// eraseState removes the state at an index, keeping the order of the rest.
func (c *QueryCursor) eraseState(index int) {
	copy(c.states[index:], c.states[index+1:])
	c.states = c.states[:len(c.states)-1]
}

// addState is ts_query_cursor__add_state. States stay ordered by start depth
// and pattern index, which lets later passes stop early.
func (c *QueryCursor) addState(pattern *patternEntry) {
	step := &c.query.steps[pattern.stepIndex]
	startDepth := c.depth - uint32(step.depth)

	index := len(c.states)
	for index > 0 {
		prevState := &c.states[index-1]
		if uint32(prevState.startDepth) < startDepth {
			break
		}
		if uint32(prevState.startDepth) == startDepth {
			// A duplicate would be pruned by the longest-match rule at once.
			if prevState.patternIndex == pattern.patternIndex && prevState.stepIndex == pattern.stepIndex {
				return
			}
			if prevState.patternIndex <= pattern.patternIndex {
				break
			}
		}
		index--
	}

	c.insertState(index, queryState{
		id:                    ^uint32(0),
		captureListID:         captureListNone,
		heapInsertOrder:       ^uint32(0),
		stepIndex:             pattern.stepIndex,
		patternIndex:          pattern.patternIndex,
		startDepth:            uint16(startDepth),
		seekingImmediateMatch: true,
		needsParent:           step.depth == 1,
	})
}

// prepareToCapture is ts_query_cursor__prepare_to_capture. With the pool
// exhausted it steals the list of the state with the earliest capture and
// marks that state dead. It reports false when no list can be had.
func (c *QueryCursor) prepareToCapture(state *queryState, stateIndexToPreserve uint32) bool {
	if state.captureListID == captureListNone {
		state.captureListID = c.captureListPool.acquire()
		if state.captureListID == captureListNone {
			c.didExceedMatchLimit = true
			stateIndex, _, _, _, found := c.firstInProgressCapture(false)
			if found && stateIndex != stateIndexToPreserve {
				otherState := &c.states[stateIndex]
				state.captureListID = otherState.captureListID
				otherState.captureListID = captureListNone
				otherState.dead = true
				c.captureListPool.list[state.captureListID] = c.captureListPool.list[state.captureListID][:0]
				return true
			}
			return false
		}
	}
	return true
}

// capture is ts_query_cursor__capture.
func (c *QueryCursor) capture(state *queryState, step *queryStep, node Node) {
	if state.dead {
		return
	}
	if !c.prepareToCapture(state, ^uint32(0)) {
		state.dead = true
		return
	}
	id := state.captureListID
	for _, captureID := range step.captureIDs {
		if captureID == noneIndex {
			break
		}
		c.captureListPool.list[id] = append(c.captureListPool.list[id], QueryCapture{Node: node, Index: uint32(captureID)})
	}
}

// copyState is ts_query_cursor__copy_state. It inserts a copy of the state
// right after it, and reports false when no capture list was left for it.
// Pointers into the states array are stale after a true return.
func (c *QueryCursor) copyState(stateIndex int) bool {
	state := c.states[stateIndex]
	stateCopy := state
	stateCopy.captureListID = captureListNone
	if state.captureListID != captureListNone {
		if !c.prepareToCapture(&stateCopy, uint32(stateIndex)) {
			return false
		}
		oldCaptures := c.captureListPool.get(c.states[stateIndex].captureListID)
		id := stateCopy.captureListID
		c.captureListPool.list[id] = append(c.captureListPool.list[id], oldCaptures...)
	}
	c.insertState(stateIndex+1, stateCopy)
	return true
}

// shouldDescend is ts_query_cursor__should_descend.
func (c *QueryCursor) shouldDescend(nodeIntersectsRange bool) bool {
	if nodeIntersectsRange && c.depth < c.maxStartDepth {
		return true
	}

	// An in-progress match with steps deeper in the tree needs the descent.
	for i := range c.states {
		state := &c.states[i]
		nextStep := &c.query.steps[state.stepIndex]
		if nextStep.depth != patternDoneMarker && uint32(state.startDepth)+uint32(nextStep.depth) > c.depth {
			return true
		}
	}

	if c.depth >= c.maxStartDepth {
		return false
	}

	// A non-rooted pattern can match one root inside a hidden node and
	// another in a sibling, so a hidden node may need the descent. A
	// repetition is only entered when the query can match a rootless pattern
	// inside that kind of repetition.
	if !c.onVisibleNode {
		sub := c.cursor.currentSubtree()
		if subtreeIsRepetition(sub) {
			_, exists := searchSorted(len(c.query.repeatSymbolsWithRootlessPatterns), func(i int) int {
				return cmpUint(c.query.repeatSymbolsWithRootlessPatterns[i], subtreeSymbol(sub))
			})
			return exists
		}
		return true
	}
	return false
}

// subtreeIsRepetition is ts_subtree_is_repetition.
func subtreeIsRepetition(s subtree) bool {
	if s.isInline() {
		return false
	}
	return !subtreeNamed(s) && !subtreeVisible(s) && subtreeChildCount(s) != 0
}

// rangeIntersects is range_intersects.
func rangeIntersects(a, b *Range) bool {
	isEmpty := a.StartByte == a.EndByte
	return (a.EndByte > b.StartByte || (isEmpty && a.EndByte == b.StartByte)) &&
		(pointGt(a.EndPoint, b.StartPoint) || (isEmpty && pointEq(a.EndPoint, b.StartPoint))) &&
		a.StartByte < b.EndByte &&
		pointLt(a.StartPoint, b.EndPoint)
}

// rangeWithin is range_within.
func rangeWithin(a, b *Range) bool {
	return a.StartByte >= b.StartByte &&
		pointGte(a.StartPoint, b.StartPoint) &&
		a.EndByte <= b.EndByte &&
		pointLte(a.EndPoint, b.EndPoint)
}

func pointGte(a, b Point) bool { return !pointLt(a, b) }

func nodeRange(n Node) Range {
	return Range{
		StartPoint: n.StartPoint(),
		EndPoint:   n.EndPoint(),
		StartByte:  n.StartByte(),
		EndByte:    n.EndByte(),
	}
}
