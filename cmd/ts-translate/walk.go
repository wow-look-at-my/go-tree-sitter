package main

import (
	"fmt"
	"strings"
)

type emitter struct {
	file *cFile
	sb   *strings.Builder
	pkg  string
	// setIndex maps a character set's C name to its encoded position.
	setIndex map[string]int
	// standalone emits Language here rather than in a hand written sibling.
	standalone bool
	// scannerPkg is the import path supplying the grammar's external scanner.
	scannerPkg string
}

func (e *emitter) printf(format string, args ...any) {
	fmt.Fprintf(e.sb, format, args...)
}

func (e *emitter) constant(name string) int64 {
	return e.file.consts[name]
}

func (e *emitter) array(name string) *arrayDecl {
	return e.file.arrays[name]
}

// dimension reports an array's declared size, or the extent of its designators.
func dimension(d *arrayDecl, index int) int64 {
	if index < len(d.dims) && d.dims[index] > 0 {
		return d.dims[index]
	}
	high := int64(0)
	pos := int64(0)
	for _, el := range d.value.elems {
		if el.index >= 0 {
			pos = int64(el.index)
		}
		pos++
		if pos > high {
			high = pos
		}
	}
	return high
}

// walk visits each element of an initializer with its resolved index.
func walk(v *initValue, visit func(index int, el *initValue)) {
	pos := 0
	for i := range v.elems {
		el := &v.elems[i]
		if el.index >= 0 {
			pos = el.index
		}
		visit(pos, el.value)
		pos++
	}
}

func scalarSlice(d *arrayDecl) []int64 {
	size := dimension(d, 0)
	out := make([]int64, size)
	walk(d.value, func(index int, el *initValue) {
		if index < len(out) {
			out[index] = el.num
		}
	})
	return out
}

func scalarSlice2(d *arrayDecl, rows, cols int64) []int64 {
	out := make([]int64, rows*cols)
	walk(d.value, func(row int, el *initValue) {
		walk(el, func(col int, inner *initValue) {
			at := int64(row)*cols + int64(col)
			if at < int64(len(out)) {
				out[at] = inner.num
			}
		})
	})
	return out
}
