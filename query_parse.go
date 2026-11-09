// query_parse.go holds the S-expression reader of query.c (v0.27.2).

package treesitter

// parentDone is PARENT_DONE: the parse reached its parent's closing bracket.
const parentDone QueryErrorKind = -1

// stream is a Stream: a reader of code points over a UTF-8 string.
type stream struct {
	source   string
	input    int
	end      int
	next     int32
	nextSize uint8
}

// newStream is stream_new.
func newStream(source string) stream {
	s := stream{source: source, end: len(source)}
	s.advance()
	return s
}

// advance is stream_advance.
func (s *stream) advance() bool {
	s.input += int(s.nextSize)
	if s.input < s.end {
		code, size := DecodeUTF8([]byte(s.source[s.input:]))
		if size > 0 {
			s.next = code
			s.nextSize = uint8(size)
			return true
		}
	} else {
		s.nextSize = 0
		s.next = 0
	}
	return false
}

// reset is stream_reset.
func (s *stream) reset(input int) {
	s.input = input
	s.nextSize = 0
	s.advance()
}

// isSpace is iswspace in the C locale, which the CLI runs under.
func isSpace(c int32) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// isAlphanumeric is iswalnum in the C locale, which the CLI runs under.
func isAlphanumeric(c int32) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// skipWhitespace is stream_skip_whitespace. A `;` starts a comment.
func (s *stream) skipWhitespace() {
	for {
		if isSpace(s.next) {
			s.advance()
		} else if s.next == ';' {
			s.advance()
			for s.next != 0 && s.next != '\n' {
				if !s.advance() {
					break
				}
			}
		} else {
			break
		}
	}
}

// isIdentStart is stream_is_ident_start.
func (s *stream) isIdentStart() bool {
	return isAlphanumeric(s.next) || s.next == '_' || s.next == '-'
}

// scanIdentifier is stream_scan_identifier.
func (s *stream) scanIdentifier() {
	for {
		s.advance()
		if !(isAlphanumeric(s.next) || s.next == '_' || s.next == '-' || s.next == '.') {
			break
		}
	}
}

// offset is stream_offset.
func (s *stream) offset() uint32 { return uint32(s.input) }

// addNegatedFields is ts_query__add_negated_fields. The negated field array
// stores lists of field ids separated by zeros, and a step reuses a list that
// already holds its fields.
func (q *Query) addNegatedFields(stepIndex uint32, fieldIDs []FieldID) {
	step := &q.steps[stepIndex]
	fieldCount := len(fieldIDs)
	failedMatch := false
	matchCount := 0
	startI := 0
	for i, existing := range q.negatedFields {
		switch {
		case existing == 0:
			if matchCount == fieldCount {
				step.negatedFieldListID = uint16(startI)
				return
			}
			startI = i + 1
			matchCount = 0
			failedMatch = false
		case matchCount < fieldCount && existing == fieldIDs[matchCount] && !failedMatch:
			matchCount++
		default:
			matchCount = 0
			failedMatch = true
		}
	}
	step.negatedFieldListID = uint16(len(q.negatedFields))
	q.negatedFields = append(q.negatedFields, fieldIDs...)
	q.negatedFields = append(q.negatedFields, 0)
}

// parseStringLiteral is ts_query__parse_string_literal. It leaves the
// unescaped text in the query's string buffer.
func (q *Query) parseStringLiteral(s *stream) QueryErrorKind {
	stringStart := s.input
	if s.next != '"' {
		return QueryErrorSyntax
	}
	s.advance()
	prevPosition := s.input

	isEscaped := false
	q.stringBuffer = q.stringBuffer[:0]
	for {
		if isEscaped {
			isEscaped = false
			switch s.next {
			case 'n':
				q.stringBuffer = append(q.stringBuffer, '\n')
			case 'r':
				q.stringBuffer = append(q.stringBuffer, '\r')
			case 't':
				q.stringBuffer = append(q.stringBuffer, '\t')
			case '0':
				q.stringBuffer = append(q.stringBuffer, 0)
			default:
				q.stringBuffer = append(q.stringBuffer, s.source[s.input:s.input+int(s.nextSize)]...)
			}
			prevPosition = s.input + int(s.nextSize)
		} else {
			switch s.next {
			case '\\':
				q.stringBuffer = append(q.stringBuffer, s.source[prevPosition:s.input]...)
				prevPosition = s.input + 1
				isEscaped = true
			case '"':
				q.stringBuffer = append(q.stringBuffer, s.source[prevPosition:s.input]...)
				s.advance()
				return QueryErrorNone
			case '\n':
				s.reset(stringStart)
				return QueryErrorSyntax
			}
		}
		if !s.advance() {
			s.reset(stringStart)
			return QueryErrorSyntax
		}
	}
}

// parsePredicate is ts_query__parse_predicate. A predicate is kept as data:
// a name, then capture and string steps, then a Done step.
func (q *Query) parsePredicate(s *stream) QueryErrorKind {
	if !s.isIdentStart() {
		return QueryErrorSyntax
	}
	predicateName := s.input
	s.scanIdentifier()
	if s.next != '?' && s.next != '!' {
		return QueryErrorSyntax
	}
	s.advance()
	id := q.predicateValues.insertName(s.source[predicateName:s.input])
	q.predicateSteps = append(q.predicateSteps, PredicateStep{Type: PredicateStepString, ValueID: uint32(id)})
	s.skipWhitespace()

	for {
		switch {
		case s.next == ')':
			s.advance()
			s.skipWhitespace()
			q.predicateSteps = append(q.predicateSteps, PredicateStep{Type: PredicateStepDone})
			return QueryErrorNone

		case s.next == '@':
			s.advance()
			if !s.isIdentStart() {
				return QueryErrorSyntax
			}
			captureName := s.input
			s.scanIdentifier()
			captureID := q.captures.idForName(s.source[captureName:s.input])
			if captureID == -1 {
				s.reset(captureName)
				return QueryErrorCapture
			}
			q.predicateSteps = append(q.predicateSteps, PredicateStep{Type: PredicateStepCapture, ValueID: uint32(captureID)})

		case s.next == '"':
			if e := q.parseStringLiteral(s); e != QueryErrorNone {
				return e
			}
			id := q.predicateValues.insertName(string(q.stringBuffer))
			q.predicateSteps = append(q.predicateSteps, PredicateStep{Type: PredicateStepString, ValueID: uint32(id)})

		case s.isIdentStart():
			symbolStart := s.input
			s.scanIdentifier()
			id := q.predicateValues.insertName(s.source[symbolStart:s.input])
			q.predicateSteps = append(q.predicateSteps, PredicateStep{Type: PredicateStepString, ValueID: uint32(id)})

		default:
			return QueryErrorSyntax
		}
		s.skipWhitespace()
	}
}

// markLastChild marks a step and its alternatives as the last child.
func (q *Query) markLastChild(lastChildStepIndex uint16) {
	lastChildStep := &q.steps[lastChildStepIndex]
	lastChildStep.isLastChild = true
	if lastChildStep.alternativeIndex != noneIndex && int(lastChildStep.alternativeIndex) < len(q.steps) {
		alternativeStep := &q.steps[lastChildStep.alternativeIndex]
		alternativeStep.isLastChild = true
		for alternativeStep.alternativeIndex != noneIndex && int(alternativeStep.alternativeIndex) < len(q.steps) {
			alternativeStep = &q.steps[alternativeStep.alternativeIndex]
			alternativeStep.isLastChild = true
		}
	}
}

// parsePattern is ts_query__parse_pattern. It reads one S-expression and
// appends its steps. The caller passes a quantifier list of its own.
func (q *Query) parsePattern(
	s *stream,
	depth uint32,
	isImmediate bool,
	isInsideAlternation bool,
	quantifiers *captureQuantifiers,
) QueryErrorKind {
	if s.next == 0 {
		return QueryErrorSyntax
	}
	if s.next == ')' || s.next == ']' {
		return parentDone
	}

	startingStepIndex := uint32(len(q.steps))

	if len(q.stepOffsets) == 0 || uint32(q.stepOffsets[len(q.stepOffsets)-1].stepIndex) != startingStepIndex {
		q.stepOffsets = append(q.stepOffsets, stepOffset{
			stepIndex:  uint16(startingStepIndex),
			byteOffset: s.offset(),
		})
	}

	switch {
	// An alternation.
	case s.next == '[':
		s.advance()
		s.skipWhitespace()

		var branchStepIndices []uint32
		var branchQuantifiers captureQuantifiers
		for {
			startIndex := uint32(len(q.steps))
			e := q.parsePattern(s, depth, isImmediate, true, &branchQuantifiers)
			if e == parentDone {
				if s.next == ']' && len(branchStepIndices) > 0 {
					s.advance()
					break
				}
				e = QueryErrorSyntax
			}
			if e != QueryErrorNone {
				return e
			}

			if startIndex == startingStepIndex {
				quantifiers.replace(branchQuantifiers)
			} else {
				quantifiers.joinAll(branchQuantifiers)
			}

			branchStepIndices = append(branchStepIndices, startIndex)
			q.steps = append(q.steps, newQueryStep(0, uint16(depth), false))
			branchQuantifiers = branchQuantifiers[:0]
		}
		q.steps = q.steps[:len(q.steps)-1]

		for i := 0; i < len(branchStepIndices)-1; i++ {
			stepIndex := branchStepIndices[i]
			nextStepIndex := branchStepIndices[i+1]
			q.steps[stepIndex].alternativeIndex = uint16(nextStepIndex)
			endStep := &q.steps[nextStepIndex-1]
			endStep.alternativeIndex = uint16(len(q.steps))
			endStep.isDeadEnd = true
		}

	// A group, a predicate, or a named node.
	case s.next == '(':
		s.advance()
		s.skipWhitespace()

		switch {
		// A grouped sequence.
		case s.next == '(' || s.next == '"' || s.next == '[':
			childIsImmediate := isImmediate
			var childQuantifiers captureQuantifiers
			for {
				if s.next == '.' {
					anchorStart := s.input
					childIsImmediate = true
					s.advance()
					s.skipWhitespace()
					// A `.` at a group's end has no sibling to anchor.
					if s.next == ')' {
						s.reset(anchorStart)
						return QueryErrorSyntax
					}
				}
				e := q.parsePattern(s, depth, childIsImmediate, isInsideAlternation, &childQuantifiers)
				if e == parentDone {
					if s.next == ')' {
						s.advance()
						break
					}
					e = QueryErrorSyntax
				}
				if e != QueryErrorNone {
					return e
				}
				quantifiers.addAll(childQuantifiers)
				childQuantifiers = childQuantifiers[:0]
				childIsImmediate = false
			}

		// A predicate.
		case s.next == '.' || s.next == '#':
			s.advance()
			return q.parsePredicate(s)

		// A named node.
		default:
			var symbol Symbol
			isMissing := false
			nodeName := s.input

			if !s.isIdentStart() {
				return QueryErrorSyntax
			}
			s.scanIdentifier()
			name := s.source[nodeName:s.input]

			switch {
			case name == "_":
				symbol = wildcardSymbol
			case name == "MISSING":
				isMissing = true
				s.skipWhitespace()
				switch {
				case s.isIdentStart():
					missingNodeName := s.input
					s.scanIdentifier()
					symbol = q.language.SymbolForName(s.source[missingNodeName:s.input], true)
					if symbol == 0 {
						s.reset(missingNodeName)
						return QueryErrorNodeType
					}
				case s.next == '"':
					stringStart := s.input
					if e := q.parseStringLiteral(s); e != QueryErrorNone {
						return e
					}
					symbol = q.language.SymbolForName(string(q.stringBuffer), false)
					if symbol == 0 {
						s.reset(stringStart + 1)
						return QueryErrorNodeType
					}
				case s.next == ')':
					symbol = wildcardSymbol
				default:
					s.reset(s.input)
					return QueryErrorSyntax
				}
			default:
				symbol = q.language.SymbolForName(name, true)
				if symbol == 0 {
					s.reset(nodeName)
					return QueryErrorNodeType
				}
			}

			q.steps = append(q.steps, newQueryStep(symbol, uint16(depth), isImmediate))
			step := &q.steps[len(q.steps)-1]
			if q.language.symbolMetadata(symbol).Supertype {
				step.supertypeSymbol = step.symbol
				step.symbol = wildcardSymbol
			}
			if isMissing {
				step.isMissing = true
			}
			if symbol == wildcardSymbol {
				step.isNamed = true
			}

			// A supertype's subtype: `(expression/identifier)`.
			if s.next == '/' {
				if step.supertypeSymbol == 0 {
					s.reset(nodeName - 1)
					return QueryErrorStructure
				}
				s.advance()
				subtypeNodeName := s.input
				switch {
				case s.isIdentStart():
					s.scanIdentifier()
					step.symbol = q.language.SymbolForName(s.source[subtypeNodeName:s.input], true)
				case s.next == '"':
					if e := q.parseStringLiteral(s); e != QueryErrorNone {
						return e
					}
					step.symbol = q.language.SymbolForName(string(q.stringBuffer), false)
				default:
					return QueryErrorSyntax
				}
				if step.symbol == 0 {
					s.reset(subtypeNodeName)
					return QueryErrorNodeType
				}

				if q.language.ABIVersion >= languageVersionWithQueries {
					subtypeIsValid := false
					for _, sub := range q.language.Subtypes(step.supertypeSymbol) {
						if sub == step.symbol {
							subtypeIsValid = true
							break
						}
					}
					if !subtypeIsValid {
						s.reset(nodeName - 1)
						return QueryErrorStructure
					}
				}
			}

			s.skipWhitespace()

			// The child patterns.
			childIsImmediate := false
			lastChildStepIndex := uint16(0)
			var negatedFieldIDs []FieldID
			var childQuantifiers captureQuantifiers
			for {
				// A negated field: `!field`.
				if s.next == '!' {
					s.advance()
					s.skipWhitespace()
					if !s.isIdentStart() {
						return QueryErrorSyntax
					}
					fieldName := s.input
					s.scanIdentifier()
					fieldID := q.language.FieldIDForName(s.source[fieldName:s.input])
					s.skipWhitespace()
					if fieldID == 0 {
						s.input = fieldName
						return QueryErrorField
					}
					if len(negatedFieldIDs) < maxNegatedFieldCount {
						negatedFieldIDs = append(negatedFieldIDs, fieldID)
					}
					continue
				}

				// A sibling anchor.
				if s.next == '.' {
					childIsImmediate = true
					s.advance()
					s.skipWhitespace()
				}

				stepIndex := len(q.steps)
				e := q.parsePattern(s, depth+1, childIsImmediate, isInsideAlternation, &childQuantifiers)
				// A predicate adds no step, so point at the step before it.
				if stepIndex == len(q.steps) {
					stepIndex--
				}
				if e == parentDone {
					if s.next == ')' {
						if childIsImmediate {
							if lastChildStepIndex == 0 {
								return QueryErrorSyntax
							}
							q.markLastChild(lastChildStepIndex)
						}
						if len(negatedFieldIDs) > 0 {
							q.addNegatedFields(startingStepIndex, negatedFieldIDs)
						}
						s.advance()
						break
					}
					e = QueryErrorSyntax
				}
				if e != QueryErrorNone {
					return e
				}

				quantifiers.addAll(childQuantifiers)
				lastChildStepIndex = uint16(stepIndex)
				childIsImmediate = false
				childQuantifiers = childQuantifiers[:0]
			}
		}

	// A wildcard.
	case s.next == '_':
		s.advance()
		s.skipWhitespace()
		q.steps = append(q.steps, newQueryStep(wildcardSymbol, uint16(depth), isImmediate))

	// An anonymous leaf: `"token"`.
	case s.next == '"':
		stringStart := s.input
		if e := q.parseStringLiteral(s); e != QueryErrorNone {
			return e
		}
		symbol := q.language.SymbolForName(string(q.stringBuffer), false)
		if symbol == 0 {
			s.reset(stringStart + 1)
			return QueryErrorNodeType
		}
		q.steps = append(q.steps, newQueryStep(symbol, uint16(depth), isImmediate))

	// A field: `name: pattern`.
	case s.isIdentStart():
		fieldName := s.input
		s.scanIdentifier()
		fieldNameEnd := s.input
		s.skipWhitespace()

		if s.next != ':' {
			s.reset(fieldName)
			return QueryErrorSyntax
		}
		s.advance()
		s.skipWhitespace()

		var fieldQuantifiers captureQuantifiers
		e := q.parsePattern(s, depth, isImmediate, isInsideAlternation, &fieldQuantifiers)
		if e != QueryErrorNone {
			if e == parentDone {
				e = QueryErrorSyntax
			}
			return e
		}

		fieldID := q.language.FieldIDForName(s.source[fieldName:fieldNameEnd])
		if fieldID == 0 {
			s.input = fieldName
			return QueryErrorField
		}

		stepIndex := startingStepIndex
		for {
			step := &q.steps[stepIndex]
			step.field = fieldID
			if step.alternativeIndex != noneIndex &&
				uint32(step.alternativeIndex) > stepIndex &&
				int(step.alternativeIndex) < len(q.steps) {
				stepIndex = uint32(step.alternativeIndex)
			} else {
				break
			}
		}
		quantifiers.addAll(fieldQuantifiers)

	default:
		return QueryErrorSyntax
	}

	s.skipWhitespace()

	// The suffixes: quantifiers and captures.
	quantifier := QuantifierOne
	for {
		switch s.next {
		case '+':
			quantifier = quantifierJoin(QuantifierOneOrMore, quantifier)
			s.advance()
			s.skipWhitespace()
		case '*':
			quantifier = quantifierJoin(QuantifierZeroOrMore, quantifier)
			s.advance()
			s.skipWhitespace()
		case '?':
			quantifier = quantifierJoin(QuantifierZeroOrOne, quantifier)
			s.advance()
			s.skipWhitespace()
		case '@':
			s.advance()
			if !s.isIdentStart() {
				return QueryErrorSyntax
			}
			captureName := s.input
			s.scanIdentifier()
			captureID := q.captures.insertName(s.source[captureName:s.input])
			s.skipWhitespace()

			quantifiers.addForID(captureID, QuantifierOne)

			stepIndex := startingStepIndex
			for {
				step := &q.steps[stepIndex]
				step.addCapture(captureID)
				if step.alternativeIndex != noneIndex &&
					uint32(step.alternativeIndex) > stepIndex &&
					int(step.alternativeIndex) < len(q.steps) {
					stepIndex = uint32(step.alternativeIndex)
				} else {
					break
				}
			}
		default:
			goto suffixesDone
		}
	}
suffixesDone:

	switch quantifier {
	case QuantifierOneOrMore:
		repeatStep := newQueryStep(wildcardSymbol, uint16(depth), false)
		repeatStep.isInsideAlternation = isInsideAlternation
		repeatStep.alternativeIndex = uint16(startingStepIndex)
		repeatStep.isPassThrough = true
		q.steps = append(q.steps, repeatStep)
	case QuantifierZeroOrMore:
		repeatStep := newQueryStep(wildcardSymbol, uint16(depth), false)
		repeatStep.isInsideAlternation = isInsideAlternation
		repeatStep.alternativeIndex = uint16(startingStepIndex)
		repeatStep.isPassThrough = true
		q.steps = append(q.steps, repeatStep)

		// Follow the alternatives up to the repeat step itself.
		step := &q.steps[startingStepIndex]
		for step.alternativeIndex != noneIndex && int(step.alternativeIndex) < len(q.steps)-1 {
			step = &q.steps[step.alternativeIndex]
		}
		step.alternativeIndex = uint16(len(q.steps))
		step.alternativeIsSkip = true
	case QuantifierZeroOrOne:
		step := &q.steps[startingStepIndex]
		for step.alternativeIndex != noneIndex && int(step.alternativeIndex) < len(q.steps) {
			step = &q.steps[step.alternativeIndex]
		}
		step.alternativeIndex = uint16(len(q.steps))
		step.alternativeIsSkip = true
	}

	quantifiers.mul(quantifier)
	return QueryErrorNone
}
