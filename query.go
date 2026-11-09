// query.go holds the compiled query and its public accessors.
//
// This file. The other query_*.go files port lib/src/query.c from tree-sitter
// v0.27.2 fb7be49145acfb07e33b61ed5689e2b6771b2e9). Each function names the
// upstream function it ports in its doc comment.

package treesitter

import (
	"fmt"
	"strings"
)

// Upstream limits. MAX_STEP_CAPTURE_COUNT and the others in query.c.
const (
	maxStepCaptureCount        = 3
	maxNegatedFieldCount       = 8
	maxStatePredecessorCount   = 256
	maxAnalysisStateDepth      = 8
	maxAnalysisIterationCount  = 256
	patternDoneMarker          = uint16(0xFFFF)
	noneIndex                  = uint16(0xFFFF)
	captureListNone            = uint32(0xFFFFFFFF)
	wildcardSymbol             = Symbol(0)
	workPerQueryCallbackCheck  = 1000
	languageVersionWithQueries = languageVersionWithReservedWords
)

// QueryErrorKind says which rule a query broke. The values follow TSQueryError.
type QueryErrorKind int

// The error kinds a query compile can report. QueryErrorPredicate comes from the
// predicate layer the Rust binding adds; the others come from ts_query_new.
const (
	QueryErrorNone QueryErrorKind = iota
	QueryErrorSyntax
	QueryErrorNodeType
	QueryErrorField
	QueryErrorCapture
	QueryErrorStructure
	QueryErrorLanguage
	QueryErrorPredicate
)

// String names the kind the way the Rust binding's QueryErrorKind does.
func (k QueryErrorKind) String() string {
	switch k {
	case QueryErrorNone:
		return "None"
	case QueryErrorSyntax:
		return "Syntax"
	case QueryErrorNodeType:
		return "NodeType"
	case QueryErrorField:
		return "Field"
	case QueryErrorCapture:
		return "Capture"
	case QueryErrorStructure:
		return "Structure"
	case QueryErrorLanguage:
		return "Language"
	case QueryErrorPredicate:
		return "Predicate"
	default:
		return fmt.Sprintf("QueryErrorKind(%d)", int(k))
	}
}

// QueryError reports why a query did not compile. Offset is the byte offset
// ts_query_new reports.
type QueryError struct {
	Kind    QueryErrorKind
	Offset  int
	Row     int
	Column  int
	Message string
}

// Error renders the error the way the Rust binding's Display does.
func (e *QueryError) Error() string {
	msg := e.Message
	switch e.Kind {
	case QueryErrorField:
		msg = "Invalid field name " + msg
	case QueryErrorNodeType:
		msg = "Invalid node type " + msg
	case QueryErrorCapture:
		msg = "Invalid capture name " + msg
	case QueryErrorPredicate:
		msg = "Invalid predicate: " + msg
	case QueryErrorStructure:
		msg = "Impossible pattern:\n" + msg
	case QueryErrorSyntax:
		msg = "Invalid syntax:\n" + msg
	case QueryErrorLanguage:
	}
	if e.Kind != QueryErrorLanguage {
		return fmt.Sprintf("Query error at %d:%d. %s", e.Row+1, e.Column+1, msg)
	}
	return msg
}

// Quantifier says how many times a capture can appear in a pattern. The values follow TSQuantifier.
type Quantifier uint8

// The quantifiers a capture can carry.
const (
	QuantifierZero Quantifier = iota
	QuantifierZeroOrOne
	QuantifierZeroOrMore
	QuantifierOne
	QuantifierOneOrMore
)

// String names the quantifier the way the Rust binding's CaptureQuantifier does.
func (q Quantifier) String() string {
	switch q {
	case QuantifierZero:
		return "Zero"
	case QuantifierZeroOrOne:
		return "ZeroOrOne"
	case QuantifierZeroOrMore:
		return "ZeroOrMore"
	case QuantifierOne:
		return "One"
	case QuantifierOneOrMore:
		return "OneOrMore"
	default:
		return fmt.Sprintf("Quantifier(%d)", uint8(q))
	}
}

// PredicateStepType says what a predicate step holds. The values follow. TSQueryPredicateStepType.
type PredicateStepType uint8

// The step types. A Done step ends one predicate.
const (
	PredicateStepDone PredicateStepType = iota
	PredicateStepCapture
	PredicateStepString
)

// PredicateStep is one token of a predicate, as TSQueryPredicateStep. A
// Capture step's ValueID is a capture id.
type PredicateStep struct {
	Type    PredicateStepType
	ValueID uint32
}

// QueryCapture is a node a pattern captured, as TSQueryCapture. Index is the
// capture id, for CaptureNameForID.
type QueryCapture struct {
	Node  Node
	Index uint32
}

// QueryMatch is one match of a pattern, as TSQueryMatch. Captures aliases a
// list the cursor owns: it is valid until the cursor's next call.
type QueryMatch struct {
	ID           uint32
	PatternIndex uint16
	Captures     []QueryCapture
}

// queryStep is a QueryStep: one node of a pattern's S-expression.
type queryStep struct {
	symbol                  Symbol
	supertypeSymbol         Symbol
	field                   FieldID
	captureIDs              [maxStepCaptureCount]uint16
	depth                   uint16
	alternativeIndex        uint16
	negatedFieldListID      uint16
	isNamed                 bool
	isImmediate             bool
	isLastChild             bool
	isPassThrough           bool
	isDeadEnd               bool
	isInsideAlternation     bool
	containsCaptures        bool
	rootPatternGuaranteed   bool
	parentPatternGuaranteed bool
	isMissing               bool
	alternativeIsSkip       bool
}

// patternEntry is a PatternEntry: where a pattern's matching starts.
type patternEntry struct {
	stepIndex    uint16
	patternIndex uint16
	isRooted     bool
}

// queryPattern is a QueryPattern.
type queryPattern struct {
	stepsOffset          uint32
	stepsLength          uint32
	predicateStepsOffset uint32
	predicateStepsLength uint32
	startByte            uint32
	endByte              uint32
	isNonLocal           bool
}

// stepOffset is a StepOffset: the byte offset of a step in the source.
type stepOffset struct {
	byteOffset uint32
	stepIndex  uint16
}

// symbolTable is a SymbolTable: a way mapping of strings to ids.
type symbolTable struct {
	names []string
}

// idForName is symbol_table_id_for_name.
func (t *symbolTable) idForName(name string) int {
	for i, n := range t.names {
		if n == name {
			return i
		}
	}
	return -1
}

// insertName is symbol_table_insert_name.
func (t *symbolTable) insertName(name string) uint16 {
	if id := t.idForName(name); id >= 0 {
		return uint16(id)
	}
	t.names = append(t.names, name)
	return uint16(len(t.names) - 1)
}

// Query is a TSQuery: patterns compiled from S-expressions. A Query is
// immutable during execution, so cursors can share it.
type Query struct {
	captures                          symbolTable
	predicateValues                   symbolTable
	captureQuantifiers                [][]uint8
	steps                             []queryStep
	patternMap                        []patternEntry
	predicateSteps                    []PredicateStep
	patterns                          []queryPattern
	stepOffsets                       []stepOffset
	negatedFields                     []FieldID
	stringBuffer                      []byte
	repeatSymbolsWithRootlessPatterns []Symbol
	language                          *Language
	wildcardRootPatternCount          uint16

	// The predicate layer the Rust binding adds on top of query.c.
	textPredicates     [][]textPredicate
	propertySettings   [][]QueryProperty
	propertyPredicates [][]PropertyPredicate
	generalPredicates  [][]QueryPredicate
}

// newQueryStep is query_step__new.
func newQueryStep(symbol Symbol, depth uint16, isImmediate bool) queryStep {
	step := queryStep{
		symbol:           symbol,
		depth:            depth,
		alternativeIndex: noneIndex,
		isImmediate:      isImmediate,
	}
	for i := range step.captureIDs {
		step.captureIDs[i] = noneIndex
	}
	return step
}

// addCapture is query_step__add_capture.
func (s *queryStep) addCapture(captureID uint16) {
	for i := range s.captureIDs {
		if s.captureIDs[i] == noneIndex {
			s.captureIDs[i] = captureID
			break
		}
	}
}

// removeCapture is query_step__remove_capture.
func (s *queryStep) removeCapture(captureID uint16) {
	for i := 0; i < maxStepCaptureCount; i++ {
		if s.captureIDs[i] == captureID {
			s.captureIDs[i] = noneIndex
			for i+1 < maxStepCaptureCount {
				if s.captureIDs[i+1] == noneIndex {
					break
				}
				s.captureIDs[i] = s.captureIDs[i+1]
				s.captureIDs[i+1] = noneIndex
				i++
			}
			break
		}
	}
}

// patternMapSearch is ts_query__pattern_map_search. It reports whether a
// pattern starts with the symbol, and where one would be inserted otherwise.
func (q *Query) patternMapSearch(needle Symbol) (uint32, bool) {
	baseIndex := uint32(q.wildcardRootPatternCount)
	size := uint32(len(q.patternMap)) - baseIndex
	if size == 0 {
		return baseIndex, false
	}
	for size > 1 {
		halfSize := size / 2
		midIndex := baseIndex + halfSize
		midSymbol := q.steps[q.patternMap[midIndex].stepIndex].symbol
		if needle > midSymbol {
			baseIndex = midIndex
		}
		size -= halfSize
	}
	symbol := q.steps[q.patternMap[baseIndex].stepIndex].symbol
	if needle > symbol {
		baseIndex++
		if baseIndex < uint32(len(q.patternMap)) {
			symbol = q.steps[q.patternMap[baseIndex].stepIndex].symbol
		}
	}
	return baseIndex, needle == symbol
}

// patternMapInsert is ts_query__pattern_map_insert.
func (q *Query) patternMapInsert(symbol Symbol, entry patternEntry) {
	index, _ := q.patternMapSearch(symbol)
	for index < uint32(len(q.patternMap)) {
		e := q.patternMap[index]
		if q.steps[e.stepIndex].symbol == symbol && e.patternIndex < entry.patternIndex {
			index++
		} else {
			break
		}
	}
	q.patternMap = append(q.patternMap, patternEntry{})
	copy(q.patternMap[index+1:], q.patternMap[index:])
	q.patternMap[index] = entry
}

// NewQuery is ts_query_new, plus the predicate parsing the Rust binding's
// Query::new performs. The error is a *QueryError.
func NewQuery(language *Language, source string) (*Query, error) {
	if language == nil || language.ABIVersion > MaxABIVersion || language.ABIVersion < MinABIVersion {
		version := uint32(0)
		if language != nil {
			version = language.ABIVersion
		}
		return nil, &QueryError{
			Kind: QueryErrorLanguage,
			Message: fmt.Sprintf(
				"Incompatible language version %d. Expected minimum %d, maximum %d",
				version, MinABIVersion, MaxABIVersion),
		}
	}

	q := &Query{language: language}
	q.negatedFields = append(q.negatedFields, 0)

	stream := newStream(source)
	stream.skipWhitespace()
	for stream.input < stream.end {
		patternIndex := len(q.patterns)
		startStepIndex := uint32(len(q.steps))
		startPredicateStepIndex := uint32(len(q.predicateSteps))
		q.patterns = append(q.patterns, queryPattern{
			stepsOffset:          startStepIndex,
			predicateStepsOffset: startPredicateStepIndex,
			startByte:            stream.offset(),
		})
		var quantifiers captureQuantifiers
		errType := q.parsePattern(&stream, 0, false, false, &quantifiers)
		q.steps = append(q.steps, newQueryStep(0, patternDoneMarker, false))

		pattern := &q.patterns[len(q.patterns)-1]
		pattern.stepsLength = uint32(len(q.steps)) - startStepIndex
		pattern.predicateStepsLength = uint32(len(q.predicateSteps)) - startPredicateStepIndex
		pattern.endByte = stream.offset()

		if errType != QueryErrorNone {
			if errType == parentDone {
				errType = QueryErrorSyntax
			}
			return nil, newQueryError(source, errType, int(stream.offset()))
		}

		q.captureQuantifiers = append(q.captureQuantifiers, quantifiers)

		wildcardRootAlternativeIndex := noneIndex
		for {
			step := &q.steps[startStepIndex]

			// A wildcard root with a non-wildcard child: skip matching the
			// wildcard, and check the parent at match time instead.
			if step.symbol == wildcardSymbol && step.depth == 0 && step.field == 0 {
				secondStep := &q.steps[startStepIndex+1]
				if secondStep.symbol != wildcardSymbol && secondStep.depth == 1 && !secondStep.isImmediate {
					wildcardRootAlternativeIndex = step.alternativeIndex
					startStepIndex++
					step = secondStep
				}
			}

			startDepth := step.depth
			isRooted := startDepth == 0
			for stepIndex := startStepIndex + 1; stepIndex < uint32(len(q.steps)); stepIndex++ {
				childStep := &q.steps[stepIndex]
				if childStep.isDeadEnd {
					break
				}
				if childStep.depth == startDepth {
					isRooted = false
					break
				}
			}

			q.patternMapInsert(step.symbol, patternEntry{
				stepIndex:    uint16(startStepIndex),
				patternIndex: uint16(patternIndex),
				isRooted:     isRooted,
			})
			if step.symbol == wildcardSymbol {
				q.wildcardRootPatternCount++
			}

			if step.alternativeIndex != noneIndex {
				startStepIndex = uint32(step.alternativeIndex)
			} else if wildcardRootAlternativeIndex != noneIndex {
				startStepIndex = uint32(wildcardRootAlternativeIndex)
				wildcardRootAlternativeIndex = noneIndex
			} else {
				break
			}
		}

		// A quantified branch of an alternation loops back to a copy of its first step that has no link to the next branch.
		patStart := pattern.stepsOffset
		patEnd := patStart + pattern.stepsLength - 1
		for i := patStart; i < patEnd; i++ {
			s := &q.steps[i]
			if !s.isPassThrough || !s.isInsideAlternation ||
				s.alternativeIndex == noneIndex || uint32(s.alternativeIndex) >= i {
				continue
			}
			targetIdx := uint32(s.alternativeIndex)
			target := &q.steps[targetIdx]
			targetAltIndex := target.alternativeIndex
			if targetAltIndex == noneIndex ||
				uint32(targetAltIndex) <= targetIdx || uint32(targetAltIndex) >= patEnd {
				continue
			}
			copyIdx := uint32(len(q.steps))
			stepCopy := *target
			stepCopy.alternativeIndex = noneIndex
			targetDepth := target.depth
			q.steps = append(q.steps, stepCopy)

			redirect := newQueryStep(0, targetDepth, false)
			redirect.isDeadEnd = true
			redirect.alternativeIndex = uint16(targetIdx + 1)
			q.steps = append(q.steps, redirect)

			q.steps[i].alternativeIndex = uint16(copyIdx)
		}
	}

	if offset, ok := q.analyzePatterns(); !ok {
		return nil, newQueryError(source, QueryErrorStructure, int(offset))
	}
	q.stringBuffer = nil

	if err := q.parsePredicates(source); err != nil {
		return nil, err
	}
	return q, nil
}

// newQueryError builds the error the Rust binding's Query::new_raw builds from
// an error type and offset: a row. A column, and a message that quotes the
// offending name or points at the offending line.
func newQueryError(source string, kind QueryErrorKind, offset int) *QueryError {
	lineStart := 0
	row := 0
	lineContainingError := ""
	hasLine := false
	for _, line := range strings.SplitAfter(source, "\n") {
		if line == "" {
			continue
		}
		lineEnd := lineStart + len(line)
		if !strings.HasSuffix(line, "\n") {
			lineEnd++
		}
		if lineEnd > offset {
			lineContainingError = strings.TrimSuffix(line, "\n")
			hasLine = true
			break
		}
		lineStart = lineEnd
		row++
	}
	column := offset - lineStart

	var message string
	switch kind {
	case QueryErrorNodeType, QueryErrorField, QueryErrorCapture:
		suffix := source[offset:]
		inQuotes := offset > 0 && source[offset-1] == '"'
		backslashes := 0
		endOffset := len(suffix)
		for i, c := range suffix {
			var found bool
			if inQuotes {
				if c == '"' && backslashes%2 == 0 {
					found = true
				} else if c == '\\' {
					backslashes++
				} else {
					backslashes = 0
				}
			} else {
				found = !isAlphanumeric(c) && c != '_' && c != '-'
			}
			if found {
				endOffset = i
				break
			}
		}
		message = "\"" + suffix[:endOffset] + "\""
	default:
		if hasLine {
			message = lineContainingError + "\n" + strings.Repeat(" ", offset-lineStart) + "^"
		} else {
			message = "Unexpected EOF"
		}
	}
	return &QueryError{Kind: kind, Offset: offset, Row: row, Column: column, Message: message}
}
