package treesitter

// InputEdit describes a change to a document.
type InputEdit struct {
	StartByte   uint32
	OldEndByte  uint32
	NewEndByte  uint32
	StartPoint  Point
	OldEndPoint Point
	NewEndPoint Point
}

// Tree is a syntax tree produced by a parser.
type Tree struct {
	root           subtree
	language       *Language
	includedRanges []Range
}

func newTree(root subtree, language *Language, includedRanges []Range) *Tree {
	return &Tree{
		root:           root,
		language:       language,
		includedRanges: append([]Range(nil), includedRanges...),
	}
}

// Copy returns a tree that shares the receiver's nodes.
func (t *Tree) Copy() *Tree {
	subtreeRetain(t.root)
	return newTree(t.root, t.language, t.includedRanges)
}

// Language reports the grammar used to build the tree.
func (t *Tree) Language() *Language { return t.language }

// IncludedRanges reports the regions the parser read.
func (t *Tree) IncludedRanges() []Range { return t.includedRanges }

// RootNode returns the tree's root.
func (t *Tree) RootNode() Node {
	return newNode(t, &t.root, subtreePadding(t.root), 0)
}

// RootNodeWithOffset returns the root shifted by the given offset.
func (t *Tree) RootNodeWithOffset(offsetBytes uint32, offsetExtent Point) Node {
	node := t.RootNode()
	node.startByte += offsetBytes
	node.startPoint = pointAdd(offsetExtent, node.startPoint)
	return node
}

// Edit adjusts the tree's node positions to account for a change.
func (t *Tree) Edit(edit InputEdit) {
	for i := range t.includedRanges {
		rangeEdit(&t.includedRanges[i], edit)
	}
	t.root = subtreeEdit(t.root, edit)
}

// String renders the tree as an S expression.
func (t *Tree) String() string {
	return subtreeString(t.root, 0, false, t.language, false)
}

func rangeEdit(r *Range, edit InputEdit) {
	if r.EndByte >= edit.OldEndByte {
		if r.EndByte != ^uint32(0) {
			r.EndByte = edit.NewEndByte + (r.EndByte - edit.OldEndByte)
			r.EndPoint = pointAdd(edit.NewEndPoint, pointSub(r.EndPoint, edit.OldEndPoint))
			if r.EndByte < edit.NewEndByte {
				r.EndByte = ^uint32(0)
				r.EndPoint = Point{Row: ^uint32(0), Column: ^uint32(0)}
			}
		}
	} else if r.EndByte > edit.StartByte {
		r.EndByte = edit.StartByte
		r.EndPoint = edit.StartPoint
	}

	if r.StartByte >= edit.OldEndByte {
		r.StartByte = edit.NewEndByte + (r.StartByte - edit.OldEndByte)
		r.StartPoint = pointAdd(edit.NewEndPoint, pointSub(r.StartPoint, edit.OldEndPoint))
		if r.StartByte < edit.NewEndByte {
			r.StartByte = ^uint32(0)
			r.StartPoint = Point{Row: ^uint32(0), Column: ^uint32(0)}
		}
	} else if r.StartByte > edit.StartByte {
		r.StartByte = edit.StartByte
		r.StartPoint = edit.StartPoint
	}
}

func pointEdit(point *Point, byteOffset *uint32, edit InputEdit) {
	if *byteOffset >= edit.OldEndByte {
		*byteOffset = edit.NewEndByte + (*byteOffset - edit.OldEndByte)
		*point = pointAdd(edit.NewEndPoint, pointSub(*point, edit.OldEndPoint))
	} else if *byteOffset > edit.StartByte {
		*byteOffset = edit.NewEndByte
		*point = edit.NewEndPoint
	}
}
