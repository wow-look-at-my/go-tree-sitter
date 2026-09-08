package treesitter

func (n Node) descendantForByteRange(rangeStart, rangeEnd uint32, includeAnonymous bool) Node {
	if rangeStart > rangeEnd {
		return nullNode()
	}
	node := n
	lastVisibleNode := n

	didDescend := true
	for didDescend {
		didDescend = false

		var child Node
		iterator := node.iterateChildren()
		for iterator.next(&child) {
			nodeEnd := iterator.position.Bytes

			if nodeEnd < rangeEnd {
				continue
			}

			isEmpty := child.StartByte() == nodeEnd
			if isEmpty {
				if nodeEnd < rangeStart {
					continue
				}
			} else if nodeEnd <= rangeStart {
				continue
			}

			if rangeStart < child.StartByte() {
				break
			}

			node = child
			if node.isRelevant(includeAnonymous) {
				lastVisibleNode = node
			}
			didDescend = true
			break
		}
	}

	return lastVisibleNode
}

func (n Node) descendantForPointRange(rangeStart, rangeEnd Point, includeAnonymous bool) Node {
	if pointGt(rangeStart, rangeEnd) {
		return nullNode()
	}
	node := n
	lastVisibleNode := n

	didDescend := true
	for didDescend {
		didDescend = false

		var child Node
		iterator := node.iterateChildren()
		for iterator.next(&child) {
			nodeEnd := iterator.position.Extent

			if pointLt(nodeEnd, rangeEnd) {
				continue
			}

			isEmpty := pointEq(child.StartPoint(), nodeEnd)
			if isEmpty {
				if pointLt(nodeEnd, rangeStart) {
					continue
				}
			} else if pointLte(nodeEnd, rangeStart) {
				continue
			}

			if pointLt(rangeStart, child.StartPoint()) {
				break
			}

			node = child
			if node.isRelevant(includeAnonymous) {
				lastVisibleNode = node
			}
			didDescend = true
			break
		}
	}

	return lastVisibleNode
}

// DescendantForByteRange returns the smallest node spanning the byte range.
func (n Node) DescendantForByteRange(start, end uint32) Node {
	return n.descendantForByteRange(start, end, true)
}

// NamedDescendantForByteRange returns the smallest named node spanning the range.
func (n Node) NamedDescendantForByteRange(start, end uint32) Node {
	return n.descendantForByteRange(start, end, false)
}

// DescendantForPointRange returns the smallest node spanning the point range.
func (n Node) DescendantForPointRange(start, end Point) Node {
	return n.descendantForPointRange(start, end, true)
}

// NamedDescendantForPointRange returns the smallest named node spanning the range.
func (n Node) NamedDescendantForPointRange(start, end Point) Node {
	return n.descendantForPointRange(start, end, false)
}

// ChildByFieldID returns the child registered under the given field.
func (n Node) ChildByFieldID(fieldID FieldID) Node {
	self := n
	for {
		if fieldID == 0 || self.ChildCount() == 0 {
			return nullNode()
		}

		fieldMap := self.tree.language.fieldMap(uint32(subtreeProductionID(self.subtree())))
		if len(fieldMap) == 0 {
			return nullNode()
		}

		start := 0
		end := len(fieldMap)
		for fieldMap[start].FieldID < fieldID {
			start++
			if start == end {
				return nullNode()
			}
		}
		for fieldMap[end-1].FieldID > fieldID {
			end--
			if start == end {
				return nullNode()
			}
		}

		var child Node
		iterator := self.iterateChildren()
		recurse := false
		for iterator.next(&child) {
			if subtreeExtra(child.subtree()) {
				continue
			}
			index := iterator.structuralChildIndex - 1
			if index < uint32(fieldMap[start].ChildIndex) {
				continue
			}

			if fieldMap[start].Inherited {
				if start+1 == end {
					self = child
					recurse = true
					break
				}
				result := child.ChildByFieldID(fieldID)
				if !result.IsNull() {
					return result
				}
				start++
				if start == end {
					return nullNode()
				}
			} else if child.isRelevant(true) {
				return child
			} else if child.ChildCount() > 0 {
				return child.Child(0)
			} else {
				start++
				if start == end {
					return nullNode()
				}
			}
		}

		if !recurse {
			return nullNode()
		}
	}
}

// ChildByFieldName returns the child registered under the given field name.
func (n Node) ChildByFieldName(name string) Node {
	return n.ChildByFieldID(n.tree.language.FieldIDForName(name))
}

func (n Node) fieldNameFromLanguage(structuralChildIndex uint32) string {
	for _, m := range n.tree.language.fieldMap(uint32(subtreeProductionID(n.subtree()))) {
		if !m.Inherited && uint32(m.ChildIndex) == structuralChildIndex {
			return n.tree.language.FieldNames[m.FieldID]
		}
	}
	return ""
}

func (n Node) fieldNameForChild(childIndex uint32, includeAnonymous bool) string {
	result := n
	didDescend := true
	inheritedFieldName := ""

	for didDescend {
		didDescend = false

		var child Node
		index := uint32(0)
		iterator := result.iterateChildren()
		for iterator.next(&child) {
			if child.isRelevant(includeAnonymous) {
				if index == childIndex {
					if child.IsExtra() {
						return ""
					}
					fieldName := result.fieldNameFromLanguage(iterator.structuralChildIndex - 1)
					if fieldName != "" {
						return fieldName
					}
					return inheritedFieldName
				}
				index++
			} else {
				grandchildIndex := childIndex - index
				grandchildCount := child.relevantChildCount(includeAnonymous)
				if grandchildIndex < grandchildCount {
					fieldName := result.fieldNameFromLanguage(iterator.structuralChildIndex - 1)
					if fieldName != "" {
						inheritedFieldName = fieldName
					}
					didDescend = true
					result = child
					childIndex = grandchildIndex
					break
				}
				index += grandchildCount
			}
		}
	}

	return ""
}

// FieldNameForChild reports the field name of the visible child at an index.
func (n Node) FieldNameForChild(index uint32) string {
	return n.fieldNameForChild(index, true)
}

// FieldNameForNamedChild reports the field name of the named child at an index.
func (n Node) FieldNameForNamedChild(index uint32) string {
	return n.fieldNameForChild(index, false)
}

// Edit shifts the node's recorded position to account for a change.
func (n *Node) Edit(edit InputEdit) {
	startByte := n.startByte
	startPoint := n.startPoint
	pointEdit(&startPoint, &startByte, edit)
	n.startByte = startByte
	n.startPoint = startPoint
}
