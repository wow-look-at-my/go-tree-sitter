// query_analysis_state.go holds the data structures of the analysis pass of query.c (v0.27.2).

package treesitter

// searchSorted is _array__search_sorted: a binary search over n sorted items.
// cmp compares item i with the needle. It reports the index of the needle, or
// the index it would take, and whether the needle is present.
func searchSorted(n int, cmp func(i int) int) (int, bool) {
	index := 0
	size := n
	if size == 0 {
		return 0, false
	}
	for size > 1 {
		halfSize := size / 2
		midIndex := index + halfSize
		if cmp(midIndex) <= 0 {
			index = midIndex
		}
		size -= halfSize
	}
	comparison := cmp(index)
	if comparison == 0 {
		return index, true
	}
	if comparison < 0 {
		index++
	}
	return index, false
}

// insertSortedUint16 is array_insert_sorted_by on a list of uint16.
func insertSortedUint16(list *[]uint16, value uint16) {
	index, exists := searchSorted(len(*list), func(i int) int { return cmpUint((*list)[i], value) })
	if !exists {
		*list = append(*list, 0)
		copy((*list)[index+1:], (*list)[index:])
		(*list)[index] = value
	}
}

// searchSortedUint16 is array_search_sorted_by on a list of uint16.
func searchSortedUint16(list []uint16, value uint16) (int, bool) {
	return searchSorted(len(list), func(i int) int { return cmpUint(list[i], value) })
}

func cmpUint[T uint16 | uint32](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// statePredecessorMap is a StatePredecessorMap: the states that can shift into
// each state, up to maxStatePredecessorCount of them.
type statePredecessorMap struct {
	contents []StateID
}

// newStatePredecessorMap is state_predecessor_map_new.
func newStatePredecessorMap(language *Language) statePredecessorMap {
	return statePredecessorMap{
		contents: make([]StateID, int(language.StateCount)*(maxStatePredecessorCount+1)),
	}
}

// add is state_predecessor_map_add.
func (m *statePredecessorMap) add(state, predecessor StateID) {
	index := int(state) * (maxStatePredecessorCount + 1)
	count := &m.contents[index]
	if *count == 0 ||
		(*count < maxStatePredecessorCount && m.contents[index+int(*count)] != predecessor) {
		*count++
		m.contents[index+int(*count)] = predecessor
	}
}

// get is state_predecessor_map_get.
func (m *statePredecessorMap) get(state StateID) []StateID {
	index := int(state) * (maxStatePredecessorCount + 1)
	count := int(m.contents[index])
	return m.contents[index+1 : index+1+count]
}

// analysisStateEntry is an AnalysisStateEntry.
type analysisStateEntry struct {
	parseState   StateID
	parentSymbol Symbol
	childIndex   uint16
	fieldID      FieldID
	done         bool
}

// analysisState is an AnalysisState: a hypothetical position in a syntax
// tree, paired with a position in a pattern.
type analysisState struct {
	stack      [maxAnalysisStateDepth]analysisStateEntry
	depth      uint16
	stepIndex  uint16
	rootSymbol Symbol
}

// recursionDepth is analysis_state__recursion_depth.
func (s *analysisState) recursionDepth() uint {
	result := uint(0)
	for i := uint16(0); i < s.depth; i++ {
		symbol := s.stack[i].parentSymbol
		for j := uint16(0); j < i; j++ {
			if s.stack[j].parentSymbol == symbol {
				result++
				break
			}
		}
	}
	return result
}

// compareAnalysisStates is analysis_state__compare.
func compareAnalysisStates(a, b *analysisState) int {
	if a.depth < b.depth {
		return 1
	}
	for i := uint16(0); i < a.depth; i++ {
		if i >= b.depth {
			return -1
		}
		s1 := a.stack[i]
		s2 := b.stack[i]
		if s1.childIndex != s2.childIndex {
			return cmpUint(s1.childIndex, s2.childIndex)
		}
		if s1.parentSymbol != s2.parentSymbol {
			return cmpUint(s1.parentSymbol, s2.parentSymbol)
		}
		if s1.parseState != s2.parseState {
			return cmpUint(s1.parseState, s2.parseState)
		}
		if s1.fieldID != s2.fieldID {
			return cmpUint(s1.fieldID, s2.fieldID)
		}
	}
	return cmpUint(a.stepIndex, b.stepIndex)
}

// top is analysis_state__top.
func (s *analysisState) top() *analysisStateEntry {
	if s.depth == 0 {
		return &s.stack[0]
	}
	return &s.stack[s.depth-1]
}

// hasSupertype is analysis_state__has_supertype.
func (s *analysisState) hasSupertype(symbol Symbol) bool {
	for i := uint16(0); i < s.depth; i++ {
		if s.stack[i].parentSymbol == symbol {
			return true
		}
	}
	return false
}

// analysisStateSet is an AnalysisStateSet: a sorted set of states, with a pool that reuses released states.
type analysisStateSet []*analysisState

// cloneOrReuse is analysis_state_pool__clone_or_reuse.
func (pool *analysisStateSet) cloneOrReuse(borrowed *analysisState) *analysisState {
	var item *analysisState
	if n := len(*pool); n > 0 {
		item = (*pool)[n-1]
		*pool = (*pool)[:n-1]
	} else {
		item = new(analysisState)
	}
	*item = *borrowed
	return item
}

// insertSorted is analysis_state_set__insert_sorted.
func (set *analysisStateSet) insertSorted(pool *analysisStateSet, borrowed *analysisState) {
	index, exists := searchSorted(len(*set), func(i int) int {
		return compareAnalysisStates((*set)[i], borrowed)
	})
	if !exists {
		item := pool.cloneOrReuse(borrowed)
		*set = append(*set, nil)
		copy((*set)[index+1:], (*set)[index:])
		(*set)[index] = item
	}
}

// push is analysis_state_set__push.
func (set *analysisStateSet) push(pool *analysisStateSet, borrowed *analysisState) {
	*set = append(*set, pool.cloneOrReuse(borrowed))
}

// clear is analysis_state_set__clear.
func (set *analysisStateSet) clear(pool *analysisStateSet) {
	*pool = append(*pool, *set...)
	*set = (*set)[:0]
}

// queryAnalysis is a QueryAnalysis.
type queryAnalysis struct {
	states                analysisStateSet
	nextStates            analysisStateSet
	deeperStates          analysisStateSet
	statePool             analysisStateSet
	finalStepIndices      []uint16
	finishedParentSymbols []uint16
	didAbort              bool
}

// analysisSubgraphNode is an AnalysisSubgraphNode.
type analysisSubgraphNode struct {
	state        StateID
	productionID uint16
	childIndex   uint8
	done         bool
}

// compareSubgraphNodes is analysis_subgraph_node__compare.
func compareSubgraphNodes(a, b *analysisSubgraphNode) int {
	if a.state != b.state {
		return cmpUint(a.state, b.state)
	}
	if a.childIndex != b.childIndex {
		return cmpUint(uint16(a.childIndex), uint16(b.childIndex))
	}
	if a.done != b.done {
		if !a.done {
			return -1
		}
		return 1
	}
	return cmpUint(a.productionID, b.productionID)
}

// analysisSubgraph is an AnalysisSubgraph: the parse states that build nodes
// of one symbol.
type analysisSubgraph struct {
	symbol      Symbol
	startStates []StateID
	nodes       []analysisSubgraphNode
}

// searchSubgraphNodes is array_search_sorted_with over a subgraph's nodes.
func searchSubgraphNodes(nodes []analysisSubgraphNode, needle *analysisSubgraphNode) (int, bool) {
	return searchSorted(len(nodes), func(i int) int { return compareSubgraphNodes(&nodes[i], needle) })
}

// searchSubgraphs is array_search_sorted_by over subgraphs by symbol.
func searchSubgraphs(subgraphs []analysisSubgraph, symbol Symbol) (int, bool) {
	return searchSorted(len(subgraphs), func(i int) int { return cmpUint(subgraphs[i].symbol, symbol) })
}

// insertSubgraph is array_insert_sorted_by over subgraphs by symbol.
func insertSubgraph(subgraphs *[]analysisSubgraph, symbol Symbol) {
	index, exists := searchSubgraphs(*subgraphs, symbol)
	if !exists {
		*subgraphs = append(*subgraphs, analysisSubgraph{})
		copy((*subgraphs)[index+1:], (*subgraphs)[index:])
		(*subgraphs)[index] = analysisSubgraph{symbol: symbol}
	}
}
