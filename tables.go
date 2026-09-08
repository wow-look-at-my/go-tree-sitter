package treesitter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// A grammar's data tables travel as a compressed binary rather than as Go
// source. The parse table of a large grammar holds millions of integers, and a
// compiler that type-checks every one of them costs far more than a decode.

// tablesMagic opens the blob, and tablesFormat changes whenever the section
// order below changes.
const (
	tablesMagic  = "TSGO"
	tablesFormat = 1
)

// Tables is a grammar's data, as the blob carries it. The function fields and
// the scanner of the embedded Language are not encoded: they are code, and the
// generated package supplies them.
type Tables struct {
	Language
	// CharacterSets holds the sets the generated lexer searches.
	CharacterSets [][]CharacterRange
}

// The blob is zstd rather than gzip. Measured on the C++ tables, zstd is both
// smaller and faster to decode, and the package is pure Go, so a consumer that
// builds with CGO_ENABLED=0 still links.
var (
	encoderOnce sync.Once
	encoder     *zstd.Encoder
	decoderOnce sync.Once
	decoder     *zstd.Decoder
	codecErr    error
)

// EncodeTables writes a grammar's data as a compressed blob.
func EncodeTables(t *Tables) ([]byte, error) {
	encoderOnce.Do(func() {
		encoder, codecErr = zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	})
	if codecErr != nil {
		return nil, codecErr
	}

	var w tableWriter
	w.raw([]byte(tablesMagic))
	w.u8(tablesFormat)
	w.encode(t)
	return encoder.EncodeAll(w.buf.Bytes(), nil), nil
}

// DecodeTables reads back what EncodeTables wrote.
func DecodeTables(blob []byte) (*Tables, error) {
	decoderOnce.Do(func() {
		decoder, codecErr = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
	})
	if codecErr != nil {
		return nil, codecErr
	}
	plain, err := decoder.DecodeAll(blob, nil)
	if err != nil {
		return nil, fmt.Errorf("reading the table blob: %w", err)
	}
	r := &tableReader{buf: plain}
	if string(r.raw(len(tablesMagic))) != tablesMagic {
		return nil, errors.New("the table blob does not start with its magic")
	}
	if v := r.u8(); v != tablesFormat {
		return nil, fmt.Errorf("the table blob is format %d, and this runtime reads %d",
			v, tablesFormat)
	}
	t := &Tables{}
	r.decode(t)
	if r.err != nil {
		return nil, r.err
	}
	if r.pos != len(r.buf) {
		return nil, fmt.Errorf("the table blob has %d trailing bytes", len(r.buf)-r.pos)
	}
	return t, nil
}

// encode writes every section, in the order decode reads them.
func (w *tableWriter) encode(t *Tables) {
	l := &t.Language
	for _, v := range []uint32{
		l.ABIVersion, l.SymbolCount, l.AliasCount, l.TokenCount, l.ExternalTokenCount,
		l.StateCount, l.LargeStateCount, l.ProductionIDCount, l.FieldCount, l.SupertypeCount,
	} {
		w.u32(v)
	}
	w.u16(l.MaxAliasSequenceLen)
	w.u16(l.MaxReservedWordSetLen)
	w.u16(uint16(l.KeywordCaptureToken))
	w.u8(l.Metadata.MajorVersion)
	w.u8(l.Metadata.MinorVersion)
	w.u8(l.Metadata.PatchVersion)
	w.str(l.Name)

	w.u16s(l.ParseTable)
	w.u16s(l.SmallParseTable)
	w.u32s(l.SmallParseTableMap)
	w.actions(l.ParseActions)
	w.strs(l.SymbolNames)
	w.strs(l.FieldNames)
	w.slices(l.FieldMapSlices)
	w.fieldEntries(l.FieldMapEntries)
	w.metadata(l.SymbolMetadataTable)
	w.symbols(l.PublicSymbolMap)
	w.u16s(l.AliasMap)
	w.symbols(l.AliasSequences)
	w.lexModes(l.LexModes)
	w.states(l.PrimaryStateIDs)
	w.symbols(l.ReservedWords)
	w.symbols(l.SupertypeSymbols)
	w.slices(l.SupertypeMapSlices)
	w.symbols(l.SupertypeMapEntries)
	w.bools(l.ExternalScannerStates)
	w.symbols(l.ExternalScannerSymbol)
	w.charSets(t.CharacterSets)
}

// decode reads every section, in the order encode wrote them.
func (r *tableReader) decode(t *Tables) {
	l := &t.Language
	for _, p := range []*uint32{
		&l.ABIVersion, &l.SymbolCount, &l.AliasCount, &l.TokenCount, &l.ExternalTokenCount,
		&l.StateCount, &l.LargeStateCount, &l.ProductionIDCount, &l.FieldCount, &l.SupertypeCount,
	} {
		*p = r.u32()
	}
	l.MaxAliasSequenceLen = r.u16()
	l.MaxReservedWordSetLen = r.u16()
	l.KeywordCaptureToken = Symbol(r.u16())
	l.Metadata.MajorVersion = r.u8()
	l.Metadata.MinorVersion = r.u8()
	l.Metadata.PatchVersion = r.u8()
	l.Name = r.str()

	l.ParseTable = r.u16s()
	l.SmallParseTable = r.u16s()
	l.SmallParseTableMap = r.u32s()
	l.ParseActions = r.actions()
	l.SymbolNames = r.strs()
	l.FieldNames = r.strs()
	l.FieldMapSlices = r.slices()
	l.FieldMapEntries = r.fieldEntries()
	l.SymbolMetadataTable = r.metadata()
	l.PublicSymbolMap = r.symbols()
	l.AliasMap = r.u16s()
	l.AliasSequences = r.symbols()
	l.LexModes = r.lexModes()
	l.PrimaryStateIDs = r.states()
	l.ReservedWords = r.symbols()
	l.SupertypeSymbols = r.symbols()
	l.SupertypeMapSlices = r.slices()
	l.SupertypeMapEntries = r.symbols()
	l.ExternalScannerStates = r.bools()
	l.ExternalScannerSymbol = r.symbols()
	t.CharacterSets = r.charSets()
}

type tableWriter struct {
	buf bytes.Buffer
	tmp [4]byte
}

func (w *tableWriter) raw(b []byte) { w.buf.Write(b) }

func (w *tableWriter) u8(v uint8) { w.buf.WriteByte(v) }

func (w *tableWriter) u16(v uint16) {
	binary.LittleEndian.PutUint16(w.tmp[:2], v)
	w.buf.Write(w.tmp[:2])
}

func (w *tableWriter) u32(v uint32) {
	binary.LittleEndian.PutUint32(w.tmp[:4], v)
	w.buf.Write(w.tmp[:4])
}

func (w *tableWriter) str(s string) {
	w.u32(uint32(len(s)))
	w.buf.WriteString(s)
}

func (w *tableWriter) strs(v []string) {
	w.u32(uint32(len(v)))
	for _, s := range v {
		w.str(s)
	}
}

func (w *tableWriter) u16s(v []uint16) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(x)
	}
}

func (w *tableWriter) u32s(v []uint32) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u32(x)
	}
}

func (w *tableWriter) symbols(v []Symbol) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(uint16(x))
	}
}

func (w *tableWriter) states(v []StateID) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(uint16(x))
	}
}

func (w *tableWriter) bools(v []bool) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u8(boolByte(x))
	}
}

func (w *tableWriter) slices(v []MapSlice) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(x.Index)
		w.u16(x.Length)
	}
}

func (w *tableWriter) fieldEntries(v []FieldMapEntry) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(uint16(x.FieldID))
		w.u8(x.ChildIndex)
		w.u8(boolByte(x.Inherited))
	}
}

func (w *tableWriter) metadata(v []SymbolMetadata) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		var bits uint8
		if x.Visible {
			bits |= 1
		}
		if x.Named {
			bits |= 2
		}
		if x.Supertype {
			bits |= 4
		}
		w.u8(bits)
	}
}

func (w *tableWriter) lexModes(v []LexerMode) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		w.u16(x.LexState)
		w.u16(x.ExternalLexState)
		w.u16(x.ReservedWordSetID)
	}
}

func (w *tableWriter) actions(v []ParseActionEntry) {
	w.u32(uint32(len(v)))
	for _, x := range v {
		a := x.Action
		var bits uint8
		if a.Extra {
			bits |= 1
		}
		if a.Repetition {
			bits |= 2
		}
		if x.Reusable {
			bits |= 4
		}
		w.u8(a.Type)
		w.u8(bits)
		w.u8(a.ChildCount)
		w.u8(x.Count)
		w.u16(uint16(a.State))
		w.u16(uint16(a.Symbol))
		w.u16(uint16(a.DynamicPrecedence))
		w.u16(a.ProductionID)
	}
}

func (w *tableWriter) charSets(v [][]CharacterRange) {
	w.u32(uint32(len(v)))
	for _, set := range v {
		w.u32(uint32(len(set)))
		for _, rng := range set {
			w.u32(uint32(rng.Start))
			w.u32(uint32(rng.End))
		}
	}
}

func boolByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

type tableReader struct {
	buf []byte
	pos int
	err error
}

// fail records the first error. Every later read then returns a zero value, so
// a truncated blob cannot panic on the way to the report.
func (r *tableReader) fail(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf(format, args...)
	}
}

func (r *tableReader) raw(n int) []byte {
	if r.err != nil {
		return make([]byte, n)
	}
	if r.pos+n > len(r.buf) {
		r.fail("the table blob ends after %d bytes, and a read wanted %d more", len(r.buf), n)
		return make([]byte, n)
	}
	out := r.buf[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *tableReader) u8() uint8 { return r.raw(1)[0] }

func (r *tableReader) u16() uint16 { return binary.LittleEndian.Uint16(r.raw(2)) }

func (r *tableReader) u32() uint32 { return binary.LittleEndian.Uint32(r.raw(4)) }

// count reads a length and rejects one the remaining bytes cannot hold, so a
// corrupt blob cannot ask for an enormous allocation.
func (r *tableReader) count(width int) int {
	n := int(r.u32())
	if r.err != nil {
		return 0
	}
	if n < 0 || n > math.MaxInt32 || n*width > len(r.buf)-r.pos {
		r.fail("the table blob claims %d entries, which its remaining bytes cannot hold", n)
		return 0
	}
	return n
}

func (r *tableReader) str() string {
	n := r.count(1)
	return string(r.raw(n))
}

func (r *tableReader) strs() []string {
	n := r.count(4)
	if n == 0 {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		out[i] = r.str()
	}
	return out
}

func (r *tableReader) u16s() []uint16 {
	n := r.count(2)
	if n == 0 {
		return nil
	}
	out := make([]uint16, n)
	for i := range out {
		out[i] = r.u16()
	}
	return out
}

func (r *tableReader) u32s() []uint32 {
	n := r.count(4)
	if n == 0 {
		return nil
	}
	out := make([]uint32, n)
	for i := range out {
		out[i] = r.u32()
	}
	return out
}

func (r *tableReader) symbols() []Symbol {
	n := r.count(2)
	if n == 0 {
		return nil
	}
	out := make([]Symbol, n)
	for i := range out {
		out[i] = Symbol(r.u16())
	}
	return out
}

func (r *tableReader) states() []StateID {
	n := r.count(2)
	if n == 0 {
		return nil
	}
	out := make([]StateID, n)
	for i := range out {
		out[i] = StateID(r.u16())
	}
	return out
}

func (r *tableReader) bools() []bool {
	n := r.count(1)
	if n == 0 {
		return nil
	}
	out := make([]bool, n)
	for i := range out {
		out[i] = r.u8() != 0
	}
	return out
}

func (r *tableReader) slices() []MapSlice {
	n := r.count(4)
	if n == 0 {
		return nil
	}
	out := make([]MapSlice, n)
	for i := range out {
		out[i].Index = r.u16()
		out[i].Length = r.u16()
	}
	return out
}

func (r *tableReader) fieldEntries() []FieldMapEntry {
	n := r.count(4)
	if n == 0 {
		return nil
	}
	out := make([]FieldMapEntry, n)
	for i := range out {
		out[i].FieldID = FieldID(r.u16())
		out[i].ChildIndex = r.u8()
		out[i].Inherited = r.u8() != 0
	}
	return out
}

func (r *tableReader) metadata() []SymbolMetadata {
	n := r.count(1)
	if n == 0 {
		return nil
	}
	out := make([]SymbolMetadata, n)
	for i := range out {
		bits := r.u8()
		out[i] = SymbolMetadata{
			Visible:   bits&1 != 0,
			Named:     bits&2 != 0,
			Supertype: bits&4 != 0,
		}
	}
	return out
}

func (r *tableReader) lexModes() []LexerMode {
	n := r.count(6)
	if n == 0 {
		return nil
	}
	out := make([]LexerMode, n)
	for i := range out {
		out[i].LexState = r.u16()
		out[i].ExternalLexState = r.u16()
		out[i].ReservedWordSetID = r.u16()
	}
	return out
}

func (r *tableReader) actions() []ParseActionEntry {
	n := r.count(12)
	if n == 0 {
		return nil
	}
	out := make([]ParseActionEntry, n)
	for i := range out {
		typ := r.u8()
		bits := r.u8()
		childCount := r.u8()
		count := r.u8()
		state := r.u16()
		symbol := r.u16()
		precedence := r.u16()
		production := r.u16()
		out[i] = ParseActionEntry{
			Action: ParseAction{
				Type:              typ,
				State:             StateID(state),
				Extra:             bits&1 != 0,
				Repetition:        bits&2 != 0,
				ChildCount:        childCount,
				Symbol:            Symbol(symbol),
				DynamicPrecedence: int16(precedence),
				ProductionID:      production,
			},
			Count:    count,
			Reusable: bits&4 != 0,
		}
	}
	return out
}

func (r *tableReader) charSets() [][]CharacterRange {
	n := r.count(4)
	if n == 0 {
		return nil
	}
	out := make([][]CharacterRange, n)
	for i := range out {
		size := r.count(8)
		if size == 0 {
			continue
		}
		set := make([]CharacterRange, size)
		for j := range set {
			set[j].Start = int32(r.u32())
			set[j].End = int32(r.u32())
		}
		out[i] = set
	}
	return out
}
