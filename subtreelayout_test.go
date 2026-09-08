package treesitter

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

// Upstream's Subtree is a tagged union: a small leaf token lives BY VALUE in the
// parent's child array, and only the rest reach the heap. This port kept the
// heap arm alone, so every leaf costs a pointer in the array plus a whole
// subtreeData behind it, retained for the tree's lifetime and traced on every
// GC cycle.
//
// These variants measure what an inline arm would cost. Go cannot overlay an
// integer on a pointer the way the C union does -- the collector has to see a
// real pointer slot -- so the pointer word is unavoidable and the question is
// only what rides beside it.

// subtreeInlineFields is the readable arm: upstream's bitfields as named bytes.
type subtreeInlineFields struct {
	symbol         uint8
	paddingColumns uint8
	paddingBytes   uint8
	sizeBytes      uint8
	paddingRows    uint8
	lookaheadBytes uint8
	flags          uint8
	parseState     StateID
}

type subtreeNamedArm struct {
	ptr    *subtreeData
	inline subtreeInlineFields
}

// subtreePackedArm is the dense arm: the same fields decoded out of one word.
type subtreePackedArm struct {
	ptr    *subtreeData
	packed uint64
}

func TestInlineLeafArmIsWorthItsAccessors(t *testing.T) {
	heap := unsafe.Sizeof(subtreeData{})
	slot := unsafe.Sizeof(subtree(nil))
	named := unsafe.Sizeof(subtreeNamedArm{})
	packed := unsafe.Sizeof(subtreePackedArm{})

	t.Logf("today: %d B slot + %d B heap object = %d B per leaf", slot, heap, slot+heap)
	t.Logf("named-field arm: %d B per leaf, no heap object", named)
	t.Logf("packed-word arm: %d B per leaf, no heap object", packed)

	// What justifies the change at all: an inline leaf must cost far less than
	// the slot plus the heap object it replaces. A future edit that grows
	// subtreeData is free to do so, but an inline arm that stops paying for
	// itself is a representation carrying accessors it no longer earns.
	assert.Less(t, named, slot+heap)
	assert.Less(t, packed, slot+heap)

	// The named fields do NOT disappear into the pointer's padding, so the
	// dense arm is genuinely smaller. It is still not the one to build: beside
	// what a leaf costs today the two arms are within a rounding error of each
	// other, and the dense one pays for that margin in hand-written decode on
	// the accessors every read goes through. This port's standing risk is a
	// subtle decode bug, not a byte.
	assert.Less(t, packed, named)
}
