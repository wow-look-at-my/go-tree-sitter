// query_cursor_next.go holds the match and capture iteration of query.c (v0.27.2).

package treesitter

// NextMatch is ts_query_cursor_next_match. The match's captures alias the
// cursor's storage and are valid until the next call.
func (c *QueryCursor) NextMatch(match *QueryMatch) bool {
	if len(c.finishedStates) == 0 {
		if !c.advance(false) {
			return false
		}
	}
	if c.finishedStatesHeapSize > 0 {
		c.heapifyFinishedStates()
	}

	stateIndex := uint32(0)
	if c.finishedStatesHeapSize > 0 {
		for i := uint32(1); i < uint32(len(c.finishedStates)); i++ {
			if c.finishedStates[i].heapInsertOrder < c.finishedStates[stateIndex].heapInsertOrder {
				stateIndex = i
			}
		}
	}

	state := &c.finishedStates[stateIndex]
	if state.id == ^uint32(0) {
		state.id = c.nextStateID
		c.nextStateID++
	}
	match.ID = state.id
	match.PatternIndex = state.patternIndex
	match.Captures = c.captureListPool.get(state.captureListID)
	c.captureListPool.release(state.captureListID)
	if c.finishedStatesHeapSize > 0 {
		c.finishedStateErase(stateIndex)
		c.finishedStatesHeapSize = uint32(len(c.finishedStates))
	} else {
		copy(c.finishedStates[stateIndex:], c.finishedStates[stateIndex+1:])
		c.finishedStates = c.finishedStates[:len(c.finishedStates)-1]
	}
	return true
}

// RemoveMatch is ts_query_cursor_remove_match: no more captures of the match
// are returned.
func (c *QueryCursor) RemoveMatch(matchID uint32) {
	if c.finishedStatesHeapSize > 0 {
		c.heapifyFinishedStates()
	}
	for i := range c.finishedStates {
		state := &c.finishedStates[i]
		if state.id == matchID {
			c.captureListPool.release(state.captureListID)
			if c.finishedStatesHeapSize > 0 {
				c.finishedStateErase(uint32(i))
				c.finishedStatesHeapSize = uint32(len(c.finishedStates))
			} else {
				copy(c.finishedStates[i:], c.finishedStates[i+1:])
				c.finishedStates = c.finishedStates[:len(c.finishedStates)-1]
			}
			return
		}
	}
	for i := range c.states {
		state := &c.states[i]
		if state.id == matchID {
			c.captureListPool.release(state.captureListID)
			c.eraseState(i)
			return
		}
	}
}

// NextCapture is ts_query_cursor_next_capture. It returns captures in
// document order, which can come from a match that has not finished yet. The
// index names the capture in the match's list.
func (c *QueryCursor) NextCapture(match *QueryMatch) (uint32, bool) {
	for {
		c.heapifyFinishedStates()

		firstUnfinishedStateIndex, firstUnfinishedCaptureByte, firstUnfinishedPatternIndex,
			firstUnfinishedStateIsDefinite, foundUnfinishedState := c.firstInProgressCapture(true)

		// The earliest finished capture sits at the heap's root.
		var firstFinishedState *queryState
		firstFinishedCaptureByte := firstUnfinishedCaptureByte
		firstFinishedPatternIndex := firstUnfinishedPatternIndex
		for len(c.finishedStates) > 0 {
			state := &c.finishedStates[0]
			captures := c.captureListPool.get(state.captureListID)

			if int(state.consumedCaptureCount) >= len(captures) {
				c.captureListPool.release(state.captureListID)
				c.finishedStatePop()
				c.finishedStatesHeapSize = uint32(len(c.finishedStates))
				continue
			}

			node := captures[state.consumedCaptureCount].Node
			nodePrecedesRange := node.EndByte() <= c.includedRange.StartByte ||
				pointLte(node.EndPoint(), c.includedRange.StartPoint)
			nodeFollowsRange := node.StartByte() >= c.includedRange.EndByte ||
				pointGte(node.StartPoint(), c.includedRange.EndPoint)
			if nodePrecedesRange || nodeFollowsRange {
				state.consumedCaptureCount++
				c.finishedStateSiftDown(0)
				continue
			}

			nodeStartByte := node.StartByte()
			if nodeStartByte < firstFinishedCaptureByte ||
				(nodeStartByte == firstFinishedCaptureByte && uint32(state.patternIndex) < firstFinishedPatternIndex) {
				firstFinishedState = state
				firstFinishedCaptureByte = nodeStartByte
				firstFinishedPatternIndex = uint32(state.patternIndex)
			}
			break
		}

		var state *queryState
		switch {
		case firstFinishedState != nil:
			state = firstFinishedState
		case firstUnfinishedStateIsDefinite:
			state = &c.states[firstUnfinishedStateIndex]
		}

		if state != nil {
			if state.id == ^uint32(0) {
				state.id = c.nextStateID
				c.nextStateID++
			}
			match.ID = state.id
			match.PatternIndex = state.patternIndex
			match.Captures = c.captureListPool.get(state.captureListID)
			captureIndex := uint32(state.consumedCaptureCount)
			state.consumedCaptureCount++
			if state == firstFinishedState {
				c.finishedStateSiftDown(0)
			}
			return captureIndex, true
		}

		if c.captureListPool.isEmpty() && foundUnfinishedState {
			c.captureListPool.release(c.states[firstUnfinishedStateIndex].captureListID)
			c.eraseState(int(firstUnfinishedStateIndex))
		}

		if !c.advance(true) && len(c.finishedStates) == 0 {
			return 0, false
		}
	}
}
