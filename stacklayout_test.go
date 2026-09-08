package treesitter

import (
	"testing"
	"unsafe"
)

// stackNode carries four 32-bit counters. Whether any of them is oversized is a
// question about the STRUCT, not about the counter: Go lays fields out in
// declaration order and pads to alignment, so a narrower field can buy nothing.
// These variants measure that directly rather than reasoning about it.

type nodeNarrowCounters struct {
	state             StateID
	position          length
	links             []stackLink
	refCount          uint16
	errorCost         uint16
	nodeCount         uint32
	dynamicPrecedence int32
}

type nodeReordered struct {
	links             []stackLink
	position          length
	nodeCount         uint32
	dynamicPrecedence int32
	refCount          uint16
	errorCost         uint16
	state             StateID
}

func TestStackNodeLayoutIsAlreadyTight(t *testing.T) {
	got := unsafe.Sizeof(stackNode{})
	narrow := unsafe.Sizeof(nodeNarrowCounters{})
	reordered := unsafe.Sizeof(nodeReordered{})

	t.Logf("stackNode           %d bytes", got)
	t.Logf("counters narrowed   %d bytes (saves %d)", narrow, int(got)-int(narrow))
	t.Logf("narrowed+reordered  %d bytes (saves %d)", reordered, int(got)-int(reordered))
	t.Logf("  StateID %d, length %d, []stackLink %d",
		unsafe.Sizeof(StateID(0)), unsafe.Sizeof(length{}), unsafe.Sizeof([]stackLink(nil)))

	// The claim this pins: narrowing the counters WITHOUT reordering saves
	// nothing, because the bytes freed fall into padding the struct already
	// carries. A future edit that narrows a counter and reports a saving is
	// reporting one it did not get.
	if narrow != got {
		t.Errorf("narrowing alone changed the size: %d -> %d; the padding assumption no longer holds", got, narrow)
	}
}
