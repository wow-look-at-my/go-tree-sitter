package treesitter

type reusableStackEntry struct {
	tree       subtree
	childIndex uint32
	byteOffset uint32
}

type reusableNode struct {
	stack             []reusableStackEntry
	lastExternalToken subtree
}

func (r *reusableNode) clear() {
	r.stack = r.stack[:0]
	r.lastExternalToken = nil
}

func (r *reusableNode) tree() subtree {
	if len(r.stack) > 0 {
		return r.stack[len(r.stack)-1].tree
	}
	return nil
}

func (r *reusableNode) byteOffset() uint32 {
	if len(r.stack) > 0 {
		return r.stack[len(r.stack)-1].byteOffset
	}
	return ^uint32(0)
}

func (r *reusableNode) advance() {
	lastEntry := r.stack[len(r.stack)-1]
	byteOffset := lastEntry.byteOffset + subtreeTotalBytes(lastEntry.tree)
	if subtreeHasExternalTokens(lastEntry.tree) {
		r.lastExternalToken = subtreeLastExternalToken(lastEntry.tree)
	}

	var tree subtree
	var nextIndex uint32
	for {
		popped := r.stack[len(r.stack)-1]
		r.stack = r.stack[:len(r.stack)-1]
		nextIndex = popped.childIndex + 1
		if len(r.stack) == 0 {
			return
		}
		tree = r.stack[len(r.stack)-1].tree
		if subtreeChildCount(tree) > nextIndex {
			break
		}
	}

	r.stack = append(r.stack, reusableStackEntry{
		tree:       tree.children[nextIndex],
		childIndex: nextIndex,
		byteOffset: byteOffset,
	})
}

func (r *reusableNode) descend() bool {
	lastEntry := r.stack[len(r.stack)-1]
	if subtreeChildCount(lastEntry.tree) > 0 {
		r.stack = append(r.stack, reusableStackEntry{
			tree:       lastEntry.tree.children[0],
			childIndex: 0,
			byteOffset: lastEntry.byteOffset,
		})
		return true
	}
	return false
}

func (r *reusableNode) advancePastLeaf() {
	for r.descend() {
	}
	r.advance()
}

func (r *reusableNode) reset(tree subtree) {
	r.clear()
	r.stack = append(r.stack, reusableStackEntry{tree: tree})
	if !r.descend() {
		r.clear()
	}
}
