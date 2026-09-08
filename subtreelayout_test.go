package treesitter

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// What a leaf costs in each arm. The collector has to see a real pointer slot,
// so the arms cannot overlay and one of them is always dead weight.

// The arm this port did not build, measured beside the real one so the choice
// stays checkable rather than remembered.
type subtreePackedArm struct {
	ptr    *subtreeData
	packed uint64
}

func TestAnInlineLeafCostsFarLessThanAHeapOne(t *testing.T) {
	slot := unsafe.Sizeof(subtree{})
	heap := unsafe.Sizeof(subtreeData{})
	packed := unsafe.Sizeof(subtreePackedArm{})

	t.Logf("inline leaf: %d B in the parent's array, no heap object", slot)
	t.Logf("heap leaf:   %d B slot + %d B object = %d B", slot, heap, slot+heap)
	t.Logf("packed arm not built: %d B", packed)

	// An arm costing no less than the object it replaces has not earned itself.
	assert.Less(t, slot, slot+heap)

	// Smaller, and still not worth the hand-written decode on every accessor.
	assert.Less(t, packed, slot+heap)
}

// The arm is reached by ordinary parsing. A representation nothing ever
// constructs is dead code that still has to be read and maintained, and every
// accessor's inline branch would be a guard for a state the parser is never in.
func TestOrdinaryParsingProducesInlineLeaves(t *testing.T) {
	tree := parseToy(t, "hello world")

	var inline, heap int
	var walk func(s subtree)
	walk = func(s subtree) {
		if subtreeChildCount(s) == 0 {
			if s.isInline() {
				inline++
			} else {
				heap++
			}
			return
		}
		for _, child := range subtreeChildren(s) {
			walk(child)
		}
	}
	walk(tree.root)

	t.Logf("leaves: %d inline, %d on the heap", inline, heap)
	assert.Positive(t, inline, "no leaf was inlined: the arm is unreachable")
}

// The exclusions are the part that decides correctness, because an inlined
// external token would lose the scanner state a caller takes a pointer into.
func TestALeafThatCannotBeInlinedGoesToTheHeap(t *testing.T) {
	language := toyLanguage()
	small := length{Bytes: 1, Extent: Point{Row: 0, Column: 1}}
	zero := length{}

	fits := newLeafSubtree(toySymWord, zero, small, 0, 1, false, false, false, language)
	assert.True(t, fits.isInline(), "a small plain leaf belongs inline")

	external := newLeafSubtree(toySymWord, zero, small, 0, 1, true, false, false, language)
	assert.False(t, external.isInline(), "an external token keeps its scanner state on the heap")

	column := newLeafSubtree(toySymWord, zero, small, 0, 1, false, true, false, language)
	assert.False(t, column.isInline(), "the inline arm cannot answer for column dependence")

	tall := length{Bytes: 4, Extent: Point{Row: 1, Column: 0}}
	assert.False(t,
		newLeafSubtree(toySymWord, zero, tall, 0, 1, false, false, false, language).isInline(),
		"a token that spans a row has an extent the inline arm cannot rebuild")

	wide := length{Bytes: maxInlineLength, Extent: Point{Row: 0, Column: maxInlineLength}}
	assert.False(t,
		newLeafSubtree(toySymWord, zero, wide, 0, 1, false, false, false, language).isInline(),
		"a token at the width cap does not fit a byte")
}
