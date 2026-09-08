package treesitter

import (
	"fmt"
	"strings"
)

type editSpan struct {
	start  length
	oldEnd length
	newEnd length
}

type editEntry struct {
	tree *subtree
	edit editSpan
}

func subtreeEdit(self subtree, inputEdit InputEdit) subtree {
	stack := []editEntry{{
		tree: &self,
		edit: editSpan{
			start:  length{Bytes: inputEdit.StartByte, Extent: inputEdit.StartPoint},
			oldEnd: length{Bytes: inputEdit.OldEndByte, Extent: inputEdit.OldEndPoint},
			newEnd: length{Bytes: inputEdit.NewEndByte, Extent: inputEdit.NewEndPoint},
		},
	}}

	for len(stack) > 0 {
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		edit := entry.edit
		isNoop := edit.oldEnd.Bytes == edit.start.Bytes && edit.newEnd.Bytes == edit.start.Bytes
		isPureInsertion := edit.oldEnd.Bytes == edit.start.Bytes
		parentDependsOnColumn := subtreeDependsOnColumn(*entry.tree)
		columnShifted := edit.newEnd.Extent.Column != edit.oldEnd.Extent.Column

		size := subtreeSize(*entry.tree)
		padding := subtreePadding(*entry.tree)
		totalSize := lengthAdd(padding, size)
		lookaheadBytes := subtreeLookaheadBytes(*entry.tree)
		endByte := totalSize.Bytes + lookaheadBytes
		if edit.start.Bytes > endByte || (isNoop && edit.start.Bytes == endByte) {
			continue
		}

		switch {
		case edit.oldEnd.Bytes <= padding.Bytes:
			padding = lengthAdd(edit.newEnd, lengthSub(padding, edit.oldEnd))
		case edit.start.Bytes < padding.Bytes:
			size = lengthSaturatingSub(size, lengthSub(edit.oldEnd, padding))
			padding = edit.newEnd
		case edit.start.Bytes < totalSize.Bytes ||
			(edit.start.Bytes == totalSize.Bytes && isPureInsertion):
			size = lengthAdd(
				lengthSub(edit.newEnd, padding),
				lengthSaturatingSub(totalSize, edit.oldEnd),
			)
		}

		result := subtreeMakeMut(*entry.tree)
		subtreeSetPadding(result, padding)
		subtreeSetSize(result, size)
		subtreeSetHasChanges(result, true)
		*entry.tree = result

		var childLeft, childRight length
		children := subtreeChildren(result)
		for i := 0; i < len(children); i++ {
			child := &children[i]
			childSize := subtreeTotalSize(*child)
			childLeft = childRight
			childRight = lengthAdd(childLeft, childSize)

			if childRight.Bytes+subtreeLookaheadBytes(*child) < edit.start.Bytes {
				continue
			}

			if ((childLeft.Bytes > edit.oldEnd.Bytes) ||
				(childLeft.Bytes == edit.oldEnd.Bytes && childSize.Bytes > 0 && i > 0)) &&
				(!parentDependsOnColumn || childLeft.Extent.Row > padding.Extent.Row) &&
				(!subtreeDependsOnColumn(*child) || !columnShifted ||
					childLeft.Extent.Row > edit.oldEnd.Extent.Row) {
				break
			}

			childEdit := editSpan{
				start:  lengthSaturatingSub(edit.start, childLeft),
				oldEnd: lengthSaturatingSub(edit.oldEnd, childLeft),
				newEnd: lengthSaturatingSub(edit.newEnd, childLeft),
			}

			if childRight.Bytes > edit.start.Bytes ||
				(childRight.Bytes == edit.start.Bytes && isPureInsertion) {
				edit.newEnd = edit.start
			} else {
				childEdit.oldEnd = childEdit.start
				childEdit.newEnd = childEdit.start
			}

			stack = append(stack, editEntry{tree: child, edit: childEdit})
		}
	}

	return self
}

func writeCharToString(sb *strings.Builder, chr int32) {
	switch {
	case chr == -1:
		sb.WriteString("INVALID")
	case chr == 0:
		sb.WriteString("'\\0'")
	case chr == '\n':
		sb.WriteString("'\\n'")
	case chr == '\t':
		sb.WriteString("'\\t'")
	case chr == '\r':
		sb.WriteString("'\\r'")
	case chr > 0 && chr < 128 && isPrintable(byte(chr)):
		sb.WriteString("'")
		sb.WriteByte(byte(chr))
		sb.WriteString("'")
	default:
		fmt.Fprintf(sb, "%d", chr)
	}
}

func isPrintable(b byte) bool { return b >= 0x20 && b < 0x7F }

type writeFrame struct {
	subtree      subtree
	aliasSymbol  Symbol
	aliasIsNamed bool
	fieldName    string
	hasField     bool
	isRoot       bool

	preWritten           bool
	isVisible            bool
	childIndex           uint32
	structuralChildIndex uint32
	aliasSequence        []Symbol
	fieldMap             []FieldMapEntry
}

func subtreeString(
	self subtree, aliasSymbol Symbol, aliasIsNamed bool, language *Language, includeAll bool,
) string {
	var sb strings.Builder
	stack := []writeFrame{{
		subtree:      self,
		aliasSymbol:  aliasSymbol,
		aliasIsNamed: aliasIsNamed,
		isRoot:       true,
	}}

	for len(stack) > 0 {
		frame := &stack[len(stack)-1]
		node := frame.subtree

		if node.isNil() {
			if !frame.isRoot {
				sb.WriteString(" ")
				if frame.hasField {
					sb.WriteString(frame.fieldName)
					sb.WriteString(": ")
				}
			}
			sb.WriteString("(NULL)")
			stack = stack[:len(stack)-1]
			continue
		}

		if !frame.preWritten {
			isVisible := includeAll || subtreeMissing(node)
			if !isVisible {
				if frame.aliasSymbol != 0 {
					isVisible = frame.aliasIsNamed
				} else {
					isVisible = subtreeVisible(node) && subtreeNamed(node)
				}
			}

			if isVisible {
				if !frame.isRoot {
					sb.WriteString(" ")
					if frame.hasField {
						sb.WriteString(frame.fieldName)
						sb.WriteString(": ")
					}
				}

				if subtreeIsError(node) && subtreeChildCount(node) == 0 && subtreeSize(node).Bytes > 0 {
					sb.WriteString("(UNEXPECTED ")
					writeCharToString(&sb, subtreeLookaheadChar(node))
				} else {
					symbol := frame.aliasSymbol
					if symbol == 0 {
						symbol = subtreeSymbol(node)
					}
					symbolName := language.SymbolName(symbol)
					if subtreeMissing(node) {
						sb.WriteString("(MISSING ")
						if frame.aliasIsNamed || subtreeNamed(node) {
							sb.WriteString(symbolName)
						} else {
							sb.WriteString("\"")
							sb.WriteString(symbolName)
							sb.WriteString("\"")
						}
					} else {
						sb.WriteString("(")
						sb.WriteString(symbolName)
					}
				}
			} else if frame.isRoot {
				symbol := frame.aliasSymbol
				if symbol == 0 {
					symbol = subtreeSymbol(node)
				}
				symbolName := language.SymbolName(symbol)
				switch {
				case subtreeChildCount(node) > 0:
					sb.WriteString("(")
					sb.WriteString(symbolName)
				case subtreeNamed(node):
					sb.WriteString("(")
					sb.WriteString(symbolName)
					sb.WriteString(")")
				default:
					sb.WriteString("(\"")
					sb.WriteString(symbolName)
					sb.WriteString("\")")
				}
			}

			if subtreeChildCount(node) > 0 {
				frame.aliasSequence = language.aliasSequence(uint32(subtreeProductionID(node)))
				frame.fieldMap = language.fieldMap(uint32(subtreeProductionID(node)))
			}

			frame.isVisible = isVisible
			frame.preWritten = true
		}

		if frame.childIndex < subtreeChildCount(node) {
			child := subtreeChildren(node)[frame.childIndex]
			childFrame := writeFrame{subtree: child}

			if !subtreeExtra(child) {
				var childAliasSymbol Symbol
				if frame.aliasSequence != nil {
					childAliasSymbol = frame.aliasSequence[frame.structuralChildIndex]
				}
				childAliasIsNamed := false
				if childAliasSymbol != 0 {
					childAliasIsNamed = language.symbolMetadata(childAliasSymbol).Named
				}

				childFieldName := ""
				hasField := false
				if !frame.isVisible && frame.hasField {
					childFieldName = frame.fieldName
					hasField = true
				}
				for _, m := range frame.fieldMap {
					if !m.Inherited && uint32(m.ChildIndex) == frame.structuralChildIndex {
						childFieldName = language.FieldNames[m.FieldID]
						hasField = true
						break
					}
				}

				childFrame.aliasSymbol = childAliasSymbol
				childFrame.aliasIsNamed = childAliasIsNamed
				childFrame.fieldName = childFieldName
				childFrame.hasField = hasField
				frame.structuralChildIndex++
			}

			frame.childIndex++
			stack = append(stack, childFrame)
			continue
		}

		if frame.isVisible {
			sb.WriteString(")")
		}
		stack = stack[:len(stack)-1]
	}

	return sb.String()
}
