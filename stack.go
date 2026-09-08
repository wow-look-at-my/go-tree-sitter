package treesitter

const maxIteratorCount = 64

type stackVersion = uint32

const stackVersionNone stackVersion = ^uint32(0)

type stackLink struct {
	node      *stackNode
	subtree   subtree
	isPending bool
}

type stackNode struct {
	state             StateID
	position          length
	links             []stackLink
	refCount          uint32
	errorCost         uint32
	nodeCount         uint32
	dynamicPrecedence int32
}

type stackIterator struct {
	node *stackNode
	// Never read len(subtrees) for subtreeCount: it counts extras as symbols.
	subtrees     []subtree
	subtreeCount uint32
	isPending    bool
}

// StackSlice is one path revealed by a pop operation.
type stackSlice struct {
	subtrees []subtree
	version  stackVersion
}

type stackSummaryEntry struct {
	position length
	depth    uint32
	state    StateID
}

type stackStatus int

const (
	stackStatusActive stackStatus = iota
	stackStatusPaused
	stackStatusHalted
)

type stackHead struct {
	node                 *stackNode
	summary              *[]stackSummaryEntry
	nodeCountAtLastError uint32
	lastExternalToken    subtree
	lookaheadWhenPaused  subtree
	status               stackStatus
}

type parseStack struct {
	heads     []stackHead
	slices    []stackSlice
	iterators []stackIterator
	baseNode  *stackNode
}

type stackAction uint

const (
	stackActionNone stackAction = 0
	stackActionStop stackAction = 1
	stackActionPop  stackAction = 2
)

type stackCallback func(iterator *stackIterator) stackAction

func stackNodeRetain(self *stackNode) {
	if self == nil {
		return
	}
	self.refCount++
}

func stackNodeRelease(self *stackNode) {
	for {
		self.refCount--
		if self.refCount > 0 {
			return
		}

		var firstPredecessor *stackNode
		if len(self.links) > 0 {
			for i := len(self.links) - 1; i > 0; i-- {
				link := self.links[i]
				if !link.subtree.isNil() {
					subtreeRelease(link.subtree)
				}
				stackNodeRelease(link.node)
			}
			link := self.links[0]
			if !link.subtree.isNil() {
				subtreeRelease(link.subtree)
			}
			firstPredecessor = self.links[0].node
		}

		if firstPredecessor == nil {
			return
		}
		self = firstPredecessor
	}
}

func stackSubtreeNodeCount(s subtree) uint32 {
	count := subtreeVisibleDescendantCount(s)
	if subtreeVisible(s) {
		count++
	}
	if subtreeSymbol(s) == builtinSymErrorRepeat {
		count++
	}
	return count
}

func stackNodeNew(previousNode *stackNode, s subtree, isPending bool, state StateID) *stackNode {
	node := &stackNode{refCount: 1, state: state}

	if previousNode != nil {
		node.links = []stackLink{{node: previousNode, subtree: s, isPending: isPending}}

		node.position = previousNode.position
		node.errorCost = previousNode.errorCost
		node.dynamicPrecedence = previousNode.dynamicPrecedence
		node.nodeCount = previousNode.nodeCount

		if !s.isNil() {
			node.errorCost += subtreeErrorCost(s)
			node.position = lengthAdd(node.position, subtreeTotalSize(s))
			node.nodeCount += stackSubtreeNodeCount(s)
			node.dynamicPrecedence += subtreeDynamicPrecedence(s)
		}
	} else {
		node.position = lengthZero()
		node.errorCost = 0
	}

	return node
}

func stackSubtreeIsEquivalent(left, right subtree) bool {
	if left == right {
		return true
	}
	if left.isNil() || right.isNil() {
		return false
	}
	if subtreeSymbol(left) != subtreeSymbol(right) {
		return false
	}
	if subtreeErrorCost(left) > 0 && subtreeErrorCost(right) > 0 {
		return true
	}
	return subtreePadding(left).Bytes == subtreePadding(right).Bytes &&
		subtreeSize(left).Bytes == subtreeSize(right).Bytes &&
		subtreeChildCount(left) == subtreeChildCount(right) &&
		subtreeExtra(left) == subtreeExtra(right) &&
		subtreeExternalScannerStateEq(left, right)
}

func stackNodeAddLink(self *stackNode, link stackLink) {
	if link.node == self {
		return
	}

	for i := range self.links {
		existingLink := &self.links[i]
		if stackSubtreeIsEquivalent(existingLink.subtree, link.subtree) {
			if existingLink.node == link.node {
				if subtreeDynamicPrecedence(link.subtree) > subtreeDynamicPrecedence(existingLink.subtree) {
					subtreeRetain(link.subtree)
					subtreeRelease(existingLink.subtree)
					existingLink.subtree = link.subtree
					self.dynamicPrecedence = link.node.dynamicPrecedence +
						subtreeDynamicPrecedence(link.subtree)
				}
				return
			}

			if existingLink.node.state == link.node.state &&
				existingLink.node.position.Bytes == link.node.position.Bytes &&
				existingLink.node.errorCost == link.node.errorCost {
				for j := range link.node.links {
					stackNodeAddLink(existingLink.node, link.node.links[j])
				}
				dynamicPrecedence := link.node.dynamicPrecedence
				if !link.subtree.isNil() {
					dynamicPrecedence += subtreeDynamicPrecedence(link.subtree)
				}
				if dynamicPrecedence > self.dynamicPrecedence {
					self.dynamicPrecedence = dynamicPrecedence
				}
				return
			}
		}
	}

	stackNodeRetain(link.node)
	nodeCount := link.node.nodeCount
	dynamicPrecedence := link.node.dynamicPrecedence
	self.links = append(self.links, link)

	if !link.subtree.isNil() {
		subtreeRetain(link.subtree)
		nodeCount += stackSubtreeNodeCount(link.subtree)
		dynamicPrecedence += subtreeDynamicPrecedence(link.subtree)
	}

	if nodeCount > self.nodeCount {
		self.nodeCount = nodeCount
	}
	if dynamicPrecedence > self.dynamicPrecedence {
		self.dynamicPrecedence = dynamicPrecedence
	}
}

func stackHeadDelete(self *stackHead) {
	if self.node != nil {
		if !self.lastExternalToken.isNil() {
			subtreeRelease(self.lastExternalToken)
		}
		if !self.lookaheadWhenPaused.isNil() {
			subtreeRelease(self.lookaheadWhenPaused)
		}
		self.summary = nil
		stackNodeRelease(self.node)
	}
}

func (s *parseStack) addVersion(originalVersion stackVersion, node *stackNode) stackVersion {
	head := stackHead{
		node:                 node,
		nodeCountAtLastError: s.heads[originalVersion].nodeCountAtLastError,
		lastExternalToken:    s.heads[originalVersion].lastExternalToken,
		status:               stackStatusActive,
	}
	s.heads = append(s.heads, head)
	stackNodeRetain(node)
	if !head.lastExternalToken.isNil() {
		subtreeRetain(head.lastExternalToken)
	}
	return stackVersion(len(s.heads) - 1)
}

func (s *parseStack) addSlice(originalVersion stackVersion, node *stackNode, subtrees []subtree) {
	for i := len(s.slices) - 1; i >= 0; i-- {
		version := s.slices[i].version
		if s.heads[version].node == node {
			slice := stackSlice{subtrees: subtrees, version: version}
			s.slices = append(s.slices, stackSlice{})
			copy(s.slices[i+2:], s.slices[i+1:])
			s.slices[i+1] = slice
			return
		}
	}

	version := s.addVersion(originalVersion, node)
	s.slices = append(s.slices, stackSlice{subtrees: subtrees, version: version})
}

func (s *parseStack) iter(
	version stackVersion, callback stackCallback, goalSubtreeCount int,
) []stackSlice {
	s.slices = s.slices[:0]
	s.iterators = s.iterators[:0]

	head := &s.heads[version]
	newIterator := stackIterator{node: head.node, isPending: true}

	includeSubtrees := goalSubtreeCount >= 0

	s.iterators = append(s.iterators, newIterator)

	for len(s.iterators) > 0 {
		size := len(s.iterators)
		for i := 0; i < size; i++ {
			iterator := &s.iterators[i]
			node := iterator.node

			action := callback(iterator)
			shouldPop := action&stackActionPop != 0
			shouldStop := action&stackActionStop != 0 || len(node.links) == 0

			if shouldPop {
				subtrees := iterator.subtrees
				if !shouldStop {
					subtrees = subtreeArrayCopy(subtrees)
				}
				subtreeArrayReverse(subtrees)
				s.addSlice(version, node, subtrees)
			}

			if shouldStop {
				if !shouldPop {
					subtreeArrayClear(&s.iterators[i].subtrees)
				}
				s.iterators = append(s.iterators[:i], s.iterators[i+1:]...)
				i--
				size--
				continue
			}

			for j := 1; j <= len(node.links); j++ {
				var nextIterator *stackIterator
				var link stackLink
				if j == len(node.links) {
					link = node.links[0]
					nextIterator = &s.iterators[i]
				} else {
					if len(s.iterators) >= maxIteratorCount {
						continue
					}
					link = node.links[j]
					currentIterator := s.iterators[i]
					currentIterator.subtrees = subtreeArrayCopy(currentIterator.subtrees)
					s.iterators = append(s.iterators, currentIterator)
					nextIterator = &s.iterators[len(s.iterators)-1]
				}

				nextIterator.node = link.node
				if !link.subtree.isNil() {
					if includeSubtrees {
						nextIterator.subtrees = append(nextIterator.subtrees, link.subtree)
						subtreeRetain(link.subtree)
					}

					if !subtreeExtra(link.subtree) {
						nextIterator.subtreeCount++
						if !link.isPending {
							nextIterator.isPending = false
						}
					}
				} else {
					nextIterator.subtreeCount++
					nextIterator.isPending = false
				}
			}
		}
	}

	return s.slices
}

func newParseStack() *parseStack {
	self := &parseStack{}
	self.baseNode = stackNodeNew(nil, subtree{}, false, 1)
	self.clear()
	return self
}

func (s *parseStack) versionCount() uint32 { return uint32(len(s.heads)) }

func (s *parseStack) haltedVersionCount() uint32 {
	count := uint32(0)
	for i := range s.heads {
		if s.heads[i].status == stackStatusHalted {
			count++
		}
	}
	return count
}

func (s *parseStack) state(version stackVersion) StateID {
	return s.heads[version].node.state
}

func (s *parseStack) position(version stackVersion) length {
	return s.heads[version].node.position
}

func (s *parseStack) lastExternalToken(version stackVersion) subtree {
	return s.heads[version].lastExternalToken
}

func (s *parseStack) setLastExternalToken(version stackVersion, token subtree) {
	head := &s.heads[version]
	if !token.isNil() {
		subtreeRetain(token)
	}
	if !head.lastExternalToken.isNil() {
		subtreeRelease(head.lastExternalToken)
	}
	head.lastExternalToken = token
}

func (s *parseStack) errorCost(version stackVersion) uint32 {
	head := &s.heads[version]
	result := head.node.errorCost
	firstLinkIsEmpty := len(head.node.links) == 0 || head.node.links[0].subtree.isNil()
	if head.status == stackStatusPaused ||
		(head.node.state == errorState && firstLinkIsEmpty) {
		result += errorCostPerRecovery
	}
	return result
}

func (s *parseStack) nodeCountSinceError(version stackVersion) uint32 {
	head := &s.heads[version]
	if head.node.nodeCount < head.nodeCountAtLastError {
		head.nodeCountAtLastError = head.node.nodeCount
	}
	return head.node.nodeCount - head.nodeCountAtLastError
}

func (s *parseStack) push(version stackVersion, sub subtree, pending bool, state StateID) {
	head := &s.heads[version]
	newNode := stackNodeNew(head.node, sub, pending, state)
	if sub.isNil() {
		head.nodeCountAtLastError = newNode.nodeCount
	}
	head.node = newNode
}

func (s *parseStack) popCount(version stackVersion, count uint32) []stackSlice {
	return s.iter(version, func(iterator *stackIterator) stackAction {
		if iterator.subtreeCount == count {
			return stackActionPop | stackActionStop
		}
		return stackActionNone
	}, int(count))
}

func (s *parseStack) popPending(version stackVersion) []stackSlice {
	pop := s.iter(version, func(iterator *stackIterator) stackAction {
		if iterator.subtreeCount >= 1 {
			if iterator.isPending {
				return stackActionPop | stackActionStop
			}
			return stackActionStop
		}
		return stackActionNone
	}, 0)
	if len(pop) > 0 {
		s.renumberVersion(pop[0].version, version)
		pop[0].version = version
	}
	return pop
}

func (s *parseStack) popError(version stackVersion) []subtree {
	node := s.heads[version].node
	for i := range node.links {
		if !node.links[i].subtree.isNil() && subtreeIsError(node.links[i].subtree) {
			foundError := false
			pop := s.iter(version, func(iterator *stackIterator) stackAction {
				if len(iterator.subtrees) > 0 {
					if !foundError && subtreeIsError(iterator.subtrees[0]) {
						foundError = true
						return stackActionPop | stackActionStop
					}
					return stackActionStop
				}
				return stackActionNone
			}, 1)
			if len(pop) > 0 {
				s.renumberVersion(pop[0].version, version)
				return pop[0].subtrees
			}
			break
		}
	}
	return nil
}

func (s *parseStack) popAll(version stackVersion) []stackSlice {
	return s.iter(version, func(iterator *stackIterator) stackAction {
		if len(iterator.node.links) == 0 {
			return stackActionPop
		}
		return stackActionNone
	}, 0)
}

func (s *parseStack) recordSummary(version stackVersion, maxDepth uint32) {
	summary := &[]stackSummaryEntry{}
	s.iter(version, func(iterator *stackIterator) stackAction {
		state := iterator.node.state
		depth := iterator.subtreeCount
		if depth > maxDepth {
			return stackActionStop
		}
		for i := len(*summary) - 1; i >= 0; i-- {
			entry := (*summary)[i]
			if entry.depth < depth {
				break
			}
			if entry.depth == depth && entry.state == state {
				return stackActionNone
			}
		}
		*summary = append(*summary, stackSummaryEntry{
			position: iterator.node.position,
			depth:    depth,
			state:    state,
		})
		return stackActionNone
	}, -1)
	s.heads[version].summary = summary
}

func (s *parseStack) getSummary(version stackVersion) *[]stackSummaryEntry {
	return s.heads[version].summary
}

func (s *parseStack) dynamicPrecedence(version stackVersion) int32 {
	return s.heads[version].node.dynamicPrecedence
}

func (s *parseStack) hasAdvancedSinceError(version stackVersion) bool {
	head := &s.heads[version]
	node := head.node
	if node.errorCost == 0 {
		return true
	}
	for node != nil {
		if len(node.links) > 0 {
			sub := node.links[0].subtree
			if !sub.isNil() {
				if subtreeTotalBytes(sub) > 0 {
					return true
				} else if node.nodeCount > head.nodeCountAtLastError &&
					subtreeErrorCost(sub) == 0 {
					node = node.links[0].node
					continue
				}
			}
		}
		break
	}
	return false
}

func (s *parseStack) removeVersion(version stackVersion) {
	stackHeadDelete(&s.heads[version])
	s.heads = append(s.heads[:version], s.heads[version+1:]...)
}

func (s *parseStack) renumberVersion(v1, v2 stackVersion) {
	if v1 == v2 {
		return
	}
	sourceHead := &s.heads[v1]
	targetHead := &s.heads[v2]
	if targetHead.summary != nil && sourceHead.summary == nil {
		sourceHead.summary = targetHead.summary
		targetHead.summary = nil
	}
	stackHeadDelete(targetHead)
	*targetHead = *sourceHead
	s.heads = append(s.heads[:v1], s.heads[v1+1:]...)
}

func (s *parseStack) swapVersions(v1, v2 stackVersion) {
	s.heads[v1], s.heads[v2] = s.heads[v2], s.heads[v1]
}

func (s *parseStack) copyVersion(version stackVersion) stackVersion {
	versionHead := s.heads[version]
	s.heads = append(s.heads, versionHead)
	head := &s.heads[len(s.heads)-1]
	stackNodeRetain(head.node)
	if !head.lastExternalToken.isNil() {
		subtreeRetain(head.lastExternalToken)
	}
	head.summary = nil
	return stackVersion(len(s.heads) - 1)
}

func (s *parseStack) merge(version1, version2 stackVersion) bool {
	if !s.canMerge(version1, version2) {
		return false
	}
	head1 := &s.heads[version1]
	head2 := &s.heads[version2]
	for i := range head2.node.links {
		stackNodeAddLink(head1.node, head2.node.links[i])
	}
	if head1.node.state == errorState {
		head1.nodeCountAtLastError = head1.node.nodeCount
	}
	s.removeVersion(version2)
	return true
}

func (s *parseStack) canMerge(version1, version2 stackVersion) bool {
	head1 := &s.heads[version1]
	head2 := &s.heads[version2]
	return head1.status == stackStatusActive &&
		head2.status == stackStatusActive &&
		head1.node.state == head2.node.state &&
		head1.node.position.Bytes == head2.node.position.Bytes &&
		head1.node.errorCost == head2.node.errorCost &&
		subtreeExternalScannerStateEq(head1.lastExternalToken, head2.lastExternalToken)
}

func (s *parseStack) halt(version stackVersion) {
	s.heads[version].status = stackStatusHalted
}

func (s *parseStack) pause(version stackVersion, lookahead subtree) {
	head := &s.heads[version]
	head.status = stackStatusPaused
	head.lookaheadWhenPaused = lookahead
	head.nodeCountAtLastError = head.node.nodeCount
}

func (s *parseStack) isActive(version stackVersion) bool {
	if version >= stackVersion(len(s.heads)) {
		return false
	}
	return s.heads[version].status == stackStatusActive
}

func (s *parseStack) isHalted(version stackVersion) bool {
	return s.heads[version].status == stackStatusHalted
}

func (s *parseStack) isPaused(version stackVersion) bool {
	return s.heads[version].status == stackStatusPaused
}

func (s *parseStack) resume(version stackVersion) subtree {
	head := &s.heads[version]
	result := head.lookaheadWhenPaused
	head.status = stackStatusActive
	head.lookaheadWhenPaused = subtree{}
	return result
}

func (s *parseStack) clear() {
	stackNodeRetain(s.baseNode)
	for i := range s.heads {
		stackHeadDelete(&s.heads[i])
	}
	s.heads = s.heads[:0]
	s.heads = append(s.heads, stackHead{node: s.baseNode, status: stackStatusActive})
}
