// query_cursor.go holds the query cursor of query.c (v0.27.2): the state
// machine that runs compiled patterns over a tree.

package treesitter

// queryState is a QueryState: one in-progress match of one pattern.
type queryState struct {
	id                        uint32
	captureListID             uint32
	heapInsertOrder           uint32
	startDepth                uint16
	stepIndex                 uint16
	patternIndex              uint16
	consumedCaptureCount      uint16
	seekingImmediateMatch     bool
	hasInProgressAlternatives bool
	dead                      bool
	needsParent               bool
	skippedQuantifier         bool
}

// captureListPool is a CaptureListPool.
type captureListPool struct {
	list                []([]QueryCapture)
	inUse               []bool
	maxCaptureListCount uint32
	freeIDStack         []uint32
}

// newCaptureListPool is capture_list_pool_new.
func newCaptureListPool() captureListPool {
	return captureListPool{maxCaptureListCount: ^uint32(0)}
}

// reset is capture_list_pool_reset.
func (p *captureListPool) reset() {
	p.freeIDStack = p.freeIDStack[:0]
	for i := range p.list {
		p.inUse[i] = false
		p.freeIDStack = append(p.freeIDStack, uint32(i))
	}
}

// get is capture_list_pool_get. An id outside the pool names the empty list.
func (p *captureListPool) get(id uint32) []QueryCapture {
	if id >= uint32(len(p.list)) {
		return nil
	}
	return p.list[id]
}

// isEmpty is capture_list_pool_is_empty: every list is in use and the pool
// may not grow.
func (p *captureListPool) isEmpty() bool {
	return len(p.freeIDStack) == 0 && uint32(len(p.list)) >= p.maxCaptureListCount
}

// acquire is capture_list_pool_acquire.
func (p *captureListPool) acquire() uint32 {
	if n := len(p.freeIDStack); n > 0 {
		id := p.freeIDStack[n-1]
		p.freeIDStack = p.freeIDStack[:n-1]
		p.list[id] = p.list[id][:0]
		p.inUse[id] = true
		return id
	}
	i := uint32(len(p.list))
	if i >= p.maxCaptureListCount {
		return captureListNone
	}
	p.list = append(p.list, nil)
	p.inUse = append(p.inUse, true)
	return i
}

// release is capture_list_pool_release.
func (p *captureListPool) release(id uint32) {
	if id >= uint32(len(p.list)) || !p.inUse[id] {
		return
	}
	p.inUse[id] = false
	p.freeIDStack = append(p.freeIDStack, id)
}

// QueryCursorState is a TSQueryCursorState: what a progress callback sees.
type QueryCursorState struct {
	Payload           any
	CurrentByteOffset uint32
}

// QueryCursorOptions is a TSQueryCursorOptions. ProgressCallback runs after a
// bounded amount of work.
type QueryCursorOptions struct {
	Payload          any
	ProgressCallback func(state *QueryCursorState) bool
}

// QueryCursor is a TSQueryCursor: it executes one query over one tree at a
// time, and can be reused across executions.
type QueryCursor struct {
	query                  *Query
	cursor                 queryTreeCursor
	states                 []queryState
	finishedStates         []queryState
	finishedStatesHeapSize uint32
	captureListPool        captureListPool
	depth                  uint32
	maxStartDepth          uint32
	includedRange          Range
	containingRange        Range
	nextStateID            uint32
	nextFinishedStateID    uint32
	options                *QueryCursorOptions
	state                  QueryCursorState
	workCount              uint
	onVisibleNode          bool
	ascending              bool
	halted                 bool
	didExceedMatchLimit    bool
	status                 cursorStatus
}

var pointMax = Point{Row: ^uint32(0), Column: ^uint32(0)}

// NewQueryCursor is ts_query_cursor_new.
func NewQueryCursor() *QueryCursor {
	return &QueryCursor{
		states:          make([]queryState, 0, 8),
		finishedStates:  make([]queryState, 0, 8),
		captureListPool: newCaptureListPool(),
		includedRange:   Range{EndPoint: pointMax, EndByte: ^uint32(0)},
		containingRange: Range{EndPoint: pointMax, EndByte: ^uint32(0)},
		maxStartDepth:   ^uint32(0),
	}
}

// DidExceedMatchLimit is ts_query_cursor_did_exceed_match_limit.
func (c *QueryCursor) DidExceedMatchLimit() bool { return c.didExceedMatchLimit }

// MatchLimit is ts_query_cursor_match_limit.
func (c *QueryCursor) MatchLimit() uint32 { return c.captureListPool.maxCaptureListCount }

// SetMatchLimit is ts_query_cursor_set_match_limit.
func (c *QueryCursor) SetMatchLimit(limit uint32) { c.captureListPool.maxCaptureListCount = limit }

// SetMaxStartDepth is ts_query_cursor_set_max_start_depth.
func (c *QueryCursor) SetMaxStartDepth(maxStartDepth uint32) { c.maxStartDepth = maxStartDepth }

// Exec is ts_query_cursor_exec: it starts the query over the node's subtree.
func (c *QueryCursor) Exec(query *Query, node Node) {
	c.states = c.states[:0]
	c.finishedStates = c.finishedStates[:0]
	c.finishedStatesHeapSize = 0
	c.cursor.reset(node)
	c.captureListPool.reset()
	c.onVisibleNode = true
	c.nextStateID = 0
	c.nextFinishedStateID = 0
	c.depth = 0
	c.ascending = false
	c.halted = false
	c.query = query
	c.didExceedMatchLimit = false
	c.workCount = 0
	c.options = nil
	c.state = QueryCursorState{}
	// A null node has nothing to walk. Upstream reads through the null
	// pointer; this halts instead, so the cursor reports no matches.
	if node.IsNull() || query == nil {
		c.halted = true
	}
}

// ExecWithOptions is ts_query_cursor_exec_with_options.
func (c *QueryCursor) ExecWithOptions(query *Query, node Node, options *QueryCursorOptions) {
	c.Exec(query, node)
	if options != nil {
		c.options = options
		c.state = QueryCursorState{Payload: options.Payload}
	}
}

// SetByteRange is ts_query_cursor_set_byte_range. An end of zero means the end
// of the document. It reports false for a start past the end.
func (c *QueryCursor) SetByteRange(startByte, endByte uint32) bool {
	if endByte == 0 {
		endByte = ^uint32(0)
	}
	if startByte > endByte {
		return false
	}
	c.includedRange.StartByte = startByte
	c.includedRange.EndByte = endByte
	return true
}

// SetPointRange is ts_query_cursor_set_point_range.
func (c *QueryCursor) SetPointRange(startPoint, endPoint Point) bool {
	if endPoint.Row == 0 && endPoint.Column == 0 {
		endPoint = pointMax
	}
	if pointGt(startPoint, endPoint) {
		return false
	}
	c.includedRange.StartPoint = startPoint
	c.includedRange.EndPoint = endPoint
	return true
}

// SetContainingByteRange is ts_query_cursor_set_containing_byte_range: only a
// match whose nodes lie within the range is returned.
func (c *QueryCursor) SetContainingByteRange(startByte, endByte uint32) bool {
	if endByte == 0 {
		endByte = ^uint32(0)
	}
	if startByte > endByte {
		return false
	}
	c.containingRange.StartByte = startByte
	c.containingRange.EndByte = endByte
	return true
}

// SetContainingPointRange is ts_query_cursor_set_containing_point_range.
func (c *QueryCursor) SetContainingPointRange(startPoint, endPoint Point) bool {
	if endPoint.Row == 0 && endPoint.Column == 0 {
		endPoint = pointMax
	}
	if pointGt(startPoint, endPoint) {
		return false
	}
	c.containingRange.StartPoint = startPoint
	c.containingRange.EndPoint = endPoint
	return true
}

// finishedStatePrecedes is finished_state_precedes: the heap order of
// finished states, by next unconsumed capture, pattern, then insertion.
func (c *QueryCursor) finishedStatePrecedes(a, b *queryState) bool {
	aCaps := c.captureListPool.get(a.captureListID)
	bCaps := c.captureListPool.get(b.captureListID)
	if int(a.consumedCaptureCount) >= len(aCaps) {
		return false
	}
	if int(b.consumedCaptureCount) >= len(bCaps) {
		return true
	}
	aByte := aCaps[a.consumedCaptureCount].Node.StartByte()
	bByte := bCaps[b.consumedCaptureCount].Node.StartByte()
	if aByte != bByte {
		return aByte < bByte
	}
	if a.patternIndex != b.patternIndex {
		return a.patternIndex < b.patternIndex
	}
	return a.heapInsertOrder < b.heapInsertOrder
}

// finishedStateSiftDown is finished_state_sift_down.
func (c *QueryCursor) finishedStateSiftDown(index uint32) {
	states := c.finishedStates
	size := uint32(len(states))
	for {
		smallest := index
		left := 2*index + 1
		right := 2*index + 2
		if left < size && c.finishedStatePrecedes(&states[left], &states[smallest]) {
			smallest = left
		}
		if right < size && c.finishedStatePrecedes(&states[right], &states[smallest]) {
			smallest = right
		}
		if smallest == index {
			break
		}
		states[index], states[smallest] = states[smallest], states[index]
		index = smallest
	}
}

// finishedStateSiftUp is finished_state_sift_up.
func (c *QueryCursor) finishedStateSiftUp(index uint32) {
	states := c.finishedStates
	for index > 0 {
		parent := (index - 1) / 2
		if c.finishedStatePrecedes(&states[index], &states[parent]) {
			states[index], states[parent] = states[parent], states[index]
			index = parent
		} else {
			break
		}
	}
}

// finishedStatePop is finished_state_pop.
func (c *QueryCursor) finishedStatePop() {
	n := len(c.finishedStates)
	if n > 1 {
		c.finishedStates[0] = c.finishedStates[n-1]
	}
	c.finishedStates = c.finishedStates[:n-1]
	if len(c.finishedStates) > 0 {
		c.finishedStateSiftDown(0)
	}
}

// finishedStateErase is finished_state_erase.
func (c *QueryCursor) finishedStateErase(index uint32) {
	n := uint32(len(c.finishedStates))
	if index == n-1 {
		c.finishedStates = c.finishedStates[:n-1]
		return
	}
	c.finishedStates[index] = c.finishedStates[n-1]
	c.finishedStates = c.finishedStates[:n-1]
	if index > 0 && c.finishedStatePrecedes(&c.finishedStates[index], &c.finishedStates[(index-1)/2]) {
		c.finishedStateSiftUp(index)
	} else {
		c.finishedStateSiftDown(index)
	}
}

// pushFinishedState is ts_query_cursor__push_finished_state.
func (c *QueryCursor) pushFinishedState(state *queryState) {
	state.heapInsertOrder = c.nextFinishedStateID
	c.nextFinishedStateID++
	c.finishedStates = append(c.finishedStates, *state)
}

// heapifyFinishedStates is ts_query_cursor__heapify_finished_states.
func (c *QueryCursor) heapifyFinishedStates() {
	for c.finishedStatesHeapSize < uint32(len(c.finishedStates)) {
		c.finishedStateSiftUp(c.finishedStatesHeapSize)
		c.finishedStatesHeapSize++
	}
}
