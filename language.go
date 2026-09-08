package treesitter

// Symbol identifies a terminal or non-terminal in a grammar.
type Symbol = uint16

// StateID identifies a parse state.
type StateID = uint16

// FieldID identifies a field name in a grammar.
type FieldID = uint16

// BuiltinSymEnd is the symbol for the end of input.
const BuiltinSymEnd Symbol = 0

// BuiltinSymError is the symbol for an error node.
const BuiltinSymError Symbol = 0xFFFF

const builtinSymErrorRepeat Symbol = BuiltinSymError - 1

const treeStateNone StateID = 0xFFFF

// Parse action types.
const (
	ParseActionTypeShift uint8 = iota
	ParseActionTypeReduce
	ParseActionTypeAccept
	ParseActionTypeRecover
)

// FieldMapEntry associates a field name with a child position.
type FieldMapEntry struct {
	FieldID    FieldID
	ChildIndex uint8
	Inherited  bool
}

// MapSlice indexes into the field and supertype maps.
type MapSlice struct {
	Index  uint16
	Length uint16
}

// SymbolMetadata records how a symbol appears in a tree.
type SymbolMetadata struct {
	Visible   bool
	Named     bool
	Supertype bool
}

// ParseAction is one entry of a parse table cell.
type ParseAction struct {
	Type              uint8
	State             StateID
	Extra             bool
	Repetition        bool
	ChildCount        uint8
	Symbol            Symbol
	DynamicPrecedence int16
	ProductionID      uint16
}

// ParseActionEntry is either a group header or a single action.
type ParseActionEntry struct {
	Action   ParseAction
	Count    uint8
	Reusable bool
}

// LexerMode selects the lex state used when scanning in a parse state.
type LexerMode struct {
	LexState          uint16
	ExternalLexState  uint16
	ReservedWordSetID uint16
}

// CharacterRange is an inclusive range of code points.
type CharacterRange struct {
	Start int32
	End   int32
}

// LanguageMetadata is a grammar's semantic version.
type LanguageMetadata struct {
	MajorVersion uint8
	MinorVersion uint8
	PatchVersion uint8
}

// ExternalScanner is the set of callbacks a hand written scanner provides.
type ExternalScanner interface {
	Create() any
	Destroy(payload any)
	Scan(payload any, lexer *Lexer, validSymbols []bool) bool
	Serialize(payload any, buffer []byte) uint32
	Deserialize(payload any, buffer []byte)
}

// Language holds a grammar's parse tables and lexer functions.
type Language struct {
	ABIVersion            uint32
	SymbolCount           uint32
	AliasCount            uint32
	TokenCount            uint32
	ExternalTokenCount    uint32
	StateCount            uint32
	LargeStateCount       uint32
	ProductionIDCount     uint32
	FieldCount            uint32
	MaxAliasSequenceLen   uint16
	ParseTable            []uint16
	SmallParseTable       []uint16
	SmallParseTableMap    []uint32
	ParseActions          []ParseActionEntry
	SymbolNames           []string
	FieldNames            []string
	FieldMapSlices        []MapSlice
	FieldMapEntries       []FieldMapEntry
	SymbolMetadataTable   []SymbolMetadata
	PublicSymbolMap       []Symbol
	AliasMap              []uint16
	AliasSequences        []Symbol
	LexModes              []LexerMode
	LexFn                 func(*Lexer, StateID) bool
	KeywordLexFn          func(*Lexer, StateID) bool
	KeywordCaptureToken   Symbol
	ExternalScannerStates []bool
	ExternalScannerSymbol []Symbol
	Scanner               ExternalScanner
	PrimaryStateIDs       []StateID
	Name                  string
	ReservedWords         []Symbol
	MaxReservedWordSetLen uint16
	SupertypeCount        uint32
	SupertypeSymbols      []Symbol
	SupertypeMapSlices    []MapSlice
	SupertypeMapEntries   []Symbol
	Metadata              LanguageMetadata
}

const languageVersionWithReservedWords = 15

const languageVersionWithPrimaryStates = 14

// MinABIVersion and MaxABIVersion bound the grammars this runtime parses with.
// A generator writes tables for one ABI. It must refuse an ABI outside this
// range, because a table the runtime rejects is a grammar that parses nothing.
const (
	MinABIVersion = 13
	MaxABIVersion = 15
)
type tableEntry struct {
	actions     []ParseActionEntry
	actionCount uint32
	isReusable  bool
}

func (l *Language) lookup(state StateID, symbol Symbol) uint16 {
	if uint32(state) >= l.LargeStateCount {
		index := l.SmallParseTableMap[uint32(state)-l.LargeStateCount]
		data := l.SmallParseTable[index:]
		pos := 0
		groupCount := data[pos]
		pos++
		for i := uint16(0); i < groupCount; i++ {
			sectionValue := data[pos]
			pos++
			symbolCount := data[pos]
			pos++
			for j := uint16(0); j < symbolCount; j++ {
				if data[pos] == symbol {
					return sectionValue
				}
				pos++
			}
		}
		return 0
	}
	return l.ParseTable[uint32(state)*l.SymbolCount+uint32(symbol)]
}

func (l *Language) hasActions(state StateID, symbol Symbol) bool {
	return l.lookup(state, symbol) != 0
}

func (l *Language) actionsAt(index uint16) ([]ParseActionEntry, uint8, bool) {
	entry := l.ParseActions[index]
	count := uint32(entry.Count)
	base := uint32(index) + 1
	return l.ParseActions[base : base+count], entry.Count, entry.Reusable
}

func (l *Language) tableEntry(state StateID, symbol Symbol, result *tableEntry) {
	if symbol == BuiltinSymError || symbol == builtinSymErrorRepeat {
		result.actionCount = 0
		result.isReusable = false
		result.actions = nil
		return
	}
	index := l.lookup(state, symbol)
	actions, count, reusable := l.actionsAt(index)
	result.actions = actions
	result.actionCount = uint32(count)
	result.isReusable = reusable
}

func (l *Language) actions(state StateID, symbol Symbol) []ParseActionEntry {
	var entry tableEntry
	l.tableEntry(state, symbol, &entry)
	return entry.actions
}

func (l *Language) hasReduceAction(state StateID, symbol Symbol) bool {
	var entry tableEntry
	l.tableEntry(state, symbol, &entry)
	return entry.actionCount > 0 && entry.actions[0].Action.Type == ParseActionTypeReduce
}

func (l *Language) lexModeForState(state StateID) LexerMode {
	return l.LexModes[state]
}

func (l *Language) isReservedWord(state StateID, symbol Symbol) bool {
	mode := l.lexModeForState(state)
	if mode.ReservedWordSetID > 0 {
		start := uint32(mode.ReservedWordSetID) * uint32(l.MaxReservedWordSetLen)
		end := start + uint32(l.MaxReservedWordSetLen)
		for i := start; i < end; i++ {
			if l.ReservedWords[i] == symbol {
				return true
			}
			if l.ReservedWords[i] == 0 {
				break
			}
		}
	}
	return false
}

func (l *Language) symbolMetadata(symbol Symbol) SymbolMetadata {
	switch symbol {
	case BuiltinSymError:
		return SymbolMetadata{Visible: true, Named: true}
	case builtinSymErrorRepeat:
		return SymbolMetadata{}
	default:
		return l.SymbolMetadataTable[symbol]
	}
}

func (l *Language) publicSymbol(symbol Symbol) Symbol {
	if symbol == BuiltinSymError {
		return symbol
	}
	return l.PublicSymbolMap[symbol]
}

func (l *Language) enabledExternalTokens(externalScannerState uint32) []bool {
	if externalScannerState == 0 {
		return nil
	}
	start := l.ExternalTokenCount * externalScannerState
	return l.ExternalScannerStates[start : start+l.ExternalTokenCount]
}

func (l *Language) aliasSequence(productionID uint32) []Symbol {
	if productionID == 0 {
		return nil
	}
	start := productionID * uint32(l.MaxAliasSequenceLen)
	return l.AliasSequences[start : start+uint32(l.MaxAliasSequenceLen)]
}

// AliasAt reports the alias applied to one child of a production.
func (l *Language) AliasAt(productionID uint32, childIndex uint32) Symbol {
	if productionID == 0 {
		return 0
	}
	return l.AliasSequences[productionID*uint32(l.MaxAliasSequenceLen)+childIndex]
}

func (l *Language) fieldMap(productionID uint32) []FieldMapEntry {
	if l.FieldCount == 0 {
		return nil
	}
	slice := l.FieldMapSlices[productionID]
	return l.FieldMapEntries[slice.Index : uint32(slice.Index)+uint32(slice.Length)]
}

// AliasesForSymbol reports every public name a symbol can take.
func (l *Language) AliasesForSymbol(originalSymbol Symbol) []Symbol {
	result := l.PublicSymbolMap[originalSymbol : uint32(originalSymbol)+1]
	idx := uint32(0)
	for {
		symbol := l.AliasMap[idx]
		idx++
		if symbol == 0 || symbol > originalSymbol {
			break
		}
		count := uint32(l.AliasMap[idx])
		idx++
		if symbol == originalSymbol {
			return l.AliasMap[idx : idx+count]
		}
		idx += count
	}
	return result
}

// StateIsPrimary reports whether a state is the canonical one of its class.
func (l *Language) StateIsPrimary(state StateID) bool {
	if l.ABIVersion >= languageVersionWithPrimaryStates {
		return state == l.PrimaryStateIDs[state]
	}
	return true
}

// LanguageName reports the grammar's own name.
func (l *Language) LanguageName() string {
	if l.ABIVersion >= languageVersionWithReservedWords {
		return l.Name
	}
	return ""
}

// Supertypes reports the grammar's supertype symbols.
func (l *Language) Supertypes() []Symbol {
	if l.ABIVersion >= languageVersionWithReservedWords {
		return l.SupertypeSymbols
	}
	return nil
}

// Subtypes reports the symbols that a supertype covers.
func (l *Language) Subtypes(supertype Symbol) []Symbol {
	if l.ABIVersion < languageVersionWithReservedWords ||
		uint32(supertype) >= l.SymbolCountTotal() ||
		!l.symbolMetadata(supertype).Supertype {
		return nil
	}
	slice := l.SupertypeMapSlices[supertype]
	return l.SupertypeMapEntries[slice.Index : uint32(slice.Index)+uint32(slice.Length)]
}

// SymbolCountTotal reports the number of symbols including aliases.
func (l *Language) SymbolCountTotal() uint32 { return l.SymbolCount + l.AliasCount }

// SymbolName returns the display name for a symbol.
func (l *Language) SymbolName(symbol Symbol) string {
	switch {
	case symbol == BuiltinSymError:
		return "ERROR"
	case symbol == builtinSymErrorRepeat:
		return "_ERROR"
	case uint32(symbol) < l.SymbolCountTotal():
		return l.SymbolNames[symbol]
	default:
		return ""
	}
}

// FieldNameForID returns a field's name.
func (l *Language) FieldNameForID(id FieldID) string {
	count := l.FieldCount
	if count != 0 && uint32(id) <= count {
		return l.FieldNames[id]
	}
	return ""
}

// FieldIDForName returns the identifier of a named field.
func (l *Language) FieldIDForName(name string) FieldID {
	count := uint16(l.FieldCount)
	for i := FieldID(1); i < count+1; i++ {
		if l.FieldNames[i] == name {
			return i
		}
	}
	return 0
}

// SymbolForName finds a symbol by its display name.
func (l *Language) SymbolForName(name string, isNamed bool) Symbol {
	if isNamed && name == "ERROR" {
		return BuiltinSymError
	}
	count := uint16(l.SymbolCountTotal())
	for i := Symbol(0); i < count; i++ {
		metadata := l.symbolMetadata(i)
		if (!metadata.Visible && !metadata.Supertype) || metadata.Named != isNamed {
			continue
		}
		if l.SymbolNames[i] == name {
			return l.PublicSymbolMap[i]
		}
	}
	return 0
}

func (l *Language) nextState(state StateID, symbol Symbol) StateID {
	if symbol == BuiltinSymError || symbol == builtinSymErrorRepeat ||
		uint32(symbol) >= l.SymbolCount || uint32(state) >= l.StateCount {
		return 0
	}
	if uint32(symbol) < l.TokenCount {
		actions := l.actions(state, symbol)
		if len(actions) > 0 {
			action := actions[len(actions)-1].Action
			if action.Type == ParseActionTypeShift {
				if action.Extra {
					return state
				}
				return action.State
			}
		}
		return 0
	}
	return l.lookup(state, symbol)
}

// SetContains reports whether a code point falls in one of the sorted ranges.
func SetContains(ranges []CharacterRange, lookahead int32) bool {
	index := 0
	size := len(ranges) - index
	for size > 1 {
		halfSize := size / 2
		midIndex := index + halfSize
		r := ranges[midIndex]
		if lookahead >= r.Start && lookahead <= r.End {
			return true
		} else if lookahead > r.End {
			index = midIndex
		}
		size -= halfSize
	}
	r := ranges[index]
	return lookahead >= r.Start && lookahead <= r.End
}
