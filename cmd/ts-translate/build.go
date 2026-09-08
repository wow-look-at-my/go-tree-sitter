package main

import (
	"sort"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// buildTables turns the parsed C tables into the value the runtime encodes. The
// lexer functions and the scanner stay out of it: those are code, and the
// generated package installs them.
func (e *emitter) buildTables() *ts.Tables {
	t := &ts.Tables{}
	l := &t.Language

	scalars := map[string]*uint32{
		"abi_version":          &l.ABIVersion,
		"symbol_count":         &l.SymbolCount,
		"alias_count":          &l.AliasCount,
		"token_count":          &l.TokenCount,
		"external_token_count": &l.ExternalTokenCount,
		"state_count":          &l.StateCount,
		"large_state_count":    &l.LargeStateCount,
		"production_id_count":  &l.ProductionIDCount,
		"field_count":          &l.FieldCount,
		"supertype_count":      &l.SupertypeCount,
	}
	for name, into := range scalars {
		if v, ok := e.file.lang[name]; ok && v.isNum {
			*into = uint32(v.num)
		}
	}
	if v, ok := e.file.lang["max_alias_sequence_length"]; ok && v.isNum {
		l.MaxAliasSequenceLen = uint16(v.num)
	}
	if v, ok := e.file.lang["max_reserved_word_set_size"]; ok && v.isNum {
		l.MaxReservedWordSetLen = uint16(v.num)
	}
	if v, ok := e.file.lang["keyword_capture_token"]; ok && v.isNum {
		l.KeywordCaptureToken = ts.Symbol(v.num)
	}
	if v, ok := e.file.lang["name"]; ok && v.isStr {
		l.Name = v.str
	}
	if v, ok := e.file.lang["metadata"]; ok && v.elems != nil {
		l.Metadata = ts.LanguageMetadata{
			MajorVersion: uint8(v.elems["major_version"]),
			MinorVersion: uint8(v.elems["minor_version"]),
			PatchVersion: uint8(v.elems["patch_version"]),
		}
	}

	if d := e.linked("parse_table"); d != nil {
		rows := e.constant("LARGE_STATE_COUNT")
		cols := e.constant("SYMBOL_COUNT")
		l.ParseTable = toU16(scalarSlice2(d, rows, cols))
	}
	if d := e.linked("small_parse_table"); d != nil {
		l.SmallParseTable = toU16(scalarSlice(d))
	}
	if d := e.linked("small_parse_table_map"); d != nil {
		l.SmallParseTableMap = toU32(scalarSlice(d))
	}
	if d := e.linked("parse_actions"); d != nil {
		l.ParseActions = buildActions(d)
	}
	if d := e.linked("symbol_names"); d != nil {
		l.SymbolNames = buildStrings(d)
	}
	if d := e.linked("field_names"); d != nil {
		l.FieldNames = buildStrings(d)
	}
	if d := e.linked("field_map_slices"); d != nil {
		l.FieldMapSlices = buildMapSlices(d)
	}
	if d := e.linked("field_map_entries"); d != nil {
		l.FieldMapEntries = buildFieldEntries(d)
	}
	if d := e.linked("symbol_metadata"); d != nil {
		l.SymbolMetadataTable = buildMetadata(d)
	}
	if d := e.linked("public_symbol_map"); d != nil {
		l.PublicSymbolMap = toSymbols(scalarSlice(d))
	}
	if d := e.linked("alias_map"); d != nil {
		l.AliasMap = toU16(scalarSlice(d))
	}
	if d := e.linked("alias_sequences"); d != nil {
		rows := e.constant("PRODUCTION_ID_COUNT")
		cols := e.constant("MAX_ALIAS_SEQUENCE_LENGTH")
		l.AliasSequences = toSymbols(scalarSlice2(d, rows, cols))
	}
	if d := e.linked("lex_modes"); d != nil {
		l.LexModes = buildLexModes(d)
	}
	if d := e.linked("primary_state_ids"); d != nil {
		l.PrimaryStateIDs = toStates(scalarSlice(d))
	}
	if d := e.linked("reserved_words"); d != nil {
		rows := dimension(d, 0)
		cols := e.constant("MAX_RESERVED_WORD_SET_SIZE")
		l.ReservedWords = toSymbols(scalarSlice2(d, rows, cols))
	}
	if d := e.linked("supertype_symbols"); d != nil {
		l.SupertypeSymbols = toSymbols(scalarSlice(d))
	}
	if d := e.linked("supertype_map_slices"); d != nil {
		l.SupertypeMapSlices = buildMapSlices(d)
	}
	if d := e.linked("supertype_map_entries"); d != nil {
		l.SupertypeMapEntries = toSymbols(scalarSlice(d))
	}
	if d := e.array("ts_external_scanner_symbol_map"); d != nil {
		l.ExternalScannerSymbol = toSymbols(scalarSlice(d))
	}
	if d := e.array("ts_external_scanner_states"); d != nil {
		rows := dimension(d, 0)
		cols := e.constant("EXTERNAL_TOKEN_COUNT")
		l.ExternalScannerStates = toBools(scalarSlice2(d, rows, cols))
	}

	t.CharacterSets = e.buildCharacterSets()
	return t
}

// linked resolves a language field to the array it points at.
func (e *emitter) linked(field string) *arrayDecl {
	v, ok := e.file.lang[field]
	if !ok || v.ref == "" {
		return nil
	}
	return e.array(v.ref)
}

// buildCharacterSets collects every set the lexer searches, in sorted order, and
// records where each one landed so the emitted lexer can index it.
func (e *emitter) buildCharacterSets() [][]ts.CharacterRange {
	names := map[string]bool{}
	for _, fn := range e.file.lexFns {
		for _, c := range fn.cases {
			for _, st := range c.stmts {
				collectSets(st, names)
			}
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	e.setIndex = map[string]int{}
	out := make([][]ts.CharacterRange, 0, len(ordered))
	for _, name := range ordered {
		d := e.array(name)
		if d == nil {
			panic("missing character set " + name)
		}
		var set []ts.CharacterRange
		walk(d.value, func(_ int, el *initValue) {
			set = append(set, ts.CharacterRange{
				Start: int32(el.elems[0].value.num),
				End:   int32(el.elems[1].value.num),
			})
		})
		e.setIndex[name] = len(out)
		out = append(out, set)
	}
	return out
}

func toU16(v []int64) []uint16 {
	out := make([]uint16, len(v))
	for i, x := range v {
		out[i] = uint16(x)
	}
	return out
}

func toU32(v []int64) []uint32 {
	out := make([]uint32, len(v))
	for i, x := range v {
		out[i] = uint32(x)
	}
	return out
}

func toSymbols(v []int64) []ts.Symbol {
	out := make([]ts.Symbol, len(v))
	for i, x := range v {
		out[i] = ts.Symbol(x)
	}
	return out
}

func toStates(v []int64) []ts.StateID {
	out := make([]ts.StateID, len(v))
	for i, x := range v {
		out[i] = ts.StateID(x)
	}
	return out
}

func toBools(v []int64) []bool {
	out := make([]bool, len(v))
	for i, x := range v {
		out[i] = x != 0
	}
	return out
}

func buildStrings(d *arrayDecl) []string {
	out := make([]string, dimension(d, 0))
	walk(d.value, func(index int, el *initValue) {
		if index < len(out) {
			out[index] = el.str
		}
	})
	return out
}

func buildMapSlices(d *arrayDecl) []ts.MapSlice {
	out := make([]ts.MapSlice, dimension(d, 0))
	walk(d.value, func(index int, el *initValue) {
		if index >= len(out) {
			return
		}
		var s ts.MapSlice
		for i, f := range el.elems {
			switch {
			case f.field == "index" || (f.field == "" && i == 0):
				s.Index = uint16(f.value.num)
			case f.field == "length" || (f.field == "" && i == 1):
				s.Length = uint16(f.value.num)
			}
		}
		out[index] = s
	})
	return out
}

func buildFieldEntries(d *arrayDecl) []ts.FieldMapEntry {
	out := make([]ts.FieldMapEntry, dimension(d, 0))
	walk(d.value, func(index int, el *initValue) {
		if index >= len(out) {
			return
		}
		var en ts.FieldMapEntry
		positional := 0
		for _, f := range el.elems {
			switch f.field {
			case "field_id":
				en.FieldID = ts.FieldID(f.value.num)
			case "child_index":
				en.ChildIndex = uint8(f.value.num)
			case "inherited":
				en.Inherited = f.value.num != 0
			case "":
				if positional == 0 {
					en.FieldID = ts.FieldID(f.value.num)
				} else {
					en.ChildIndex = uint8(f.value.num)
				}
				positional++
			}
		}
		out[index] = en
	})
	return out
}

func buildMetadata(d *arrayDecl) []ts.SymbolMetadata {
	out := make([]ts.SymbolMetadata, dimension(d, 0))
	walk(d.value, func(index int, el *initValue) {
		if index >= len(out) {
			return
		}
		var m ts.SymbolMetadata
		for _, f := range el.elems {
			switch f.field {
			case "visible":
				m.Visible = f.value.num != 0
			case "named":
				m.Named = f.value.num != 0
			case "supertype":
				m.Supertype = f.value.num != 0
			}
		}
		out[index] = m
	})
	return out
}

func buildLexModes(d *arrayDecl) []ts.LexerMode {
	out := make([]ts.LexerMode, dimension(d, 0))
	walk(d.value, func(index int, el *initValue) {
		if index >= len(out) {
			return
		}
		var m ts.LexerMode
		// A state with no lexer is written positionally, as a cast of a negative
		// value, while every other state names its fields. The runtime reads
		// that sentinel to find the end of a non-terminal extra.
		positional := 0
		for _, f := range el.elems {
			switch f.field {
			case "lex_state":
				m.LexState = uint16(f.value.num)
			case "external_lex_state":
				m.ExternalLexState = uint16(f.value.num)
			case "reserved_word_set_id":
				m.ReservedWordSetID = uint16(f.value.num)
			case "":
				switch positional {
				case 0:
					m.LexState = uint16(f.value.num)
				case 1:
					m.ExternalLexState = uint16(f.value.num)
				case 2:
					m.ReservedWordSetID = uint16(f.value.num)
				}
				positional++
			}
		}
		out[index] = m
	})
	return out
}

func buildActions(d *arrayDecl) []ts.ParseActionEntry {
	size := dimension(d, 0)
	values := make([]*initValue, size)
	walk(d.value, func(index int, el *initValue) {
		if index < len(values) {
			values[index] = el
		}
	})
	out := make([]ts.ParseActionEntry, size)
	for i, el := range values {
		switch {
		case el == nil:
		case el.isAction:
			out[i] = ts.ParseActionEntry{Action: buildAction(el.action)}
		default:
			if len(el.elems) > 0 && el.elems[0].field == "entry" {
				for _, f := range el.elems[0].value.elems {
					switch f.field {
					case "count":
						out[i].Count = uint8(f.value.num)
					case "reusable":
						out[i].Reusable = f.value.num != 0
					}
				}
			}
		}
	}
	return out
}

func buildAction(a actionValue) ts.ParseAction {
	switch a.kind {
	case "shift":
		return ts.ParseAction{Type: ts.ParseActionTypeShift, State: ts.StateID(a.state)}
	case "shiftRepeat":
		return ts.ParseAction{
			Type: ts.ParseActionTypeShift, State: ts.StateID(a.state), Repetition: true,
		}
	case "shiftExtra":
		return ts.ParseAction{Type: ts.ParseActionTypeShift, Extra: true}
	case "reduce":
		return ts.ParseAction{
			Type:              ts.ParseActionTypeReduce,
			Symbol:            ts.Symbol(a.symbol),
			ChildCount:        uint8(a.childCount),
			DynamicPrecedence: int16(a.dynamicPrecedence),
			ProductionID:      uint16(a.productionID),
		}
	case "recover":
		return ts.ParseAction{Type: ts.ParseActionTypeRecover}
	default:
		return ts.ParseAction{Type: ts.ParseActionTypeAccept}
	}
}
