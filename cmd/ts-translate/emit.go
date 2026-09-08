package main

import (
	"fmt"
	"strings"
)

type emitter struct {
	file *cFile
	sb   *strings.Builder
	pkg  string
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

// dimension reports an array's declared size, or its highest index plus one.
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

func (e *emitter) emitIntSlice(name, goType string, values []int64) {
	e.printf("var %s = []%s{", name, goType)
	for i, v := range values {
		if i%16 == 0 {
			e.printf("\n\t")
		}
		e.printf("%d, ", v)
	}
	e.printf("\n}\n\n")
}

func (e *emitter) emitStringSlice(name string, d *arrayDecl) {
	size := dimension(d, 0)
	values := make([]string, size)
	walk(d.value, func(index int, el *initValue) {
		if index < len(values) {
			values[index] = el.str
		}
	})
	e.printf("var %s = []string{\n", name)
	for _, v := range values {
		e.printf("\t%q,\n", v)
	}
	e.printf("}\n\n")
}

func (e *emitter) emitSymbolMetadata(name string, d *arrayDecl) {
	size := dimension(d, 0)
	type meta struct{ visible, named, supertype bool }
	values := make([]meta, size)
	walk(d.value, func(index int, el *initValue) {
		if index >= len(values) {
			return
		}
		var m meta
		for _, f := range el.elems {
			switch f.field {
			case "visible":
				m.visible = f.value.num != 0
			case "named":
				m.named = f.value.num != 0
			case "supertype":
				m.supertype = f.value.num != 0
			}
		}
		values[index] = m
	})
	e.printf("var %s = []ts.SymbolMetadata{\n", name)
	for _, m := range values {
		e.printf("\t{Visible: %t, Named: %t, Supertype: %t},\n", m.visible, m.named, m.supertype)
	}
	e.printf("}\n\n")
}

func (e *emitter) emitMapSlices(name string, d *arrayDecl) {
	size := dimension(d, 0)
	type slice struct{ index, length int64 }
	values := make([]slice, size)
	walk(d.value, func(index int, el *initValue) {
		if index >= len(values) {
			return
		}
		var s slice
		for i, f := range el.elems {
			switch {
			case f.field == "index" || (f.field == "" && i == 0):
				s.index = f.value.num
			case f.field == "length" || (f.field == "" && i == 1):
				s.length = f.value.num
			}
		}
		values[index] = s
	})
	e.printf("var %s = []ts.MapSlice{\n", name)
	for _, s := range values {
		e.printf("\t{Index: %d, Length: %d},\n", s.index, s.length)
	}
	e.printf("}\n\n")
}

func (e *emitter) emitFieldMapEntries(name string, d *arrayDecl) {
	size := dimension(d, 0)
	type entry struct {
		fieldID    int64
		childIndex int64
		inherited  bool
	}
	values := make([]entry, size)
	walk(d.value, func(index int, el *initValue) {
		if index >= len(values) {
			return
		}
		var en entry
		positional := 0
		for _, f := range el.elems {
			switch f.field {
			case "field_id":
				en.fieldID = f.value.num
			case "child_index":
				en.childIndex = f.value.num
			case "inherited":
				en.inherited = f.value.num != 0
			case "":
				if positional == 0 {
					en.fieldID = f.value.num
				} else {
					en.childIndex = f.value.num
				}
				positional++
			}
		}
		values[index] = en
	})
	e.printf("var %s = []ts.FieldMapEntry{\n", name)
	for _, en := range values {
		e.printf("\t{FieldID: %d, ChildIndex: %d, Inherited: %t},\n",
			en.fieldID, en.childIndex, en.inherited)
	}
	e.printf("}\n\n")
}

func (e *emitter) emitLexModes(name string, d *arrayDecl) {
	size := dimension(d, 0)
	type mode struct{ lexState, externalLexState, reservedWordSetID int64 }
	values := make([]mode, size)
	walk(d.value, func(index int, el *initValue) {
		if index >= len(values) {
			return
		}
		var m mode
		for _, f := range el.elems {
			switch f.field {
			case "lex_state":
				m.lexState = f.value.num
			case "external_lex_state":
				m.externalLexState = f.value.num
			case "reserved_word_set_id":
				m.reservedWordSetID = f.value.num
			}
		}
		values[index] = m
	})
	e.printf("var %s = []ts.LexerMode{\n", name)
	for _, m := range values {
		e.printf("\t{LexState: %d, ExternalLexState: %d, ReservedWordSetID: %d},\n",
			uint16(m.lexState), uint16(m.externalLexState), uint16(m.reservedWordSetID))
	}
	e.printf("}\n\n")
}

func (e *emitter) emitCharacterSet(name string, d *arrayDecl) {
	e.printf("var %s = []ts.CharacterRange{\n", name)
	walk(d.value, func(_ int, el *initValue) {
		start := el.elems[0].value.num
		end := el.elems[1].value.num
		e.printf("\t{Start: %d, End: %d},\n", start, end)
	})
	e.printf("}\n\n")
}

func (e *emitter) emitParseActions(name string, d *arrayDecl) {
	e.printf("var %s = []ts.ParseActionEntry{\n", name)
	size := dimension(d, 0)
	values := make([]*initValue, size)
	walk(d.value, func(index int, el *initValue) {
		if index < len(values) {
			values[index] = el
		}
	})
	for _, el := range values {
		if el == nil {
			e.printf("\t{},\n")
			continue
		}
		if el.isAction {
			e.emitAction(el.action)
			continue
		}
		count := int64(0)
		reusable := false
		if len(el.elems) > 0 && el.elems[0].field == "entry" {
			for _, f := range el.elems[0].value.elems {
				switch f.field {
				case "count":
					count = f.value.num
				case "reusable":
					reusable = f.value.num != 0
				}
			}
		}
		e.printf("\t{Count: %d, Reusable: %t},\n", count, reusable)
	}
	e.printf("}\n\n")
}

func (e *emitter) emitAction(a actionValue) {
	switch a.kind {
	case "shift":
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeShift, State: %d}},\n", a.state)
	case "shiftRepeat":
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeShift, State: %d, Repetition: true}},\n", a.state)
	case "shiftExtra":
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeShift, Extra: true}},\n")
	case "reduce":
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeReduce, Symbol: %d, ChildCount: %d, DynamicPrecedence: %d, ProductionID: %d}},\n",
			a.symbol, a.childCount, a.dynamicPrecedence, a.productionID)
	case "recover":
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeRecover}},\n")
	default:
		e.printf("\t{Action: ts.ParseAction{Type: ts.ParseActionTypeAccept}},\n")
	}
}
