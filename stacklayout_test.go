package treesitter

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"unsafe"
)

// Whether a counter is oversized is a question about the STRUCT: Go pads to
// alignment, so a narrower field can buy nothing. Measured, not reasoned.

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

	// Narrowing without reordering saves nothing: the bytes fall into padding.
	assert.Equal(t, got, narrow)

}
