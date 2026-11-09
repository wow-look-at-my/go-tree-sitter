// query_cursor_advance.go holds ts_query_cursor__advance of query.c (v0.27.2): the tree walk that drives the states.

package treesitter

// advance is ts_query_cursor__advance. It walks the tree until at least one
// pattern finishes, and reports whether one did. With stopOnDefiniteStep it
// also stops when a state reaches a step from which its pattern is sure to
// finish.
func (c *QueryCursor) advance(stopOnDefiniteStep bool) bool {
	didMatch := false
	for {
		if c.halted {
			for len(c.states) > 0 {
				state := c.states[len(c.states)-1]
				c.states = c.states[:len(c.states)-1]
				c.captureListPool.release(state.captureListID)
			}
		}

		if didMatch || c.halted {
			return didMatch
		}

		// Ask the progress callback after a bounded amount of work.
		c.workCount += 1 + uint(len(c.states))
		if c.workCount >= workPerQueryCallbackCheck {
			c.workCount = 0
			if c.options != nil && c.options.ProgressCallback != nil {
				c.state.CurrentByteOffset = c.cursor.currentNode().StartByte()
				if c.options.ProgressCallback(&c.state) {
					c.halted = true
					continue
				}
			}
		}

		if c.ascending {
			c.leaveNode(&didMatch)
			continue
		}
		c.enterNode(stopOnDefiniteStep, &didMatch)
	}
}

// leaveNode is the ascending half of ts_query_cursor__advance.
func (c *QueryCursor) leaveNode(didMatch *bool) {
	if c.onVisibleNode {
		// Drop the states that cannot progress past this node, and finish the deferred ones that completed inside it.
		deletedCount := 0
		for i, n := 0, len(c.states); i < n; i++ {
			state := &c.states[i]
			step := &c.query.steps[state.stepIndex]
			switch {
			case step.depth == patternDoneMarker && (uint32(state.startDepth) > c.depth || c.depth == 0):
				c.pushFinishedState(state)
				*didMatch = true
				deletedCount++
			case step.depth != patternDoneMarker && uint32(state.startDepth)+uint32(step.depth) > c.depth:
				c.captureListPool.release(state.captureListID)
				deletedCount++
			case deletedCount > 0:
				c.states[i-deletedCount] = *state
			}
		}
		c.states = c.states[:len(c.states)-deletedCount]
	}

	switch c.cursor.gotoNextSiblingInternal() {
	case treeCursorStepVisible:
		if !c.onVisibleNode {
			c.depth++
			c.onVisibleNode = true
		}
		c.ascending = false
	case treeCursorStepHidden:
		if c.onVisibleNode {
			c.depth--
			c.onVisibleNode = false
		}
		c.ascending = false
	default:
		if c.cursor.gotoParent() {
			c.depth--
		} else {
			c.halted = true
		}
	}
}

// enterNode is the descending half of ts_query_cursor__advance.
func (c *QueryCursor) enterNode(stopOnDefiniteStep bool, didMatch *bool) {
	node := c.cursor.currentNode()
	parentNode := c.cursor.parentNode()

	parentIntersectsRange := parentNode.IsNull()
	if !parentIntersectsRange {
		parentRange := nodeRange(parentNode)
		parentIntersectsRange = rangeIntersects(&parentRange, &c.includedRange)
	}
	nr := nodeRange(node)
	nodeIntersectsRange := parentIntersectsRange && rangeIntersects(&nr, &c.includedRange)
	nodeIntersectsContainingRange := rangeIntersects(&nr, &c.containingRange)
	nodeWithinContainingRange := rangeWithin(&nr, &c.containingRange)

	if nodeWithinContainingRange && c.onVisibleNode {
		c.matchNode(node, parentNode, nodeIntersectsRange, parentIntersectsRange, stopOnDefiniteStep, didMatch)
	}

	if nodeIntersectsContainingRange && c.shouldDescend(nodeIntersectsRange) {
		switch c.cursor.gotoFirstChildInternal() {
		case treeCursorStepVisible:
			c.depth++
			c.onVisibleNode = true
			return
		case treeCursorStepHidden:
			c.onVisibleNode = false
			return
		}
	}
	c.ascending = true
}

// matchNode is the part of ts_query_cursor__advance that starts new states
// at a visible node and advances the in-progress ones.
func (c *QueryCursor) matchNode(node, parentNode Node, nodeIntersectsRange, parentIntersectsRange, stopOnDefiniteStep bool, didMatch *bool) {
	q := c.query
	symbol := node.Symbol()
	isNamed := node.IsNamed()
	isMissing := node.IsMissing()
	status := &c.status
	c.cursor.currentStatus(status)

	nodeIsError := symbol == BuiltinSymError
	parentIsError := !parentNode.IsNull() && parentNode.Symbol() == BuiltinSymError

	// New states for the patterns whose root is a wildcard.
	if !nodeIsError {
		for i := 0; i < int(q.wildcardRootPatternCount); i++ {
			pattern := &q.patternMap[i]
			step := &q.steps[pattern.stepIndex]
			startDepth := c.depth - uint32(step.depth)
			var startsHere bool
			if pattern.isRooted {
				startsHere = nodeIntersectsRange
			} else {
				startsHere = parentIntersectsRange && !parentIsError
			}
			if startsHere &&
				(step.field == 0 || status.fieldID == step.field) &&
				(step.supertypeSymbol == 0 || status.supertypeCount > 0) &&
				startDepth <= c.maxStartDepth {
				c.addState(pattern)
			}
		}
	}

	// New states for the patterns whose root matches this node.
	if i, found := q.patternMapSearch(symbol); found {
		pattern := &q.patternMap[i]
		step := &q.steps[pattern.stepIndex]
		startDepth := c.depth - uint32(step.depth)
		for {
			var startsHere bool
			if pattern.isRooted {
				startsHere = nodeIntersectsRange
			} else {
				startsHere = parentIntersectsRange && !parentIsError
			}
			if startsHere &&
				(step.field == 0 || status.fieldID == step.field) &&
				startDepth <= c.maxStartDepth {
				c.addState(pattern)
			}
			i++
			if i == uint32(len(q.patternMap)) {
				break
			}
			pattern = &q.patternMap[i]
			step = &q.steps[pattern.stepIndex]
			if step.symbol != symbol {
				break
			}
		}
	}

	// Advance the in-progress states with this node.
	for j, copyCount := 0, 0; j < len(c.states); j += 1 + copyCount {
		state := &c.states[j]
		step := &q.steps[state.stepIndex]
		state.hasInProgressAlternatives = false
		copyCount = 0

		if uint32(state.startDepth)+uint32(step.depth) != c.depth {
			continue
		}

		// Does the node match this step, and can a later sibling match it?
		var nodeDoesMatch bool
		if step.symbol == wildcardSymbol {
			if step.isMissing {
				nodeDoesMatch = isMissing
			} else {
				nodeDoesMatch = !nodeIsError && (isNamed || !step.isNamed)
			}
		} else {
			nodeDoesMatch = symbol == step.symbol && (!step.isMissing || isMissing)
		}
		laterSiblingCanMatch := status.hasLaterSiblings
		if (step.isImmediate && isNamed && !state.skippedQuantifier) || state.seekingImmediateMatch {
			laterSiblingCanMatch = false
		}
		if step.isLastChild && status.hasLaterNamedSiblings {
			nodeDoesMatch = false
		}
		if step.supertypeSymbol != 0 {
			hasSupertype := false
			for k := 0; k < status.supertypeCount; k++ {
				if status.supertypes[k] == step.supertypeSymbol {
					hasSupertype = true
					break
				}
			}
			if !hasSupertype {
				nodeDoesMatch = false
			}
		}
		if step.field != 0 {
			if step.field == status.fieldID {
				if !status.canHaveLaterSiblingsWithThisField {
					laterSiblingCanMatch = false
				}
			} else {
				nodeDoesMatch = false
			}
		}

		if step.negatedFieldListID != 0 {
			for _, negatedFieldID := range q.negatedFields[step.negatedFieldListID:] {
				if negatedFieldID == 0 {
					break
				}
				if !node.ChildByFieldID(negatedFieldID).IsNull() {
					nodeDoesMatch = false
					break
				}
			}
		}

		// A state that can no longer match goes at once.
		if !nodeDoesMatch {
			if !laterSiblingCanMatch {
				c.captureListPool.release(state.captureListID)
				c.eraseState(j)
				j--
			}
			continue
		}

		// A step that could also match a later sibling splits the state:
		// one copy takes this node, one skips it.
		if laterSiblingCanMatch && (step.containsCaptures || q.stepIsFallible(state.stepIndex)) {
			if c.copyState(j) {
				copyCount++
				state = &c.states[j]
			}
		}

		// A pattern that skipped its wildcard root needs a parent, and
		// captures it when the root step captures.
		if state.needsParent {
			parent := c.cursor.parentNode()
			if parent.IsNull() {
				state.dead = true
			} else {
				state.needsParent = false
				skipped := state.stepIndex
				for {
					skipped--
					s := &q.steps[skipped]
					if !(s.isDeadEnd || s.isPassThrough || s.depth > 0) {
						break
					}
				}
				skippedWildcardStep := &q.steps[skipped]
				if skippedWildcardStep.captureIDs[0] != noneIndex {
					c.capture(state, skippedWildcardStep, parent)
				}
			}
		}

		if step.captureIDs[0] != noneIndex {
			c.capture(state, step, node)
		}

		if state.dead {
			c.eraseState(j)
			j--
			continue
		}

		state.stepIndex++
		nextStep := &q.steps[state.stepIndex]

		// An unnamed wildcard before an anchor must match the next sibling, named or not, so it reuses seekingImmediateMatch.
		state.seekingImmediateMatch = step.symbol == wildcardSymbol && !step.isNamed && nextStep.isImmediate
		// A zero-skip's exemption covers only the step it lands on.
		state.skippedQuantifier = false

		if stopOnDefiniteStep && nextStep.rootPatternGuaranteed {
			*didMatch = true
		}

		// Follow alternatives, copying the state for each branch.
		endIndex := j + 1
		for k := j; k < endIndex; k++ {
			stateIndex := k
			childState := &c.states[stateIndex]
			childStep := &q.steps[childState.stepIndex]
			if childStep.alternativeIndex == noneIndex {
				continue
			}

			// A dead end only redirects to its alternative.
			if childStep.isDeadEnd {
				childState.stepIndex = childStep.alternativeIndex
				k--
				continue
			}

			// A pass-through splits, then advances past itself. The same
			// state is looked at again on the next turn of the loop.
			if childStep.isPassThrough {
				childState.stepIndex++
				k--
			}

			// A zero-skip past a step that carries a trailing anchor hands
			// that anchor to the last matched node.
			if childStep.alternativeIsSkip && childStep.isLastChild && status.hasLaterNamedSiblings {
				continue
			}

			if c.copyState(stateIndex) {
				endIndex++
				copyCount++
				childState = &c.states[stateIndex]
				stateCopy := &c.states[stateIndex+1]
				stateCopy.stepIndex = childStep.alternativeIndex
				if childStep.isPassThrough {
					stateCopy.seekingImmediateMatch = true
				}
				// A zero-skip matched nothing: how an adjacent anchor
				// behaves depends on where it sat.
				if childStep.alternativeIsSkip {
					if !childStep.isImmediate {
						// No leading anchor on the skipped step: an anchor on the skip target is vacuous.
						skipTarget := &q.steps[childStep.alternativeIndex]
						stateCopy.skippedQuantifier = skipTarget.depth == childStep.depth
					} else if q.steps[childState.stepIndex-1].depth < childStep.depth {
						// The skipped step was the first child with a leading anchor: the skip target must be the first child.
						stateCopy.seekingImmediateMatch = true
					}
				}
			}
		}
	}

	c.sortStatesByCapture()

	for j := 0; j < len(c.states); j++ {
		state := &c.states[j]
		if state.dead {
			c.eraseState(j)
			j--
			continue
		}

		// The longest-match rule.
		didRemove := false
		for k := j + 1; k < len(c.states); k++ {
			otherState := &c.states[k]
			if otherState.startDepth != state.startDepth || otherState.patternIndex != state.patternIndex {
				break
			}

			// Once the other state's captures start past this state's last capture, the rest of the group is disjoint from it.
			stateCaptures := c.captureListPool.get(state.captureListID)
			otherCaptures := c.captureListPool.get(otherState.captureListID)
			if len(stateCaptures) > 0 && len(otherCaptures) > 0 &&
				otherCaptures[0].Node.StartByte() >= stateCaptures[len(stateCaptures)-1].Node.EndByte() {
				break
			}

			leftContainsRight, rightContainsLeft := c.compareCaptures(state, otherState)
			if leftContainsRight {
				if state.stepIndex == otherState.stepIndex &&
					(otherState.seekingImmediateMatch || !state.seekingImmediateMatch) {
					c.captureListPool.release(otherState.captureListID)
					c.eraseState(k)
					k--
					continue
				}
				otherState.hasInProgressAlternatives = true
			}
			if rightContainsLeft {
				if state.stepIndex == otherState.stepIndex &&
					(state.seekingImmediateMatch || !otherState.seekingImmediateMatch) {
					c.captureListPool.release(state.captureListID)
					c.eraseState(j)
					j--
					didRemove = true
					break
				}
				state.hasInProgressAlternatives = true
			}
		}

		// A state at the end of its pattern finishes, unless a longer
		// alternative is still in progress.
		if !didRemove {
			nextStep := &q.steps[state.stepIndex]
			if nextStep.depth == patternDoneMarker && !state.hasInProgressAlternatives {
				c.pushFinishedState(state)
				c.eraseState(j)
				*didMatch = true
				j--
			}
		}
	}
}
