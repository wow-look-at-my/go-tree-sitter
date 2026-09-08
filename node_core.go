package treesitter

// Node is a single node in a syntax tree.
type Node struct {
	startByte  uint32
	startPoint Point
	alias      Symbol
	id         *subtree
	tree       *Tree
}

func newNode(tree *Tree, sub *subtree, position length, alias Symbol) Node {
	return Node{
		startByte:  position.Bytes,
		startPoint: position.Extent,
		alias:      alias,
		id:         sub,
		tree:       tree,
	}
}

func nullNode() Node { return Node{} }

func (n Node) subtree() subtree { return *n.id }

type nodeChildIterator struct {
	parent               subtree
	tree                 *Tree
	position             length
	childIndex           uint32
	structuralChildIndex uint32
	aliasSequence        []Symbol
}

func (n *Node) iterateChildren() nodeChildIterator {
	sub := n.subtree()
	if subtreeChildCount(sub) == 0 {
		return nodeChildIterator{tree: n.tree}
	}
	return nodeChildIterator{
		tree:          n.tree,
		parent:        sub,
		position:      length{Bytes: n.startByte, Extent: n.startPoint},
		aliasSequence: n.tree.language.aliasSequence(uint32(subtreeProductionID(sub))),
	}
}

func (it *nodeChildIterator) done() bool {
	return it.childIndex == subtreeChildCount(it.parent)
}

func (it *nodeChildIterator) next(result *Node) bool {
	if it.parent.isNil() || it.done() {
		return false
	}
	child := &subtreeChildren(it.parent)[it.childIndex]
	var aliasSymbol Symbol
	if !subtreeExtra(*child) {
		if it.aliasSequence != nil {
			aliasSymbol = it.aliasSequence[it.structuralChildIndex]
		}
		it.structuralChildIndex++
	}
	if it.childIndex > 0 {
		it.position = lengthAdd(it.position, subtreePadding(*child))
	}
	*result = newNode(it.tree, child, it.position, aliasSymbol)
	it.position = lengthAdd(it.position, subtreeSize(*child))
	it.childIndex++
	return true
}

func (n Node) isRelevant(includeAnonymous bool) bool {
	tree := n.subtree()
	if includeAnonymous {
		return subtreeVisible(tree) || n.alias != 0
	}
	if n.alias != 0 {
		return n.tree.language.symbolMetadata(n.alias).Named
	}
	return subtreeVisible(tree) && subtreeNamed(tree)
}

func (n Node) relevantChildCount(includeAnonymous bool) uint32 {
	tree := n.subtree()
	if subtreeChildCount(tree) > 0 {
		if includeAnonymous {
			return subtreeVisibleChildCount(tree)
		}
		return subtreeNamedChildCount(tree)
	}
	return 0
}

// IsNull reports whether the node refers to nothing.
func (n Node) IsNull() bool { return n.id == nil }

// StartByte reports the node's first byte offset.
func (n Node) StartByte() uint32 { return n.startByte }

// StartPoint reports the node's first row and column.
func (n Node) StartPoint() Point { return n.startPoint }

// EndByte reports the offset just past the node.
func (n Node) EndByte() uint32 {
	return n.startByte + subtreeSize(n.subtree()).Bytes
}

// EndPoint reports the row and column just past the node.
func (n Node) EndPoint() Point {
	return pointAdd(n.startPoint, subtreeSize(n.subtree()).Extent)
}

// Symbol reports the node's public grammar symbol. A null node has none.
func (n Node) Symbol() Symbol {
	if n.IsNull() {
		return 0
	}
	symbol := n.alias
	if symbol == 0 {
		symbol = subtreeSymbol(n.subtree())
	}
	return n.tree.language.publicSymbol(symbol)
}

// Type reports the node's grammar rule name. A null node has none.
func (n Node) Type() string {
	if n.IsNull() {
		return ""
	}
	symbol := n.alias
	if symbol == 0 {
		symbol = subtreeSymbol(n.subtree())
	}
	return n.tree.language.SymbolName(symbol)
}

// GrammarSymbol reports the node's symbol before alias resolution.
func (n Node) GrammarSymbol() Symbol { return subtreeSymbol(n.subtree()) }

// GrammarType reports the node's rule name before alias resolution.
func (n Node) GrammarType() string {
	return n.tree.language.SymbolName(subtreeSymbol(n.subtree()))
}

// Tree reports the tree that owns the node.
func (n Node) Tree() *Tree { return n.tree }

// String renders the node as an S expression.
func (n Node) String() string {
	if n.IsNull() {
		return "(NULL)"
	}
	return subtreeString(
		n.subtree(), n.alias,
		n.tree.language.symbolMetadata(n.alias).Visible,
		n.tree.language, false,
	)
}

// Equal reports whether two nodes refer to the same place in the same tree.
func (n Node) Equal(other Node) bool {
	return n.tree == other.tree && n.id == other.id
}

// IsExtra reports whether the node sits outside the grammar's main rules.
func (n Node) IsExtra() bool { return subtreeExtra(n.subtree()) }

// IsNamed reports whether the node has a name in the grammar.
func (n Node) IsNamed() bool {
	if n.alias != 0 {
		return n.tree.language.symbolMetadata(n.alias).Named
	}
	return subtreeNamed(n.subtree())
}

// IsMissing reports whether the parser inserted the node during recovery.
func (n Node) IsMissing() bool { return subtreeMissing(n.subtree()) }

// HasChanges reports whether an edit touched the node.
func (n Node) HasChanges() bool { return subtreeHasChanges(n.subtree()) }

// HasError reports whether the node or a descendant is an error.
func (n Node) HasError() bool { return subtreeErrorCost(n.subtree()) > 0 }

// IsError reports whether the node itself is an error node.
func (n Node) IsError() bool { return n.Symbol() == BuiltinSymError }

// DescendantCount reports how many visible nodes the subtree holds.
func (n Node) DescendantCount() uint32 {
	return subtreeVisibleDescendantCount(n.subtree()) + 1
}

// ParseState reports the parse state the node was created in.
func (n Node) ParseState() StateID { return subtreeParseState(n.subtree()) }

// NextParseState reports the state reached after the node.
func (n Node) NextParseState() StateID {
	state := n.ParseState()
	if state == treeStateNone {
		return treeStateNone
	}
	return n.tree.language.nextState(state, n.GrammarSymbol())
}

// ChildCount reports the number of visible children.
func (n Node) ChildCount() uint32 {
	tree := n.subtree()
	if subtreeChildCount(tree) > 0 {
		return subtreeVisibleChildCount(tree)
	}
	return 0
}

// NamedChildCount reports the number of named children.
func (n Node) NamedChildCount() uint32 {
	tree := n.subtree()
	if subtreeChildCount(tree) > 0 {
		return subtreeNamedChildCount(tree)
	}
	return 0
}

func (n Node) child(childIndex uint32, includeAnonymous bool) Node {
	result := n
	didDescend := true

	for didDescend {
		didDescend = false

		var child Node
		index := uint32(0)
		iterator := result.iterateChildren()
		for iterator.next(&child) {
			if child.isRelevant(includeAnonymous) {
				if index == childIndex {
					return child
				}
				index++
			} else {
				grandchildIndex := childIndex - index
				grandchildCount := child.relevantChildCount(includeAnonymous)
				if grandchildIndex < grandchildCount {
					didDescend = true
					result = child
					childIndex = grandchildIndex
					break
				}
				index += grandchildCount
			}
		}
	}

	return nullNode()
}

// Child returns the visible child at the given index.
func (n Node) Child(index uint32) Node { return n.child(index, true) }

// NamedChild returns the named child at the given index.
func (n Node) NamedChild(index uint32) Node { return n.child(index, false) }

// Parent returns the node that contains this one.
func (n Node) Parent() Node {
	node := n.tree.RootNode()
	if node.id == n.id {
		return nullNode()
	}
	for {
		nextNode := node.ChildWithDescendant(n)
		if nextNode.id == n.id || nextNode.IsNull() {
			break
		}
		node = nextNode
	}
	return node
}

// ChildWithDescendant returns the child that contains the given descendant.
func (n Node) ChildWithDescendant(descendant Node) Node {
	self := n
	startByte := descendant.StartByte()
	endByte := descendant.EndByte()
	isEmpty := startByte == endByte

	for {
		iter := self.iterateChildren()
		for {
			if !iter.next(&self) || self.StartByte() > startByte {
				return nullNode()
			}
			if self.id == descendant.id {
				return self
			}

			if isEmpty && iter.position.Bytes >= endByte && self.ChildCount() > 0 {
				child := self.ChildWithDescendant(descendant)
				if !child.IsNull() {
					if self.isRelevant(true) {
						return self
					}
					return child
				}
			}

			cont := iter.position.Bytes < endByte
			if isEmpty {
				cont = iter.position.Bytes <= endByte
			}
			if !cont && self.ChildCount() != 0 {
				break
			}
		}
		if self.isRelevant(true) {
			break
		}
	}

	return self
}

func subtreeHasTrailingEmptyDescendant(self, other subtree) bool {
	children := subtreeChildren(self)
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if subtreeTotalBytes(child) > 0 {
			break
		}
		if child == other || subtreeHasTrailingEmptyDescendant(child, other) {
			return true
		}
	}
	return false
}

func (n Node) prevSibling(includeAnonymous bool) Node {
	selfSubtree := n.subtree()
	selfIsEmpty := subtreeTotalBytes(selfSubtree) == 0
	targetEndByte := n.EndByte()

	node := n.Parent()
	earlierNode := nullNode()
	earlierNodeIsRelevant := false

	for !node.IsNull() {
		earlierChild := nullNode()
		earlierChildIsRelevant := false
		foundChildContainingTarget := false

		var child Node
		iterator := node.iterateChildren()
		for iterator.next(&child) {
			if child.id == n.id {
				break
			}
			if iterator.position.Bytes > targetEndByte {
				foundChildContainingTarget = true
				break
			}
			if iterator.position.Bytes == targetEndByte &&
				(!selfIsEmpty ||
					subtreeHasTrailingEmptyDescendant(child.subtree(), selfSubtree)) {
				foundChildContainingTarget = true
				break
			}

			if child.isRelevant(includeAnonymous) {
				earlierChild = child
				earlierChildIsRelevant = true
			} else if child.relevantChildCount(includeAnonymous) > 0 {
				earlierChild = child
				earlierChildIsRelevant = false
			}
		}

		switch {
		case foundChildContainingTarget:
			if !earlierChild.IsNull() {
				earlierNode = earlierChild
				earlierNodeIsRelevant = earlierChildIsRelevant
			}
			node = child
		case earlierChildIsRelevant:
			return earlierChild
		case !earlierChild.IsNull():
			node = earlierChild
		case earlierNodeIsRelevant:
			return earlierNode
		default:
			node = earlierNode
			earlierNode = nullNode()
			earlierNodeIsRelevant = false
		}
	}

	return nullNode()
}

func (n Node) nextSibling(includeAnonymous bool) Node {
	targetEndByte := n.EndByte()

	node := n.Parent()
	laterNode := nullNode()
	laterNodeIsRelevant := false

	for !node.IsNull() {
		laterChild := nullNode()
		laterChildIsRelevant := false
		childContainingTarget := nullNode()

		var child Node
		iterator := node.iterateChildren()
		for iterator.next(&child) {
			if iterator.position.Bytes <= targetEndByte {
				continue
			}
			startByte := n.StartByte()
			childStartByte := child.StartByte()

			isEmpty := startByte == targetEndByte
			var containsTarget bool
			if isEmpty {
				containsTarget = childStartByte < startByte
			} else {
				containsTarget = childStartByte <= startByte
			}

			if containsTarget {
				if child.subtree() != n.subtree() {
					childContainingTarget = child
				}
			} else if child.isRelevant(includeAnonymous) {
				laterChild = child
				laterChildIsRelevant = true
				break
			} else if child.relevantChildCount(includeAnonymous) > 0 {
				laterChild = child
				laterChildIsRelevant = false
				break
			}
		}

		switch {
		case !childContainingTarget.IsNull():
			if !laterChild.IsNull() {
				laterNode = laterChild
				laterNodeIsRelevant = laterChildIsRelevant
			}
			node = childContainingTarget
		case laterChildIsRelevant:
			return laterChild
		case !laterChild.IsNull():
			node = laterChild
		case laterNodeIsRelevant:
			return laterNode
		default:
			node = laterNode
		}
	}

	return nullNode()
}

// NextSibling returns the next visible node with the same parent.
func (n Node) NextSibling() Node { return n.nextSibling(true) }

// NextNamedSibling returns the next named node with the same parent.
func (n Node) NextNamedSibling() Node { return n.nextSibling(false) }

// PrevSibling returns the previous visible node with the same parent.
func (n Node) PrevSibling() Node { return n.prevSibling(true) }

// PrevNamedSibling returns the previous named node with the same parent.
func (n Node) PrevNamedSibling() Node { return n.prevSibling(false) }
