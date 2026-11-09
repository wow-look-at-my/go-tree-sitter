// query_api.go holds the public accessors of a compiled query (ts_query_* in query.c, v0.27.2).

package treesitter

// Copy is ts_query_copy: an independent query to disable captures or
// patterns on.
func (q *Query) Copy() *Query {
	c := *q
	c.steps = append([]queryStep(nil), q.steps...)
	c.patternMap = append([]patternEntry(nil), q.patternMap...)
	c.predicateSteps = append([]PredicateStep(nil), q.predicateSteps...)
	c.patterns = append([]queryPattern(nil), q.patterns...)
	c.stepOffsets = append([]stepOffset(nil), q.stepOffsets...)
	c.negatedFields = append([]FieldID(nil), q.negatedFields...)
	c.repeatSymbolsWithRootlessPatterns = append([]Symbol(nil), q.repeatSymbolsWithRootlessPatterns...)
	c.captures.names = append([]string(nil), q.captures.names...)
	c.predicateValues.names = append([]string(nil), q.predicateValues.names...)
	c.captureQuantifiers = make([][]uint8, len(q.captureQuantifiers))
	for i, cq := range q.captureQuantifiers {
		c.captureQuantifiers[i] = append([]uint8(nil), cq...)
	}
	return &c
}

// Language reports the grammar the query was compiled for.
func (q *Query) Language() *Language { return q.language }

// PatternCount is ts_query_pattern_count.
func (q *Query) PatternCount() int { return len(q.patterns) }

// CaptureCount is ts_query_capture_count.
func (q *Query) CaptureCount() int { return len(q.captures.names) }

// StringCount is ts_query_string_count.
func (q *Query) StringCount() int { return len(q.predicateValues.names) }

// CaptureNameForID is ts_query_capture_name_for_id.
func (q *Query) CaptureNameForID(id uint32) string { return q.captures.names[id] }

// CaptureNames reports every capture name, indexed by capture id.
func (q *Query) CaptureNames() []string { return q.captures.names }

// CaptureIndexForName reports the capture id of a name, as the Rust binding's
// capture_index_for_name.
func (q *Query) CaptureIndexForName(name string) (uint32, bool) {
	id := q.captures.idForName(name)
	if id < 0 {
		return 0, false
	}
	return uint32(id), true
}

// CaptureQuantifierForID is ts_query_capture_quantifier_for_id.
func (q *Query) CaptureQuantifierForID(patternIndex, captureIndex uint32) Quantifier {
	return captureQuantifiers(q.captureQuantifiers[patternIndex]).forID(uint16(captureIndex))
}

// CaptureQuantifiers reports a pattern's quantifier for every capture id, as
// the Rust binding's capture_quantifiers.
func (q *Query) CaptureQuantifiers(patternIndex int) []Quantifier {
	result := make([]Quantifier, len(q.captures.names))
	for i := range result {
		result[i] = q.CaptureQuantifierForID(uint32(patternIndex), uint32(i))
	}
	return result
}

// StringValueForID is ts_query_string_value_for_id.
func (q *Query) StringValueForID(id uint32) string { return q.predicateValues.names[id] }

// PredicatesForPattern is ts_query_predicates_for_pattern. Each predicate ends
// with a Done step.
func (q *Query) PredicatesForPattern(patternIndex int) []PredicateStep {
	p := q.patterns[patternIndex]
	if p.predicateStepsLength == 0 {
		return nil
	}
	return q.predicateSteps[p.predicateStepsOffset : p.predicateStepsOffset+p.predicateStepsLength]
}

// StartByteForPattern is ts_query_start_byte_for_pattern.
func (q *Query) StartByteForPattern(patternIndex int) uint32 {
	return q.patterns[patternIndex].startByte
}

// EndByteForPattern is ts_query_end_byte_for_pattern.
func (q *Query) EndByteForPattern(patternIndex int) uint32 {
	return q.patterns[patternIndex].endByte
}

// IsPatternRooted is ts_query_is_pattern_rooted.
func (q *Query) IsPatternRooted(patternIndex int) bool {
	for _, entry := range q.patternMap {
		if int(entry.patternIndex) == patternIndex && !entry.isRooted {
			return false
		}
	}
	return true
}

// IsPatternNonLocal is ts_query_is_pattern_non_local.
func (q *Query) IsPatternNonLocal(patternIndex int) bool {
	if patternIndex < len(q.patterns) {
		return q.patterns[patternIndex].isNonLocal
	}
	return false
}

// IsPatternGuaranteedAtStep is ts_query_is_pattern_guaranteed_at_step.
func (q *Query) IsPatternGuaranteedAtStep(byteOffset uint32) bool {
	stepIndex := uint32(0xFFFFFFFF)
	for _, so := range q.stepOffsets {
		if so.byteOffset > byteOffset {
			break
		}
		stepIndex = uint32(so.stepIndex)
	}
	if stepIndex < uint32(len(q.steps)) {
		return q.steps[stepIndex].rootPatternGuaranteed
	}
	return false
}

// stepIsFallible is ts_query__step_is_fallible.
func (q *Query) stepIsFallible(stepIndex uint16) bool {
	i := uint32(1)
	step := &q.steps[stepIndex]
	var nextStep *queryStep
	for {
		nextStep = &q.steps[uint32(stepIndex)+i]
		i++
		if !nextStep.isPassThrough {
			break
		}
	}
	return nextStep.depth != patternDoneMarker &&
		(nextStep.depth > step.depth ||
			(nextStep.depth == step.depth && nextStep.isImmediate)) &&
		(!nextStep.parentPatternGuaranteed || step.symbol == wildcardSymbol)
}

// DisableCapture is ts_query_disable_capture.
func (q *Query) DisableCapture(name string) {
	id := q.captures.idForName(name)
	if id != -1 {
		for i := range q.steps {
			q.steps[i].removeCapture(uint16(id))
		}
	}
}

// DisablePattern is ts_query_disable_pattern.
func (q *Query) DisablePattern(patternIndex int) {
	kept := q.patternMap[:0]
	for _, entry := range q.patternMap {
		if int(entry.patternIndex) != patternIndex {
			kept = append(kept, entry)
		}
	}
	q.patternMap = kept
}
