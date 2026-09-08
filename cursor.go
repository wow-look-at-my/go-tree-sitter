package treesitter

type cursorEntry struct {
	node       Node
	childIndex uint32
}

// TreeCursor walks a syntax tree without allocating for each step.
type TreeCursor struct {
	stack []cursorEntry
}

// Walk returns a cursor positioned on the node.
func (n Node) Walk() *TreeCursor {
	return &TreeCursor{stack: []cursorEntry{{node: n}}}
}

// Walk returns a cursor positioned on the tree's root.
func (t *Tree) Walk() *TreeCursor { return t.RootNode().Walk() }

// Node reports the node the cursor sits on.
func (c *TreeCursor) Node() Node { return c.stack[len(c.stack)-1].node }

// Reset moves the cursor to the given node and forgets its history.
func (c *TreeCursor) Reset(n Node) {
	c.stack = append(c.stack[:0], cursorEntry{node: n})
}

// Copy returns an independent cursor at the same position.
func (c *TreeCursor) Copy() *TreeCursor {
	return &TreeCursor{stack: append([]cursorEntry(nil), c.stack...)}
}

// GotoFirstChild moves to the first visible child, reporting whether one exists.
func (c *TreeCursor) GotoFirstChild() bool {
	current := c.Node()
	child := current.Child(0)
	if child.IsNull() {
		return false
	}
	c.stack = append(c.stack, cursorEntry{node: child})
	return true
}

// GotoNextSibling moves to the next visible sibling, reporting whether one exists.
func (c *TreeCursor) GotoNextSibling() bool {
	if len(c.stack) < 2 {
		return false
	}
	entry := &c.stack[len(c.stack)-1]
	parent := c.stack[len(c.stack)-2].node
	next := parent.Child(entry.childIndex + 1)
	if next.IsNull() {
		return false
	}
	entry.childIndex++
	entry.node = next
	return true
}

// GotoPreviousSibling moves to the previous visible sibling.
func (c *TreeCursor) GotoPreviousSibling() bool {
	if len(c.stack) < 2 {
		return false
	}
	entry := &c.stack[len(c.stack)-1]
	if entry.childIndex == 0 {
		return false
	}
	parent := c.stack[len(c.stack)-2].node
	prev := parent.Child(entry.childIndex - 1)
	if prev.IsNull() {
		return false
	}
	entry.childIndex--
	entry.node = prev
	return true
}

// GotoParent moves to the parent node, reporting whether one exists.
func (c *TreeCursor) GotoParent() bool {
	if len(c.stack) < 2 {
		return false
	}
	c.stack = c.stack[:len(c.stack)-1]
	return true
}

// FieldName reports the field the current node fills in its parent.
func (c *TreeCursor) FieldName() string {
	if len(c.stack) < 2 {
		return ""
	}
	parent := c.stack[len(c.stack)-2].node
	return parent.FieldNameForChild(c.stack[len(c.stack)-1].childIndex)
}

// Depth reports how far the cursor has descended from its starting node.
func (c *TreeCursor) Depth() uint32 { return uint32(len(c.stack) - 1) }
