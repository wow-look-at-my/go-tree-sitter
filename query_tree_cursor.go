// query_tree_cursor.go ports the parts of lib/src/tree_cursor.c (v0.27.2)
// that the query cursor walks the tree with. Unlike TreeCursor, it visits
// hidden nodes too: the query cursor counts them to keep its depth right.

package treesitter

// treeCursorEntry is a treeCursorEntry: one level of the cursor's stack.
type treeCursorEntry struct {
	subtree              *subtree
	position             length
	childIndex           uint32
	structuralChildIndex uint32
	descendantIndex      uint32
}

// treeCursorStep is a treeCursorStep.
type treeCursorStep uint8

// The outcomes of an internal step.
const (
	treeCursorStepNone treeCursorStep = iota
	treeCursorStepHidden
	treeCursorStepVisible
)

// queryTreeCursor is a TreeCursor. The stack is reused across resets, so a
// walk allocates nothing once it has reached its deepest level.
type queryTreeCursor struct {
	tree            *Tree
	stack           []treeCursorEntry
	rootAliasSymbol Symbol
}

// cursorChildIterator is a cursorChildIterator.
type cursorChildIterator struct {
	parent               subtree
	tree                 *Tree
	position             length
	childIndex           uint32
	structuralChildIndex uint32
	descendantIndex      uint32
	aliasSequence        []Symbol
	valid                bool
}

// reset is ts_tree_cursor_init.
func (c *queryTreeCursor) reset(node Node) {
	c.tree = node.tree
	c.rootAliasSymbol = node.alias
	c.stack = append(c.stack[:0], treeCursorEntry{
		subtree:  node.id,
		position: length{Bytes: node.startByte, Extent: node.startPoint},
	})
}

// isEntryVisible is ts_tree_cursor_is_entry_visible.
func (c *queryTreeCursor) isEntryVisible(index int) bool {
	entry := &c.stack[index]
	switch {
	case index == 0 || subtreeVisible(*entry.subtree):
		return true
	case !subtreeExtra(*entry.subtree):
		parentEntry := &c.stack[index-1]
		return c.tree.language.AliasAt(
			uint32(subtreeProductionID(*parentEntry.subtree)),
			entry.structuralChildIndex,
		) != 0
	default:
		return false
	}
}

// iterateChildren is ts_tree_cursor_iterate_children.
func (c *queryTreeCursor) iterateChildren() cursorChildIterator {
	lastEntry := &c.stack[len(c.stack)-1]
	if subtreeChildCount(*lastEntry.subtree) == 0 {
		return cursorChildIterator{tree: c.tree}
	}
	descendantIndex := lastEntry.descendantIndex
	if c.isEntryVisible(len(c.stack) - 1) {
		descendantIndex++
	}
	return cursorChildIterator{
		tree:            c.tree,
		parent:          *lastEntry.subtree,
		position:        lastEntry.position,
		descendantIndex: descendantIndex,
		aliasSequence:   c.tree.language.aliasSequence(uint32(subtreeProductionID(*lastEntry.subtree))),
		valid:           true,
	}
}

// next is ts_tree_cursor_child_iterator_next.
func (it *cursorChildIterator) next(result *treeCursorEntry) (visible, ok bool) {
	if !it.valid || it.childIndex == subtreeChildCount(it.parent) {
		return false, false
	}
	children := subtreeChildren(it.parent)
	child := &children[it.childIndex]
	*result = treeCursorEntry{
		subtree:              child,
		position:             it.position,
		childIndex:           it.childIndex,
		structuralChildIndex: it.structuralChildIndex,
		descendantIndex:      it.descendantIndex,
	}
	visible = subtreeVisible(*child)
	if !subtreeExtra(*child) {
		if it.aliasSequence != nil && it.aliasSequence[it.structuralChildIndex] != 0 {
			visible = true
		}
		it.structuralChildIndex++
	}

	it.descendantIndex += subtreeVisibleDescendantCount(*child)
	if visible {
		it.descendantIndex++
	}

	it.position = lengthAdd(it.position, subtreeSize(*child))
	it.childIndex++

	if it.childIndex < uint32(len(children)) {
		it.position = lengthAdd(it.position, subtreePadding(children[it.childIndex]))
	}
	return visible, true
}

// gotoFirstChildInternal is ts_tree_cursor_goto_first_child_internal.
func (c *queryTreeCursor) gotoFirstChildInternal() treeCursorStep {
	var entry treeCursorEntry
	it := c.iterateChildren()
	for {
		visible, ok := it.next(&entry)
		if !ok {
			break
		}
		if visible {
			c.stack = append(c.stack, entry)
			return treeCursorStepVisible
		}
		if subtreeVisibleChildCount(*entry.subtree) > 0 {
			c.stack = append(c.stack, entry)
			return treeCursorStepHidden
		}
	}
	return treeCursorStepNone
}

// gotoNextSiblingInternal is ts_tree_cursor_goto_next_sibling_internal.
func (c *queryTreeCursor) gotoNextSiblingInternal() treeCursorStep {
	initialSize := len(c.stack)

	for len(c.stack) > 1 {
		entry := c.stack[len(c.stack)-1]
		c.stack = c.stack[:len(c.stack)-1]
		it := c.iterateChildren()
		it.childIndex = entry.childIndex
		it.structuralChildIndex = entry.structuralChildIndex
		it.position = entry.position
		it.descendantIndex = entry.descendantIndex

		visible, _ := it.next(&entry)
		if visible && len(c.stack)+1 < initialSize {
			break
		}

		for {
			visible, ok := it.next(&entry)
			if !ok {
				break
			}
			if visible {
				c.stack = append(c.stack, entry)
				return treeCursorStepVisible
			}
			if subtreeVisibleChildCount(*entry.subtree) > 0 {
				c.stack = append(c.stack, entry)
				return treeCursorStepHidden
			}
		}
	}

	c.stack = c.stack[:initialSize]
	return treeCursorStepNone
}

// gotoParent is ts_tree_cursor_goto_parent.
func (c *queryTreeCursor) gotoParent() bool {
	for i := len(c.stack) - 2; i >= 0; i-- {
		if c.isEntryVisible(i) {
			c.stack = c.stack[:i+1]
			return true
		}
	}
	return false
}

// currentSubtree is ts_tree_cursor_current_subtree.
func (c *queryTreeCursor) currentSubtree() subtree {
	return *c.stack[len(c.stack)-1].subtree
}

// currentNode is ts_tree_cursor_current_node.
func (c *queryTreeCursor) currentNode() Node {
	lastEntry := &c.stack[len(c.stack)-1]
	isExtra := subtreeExtra(*lastEntry.subtree)
	var aliasSymbol Symbol
	if !isExtra {
		aliasSymbol = c.rootAliasSymbol
	}
	if len(c.stack) > 1 && !isExtra {
		parentEntry := &c.stack[len(c.stack)-2]
		aliasSymbol = c.tree.language.AliasAt(
			uint32(subtreeProductionID(*parentEntry.subtree)),
			lastEntry.structuralChildIndex,
		)
	}
	return newNode(c.tree, lastEntry.subtree, lastEntry.position, aliasSymbol)
}

// parentNode is ts_tree_cursor_parent_node.
func (c *queryTreeCursor) parentNode() Node {
	for i := len(c.stack) - 2; i >= 0; i-- {
		entry := &c.stack[i]
		isVisible := true
		var aliasSymbol Symbol
		if i > 0 {
			parentEntry := &c.stack[i-1]
			aliasSymbol = c.tree.language.AliasAt(
				uint32(subtreeProductionID(*parentEntry.subtree)),
				entry.structuralChildIndex,
			)
			isVisible = aliasSymbol != 0 || subtreeVisible(*entry.subtree)
		}
		if isVisible {
			return newNode(c.tree, entry.subtree, entry.position, aliasSymbol)
		}
	}
	return nullNode()
}

// cursorStatus is what ts_tree_cursor_current_status reports.
type cursorStatus struct {
	fieldID                           FieldID
	hasLaterSiblings                  bool
	hasLaterNamedSiblings             bool
	canHaveLaterSiblingsWithThisField bool
	supertypes                        [8]Symbol
	supertypeCount                    int
}

// currentStatus is ts_tree_cursor_current_status: the facts about the current
// node that query execution needs. It walks the hidden ancestors too, because
// a field can reach a node through a hidden wrapper.
func (c *queryTreeCursor) currentStatus(status *cursorStatus) {
	language := c.tree.language
	status.fieldID = 0
	status.supertypeCount = 0
	status.hasLaterSiblings = false
	status.hasLaterNamedSiblings = false
	status.canHaveLaterSiblingsWithThisField = false

	for i := len(c.stack) - 1; i > 0; i-- {
		entry := &c.stack[i]
		parentEntry := &c.stack[i-1]
		aliasSequence := language.aliasSequence(uint32(subtreeProductionID(*parentEntry.subtree)))

		symbolOf := func(sub subtree, structuralChildIndex uint32) Symbol {
			if !subtreeExtra(sub) && aliasSequence != nil && aliasSequence[structuralChildIndex] != 0 {
				return aliasSequence[structuralChildIndex]
			}
			return subtreeSymbol(sub)
		}

		entrySymbol := symbolOf(*entry.subtree, entry.structuralChildIndex)
		entryMetadata := language.symbolMetadata(entrySymbol)
		if i != len(c.stack)-1 && entryMetadata.Visible {
			break
		}

		if entryMetadata.Supertype && status.supertypeCount < len(status.supertypes) {
			status.supertypes[status.supertypeCount] = entrySymbol
			status.supertypeCount++
		}

		// A later anonymous sibling settles hasLaterSiblings and says nothing
		// about named ones.
		if !status.hasLaterNamedSiblings {
			siblings := subtreeChildren(*parentEntry.subtree)
			structuralChildIndex := entry.structuralChildIndex
			if !subtreeExtra(*entry.subtree) {
				structuralChildIndex++
			}
			for j := entry.childIndex + 1; j < uint32(len(siblings)); j++ {
				sibling := siblings[j]
				siblingMetadata := language.symbolMetadata(symbolOf(sibling, structuralChildIndex))
				if siblingMetadata.Visible {
					status.hasLaterSiblings = true
					if siblingMetadata.Named {
						status.hasLaterNamedSiblings = true
						break
					}
				} else if subtreeVisibleChildCount(sibling) > 0 {
					status.hasLaterSiblings = true
					if subtreeNamedChildCount(sibling) > 0 {
						status.hasLaterNamedSiblings = true
						break
					}
				}
				if !subtreeExtra(sibling) {
					structuralChildIndex++
				}
			}
		}

		if !subtreeExtra(*entry.subtree) {
			fieldMap := language.fieldMap(uint32(subtreeProductionID(*parentEntry.subtree)))
			if status.fieldID == 0 {
				for _, m := range fieldMap {
					if !m.Inherited && uint32(m.ChildIndex) == entry.structuralChildIndex {
						status.fieldID = m.FieldID
						break
					}
				}
			}
			if status.fieldID != 0 {
				for _, m := range fieldMap {
					if m.FieldID == status.fieldID && uint32(m.ChildIndex) > entry.structuralChildIndex {
						status.canHaveLaterSiblingsWithThisField = true
						break
					}
				}
			}
		}
	}
}
