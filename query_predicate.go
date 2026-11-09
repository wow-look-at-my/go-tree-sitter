// query_predicate.go holds the predicate layer the Rust binding
// (lib/binding_rust/lib.rs, v0.27.2) adds on top of query.c. Text
// predicates, properties, general predicates, and the match iterators.

package treesitter

import (
	"bytes"
	"fmt"
	"iter"
	"regexp"
	"strings"
)

// textPredicateKind names the TextPredicateCapture variants.
type textPredicateKind uint8

const (
	textPredicateEqString textPredicateKind = iota
	textPredicateEqCapture
	textPredicateMatchString
	textPredicateAnyString
)

// textPredicate is a TextPredicateCapture: a predicate the cursor checks
// against node text after a match.
type textPredicate struct {
	kind          textPredicateKind
	captureIndex  uint32
	otherCapture  uint32
	value         string
	values        []string
	regex         *regexp.Regexp
	isPositive    bool
	matchAllNodes bool
}

// QueryProperty is a QueryProperty. A property with no value has HasValue
// false.
type QueryProperty struct {
	Key          string
	Value        string
	HasValue     bool
	CaptureIndex int
}

// PropertyPredicate is a property paired with whether it was `#is?` (true)
// or `#is-not?` (false).
type PropertyPredicate struct {
	Property QueryProperty
	Positive bool
}

// QueryPredicateArgKind says what a general predicate's argument is.
type QueryPredicateArgKind uint8

// The argument kinds, as QueryPredicateArg.
const (
	QueryPredicateArgCapture QueryPredicateArgKind = iota
	QueryPredicateArgString
)

// QueryPredicateArg is a QueryPredicateArg.
type QueryPredicateArg struct {
	Kind         QueryPredicateArgKind
	CaptureIndex uint32
	Value        string
}

// QueryPredicate is a QueryPredicate: a predicate the library does not
// interpret, handed to the caller as data.
type QueryPredicate struct {
	Operator string
	Args     []QueryPredicateArg
}

// predicateError is predicate_error.
func predicateError(row int, message string) *QueryError {
	return &QueryError{Kind: QueryErrorPredicate, Row: row, Message: message}
}

// parsePredicates is the predicate part of the Rust binding's
// Query::from_raw_parts. It sorts each pattern's predicates into text
// predicates, properties and general predicates.
func (q *Query) parsePredicates(source string) error {
	patternCount := len(q.patterns)
	q.textPredicates = make([][]textPredicate, patternCount)
	q.propertySettings = make([][]QueryProperty, patternCount)
	q.propertyPredicates = make([][]PropertyPredicate, patternCount)
	q.generalPredicates = make([][]QueryPredicate, patternCount)
	captureNames := q.captures.names
	stringValues := q.predicateValues.names

	for i := range q.patterns {
		steps := q.PredicatesForPattern(i)
		byteOffset := int(q.patterns[i].startByte)
		if byteOffset > len(source) {
			byteOffset = len(source)
		}
		row := strings.Count(source[:byteOffset], "\n")

		for len(steps) > 0 {
			end := 0
			for end < len(steps) && steps[end].Type != PredicateStepDone {
				end++
			}
			p := steps[:end]
			if end < len(steps) {
				steps = steps[end+1:]
			} else {
				steps = nil
			}
			if len(p) == 0 {
				continue
			}

			if p[0].Type != PredicateStepString {
				return predicateError(row, fmt.Sprintf(
					"Expected predicate to start with a function name. Got @%s.",
					captureNames[p[0].ValueID]))
			}

			operatorName := stringValues[p[0].ValueID]
			switch operatorName {
			case "eq?", "not-eq?", "any-eq?", "any-not-eq?":
				if len(p) != 3 {
					return predicateError(row, fmt.Sprintf(
						"Wrong number of arguments to #eq? predicate. Expected 2, got %d.", len(p)-1))
				}
				if p[1].Type != PredicateStepCapture {
					return predicateError(row, fmt.Sprintf(
						"First argument to #eq? predicate must be a capture name. Got literal \"%s\".",
						stringValues[p[1].ValueID]))
				}
				isPositive := operatorName == "eq?" || operatorName == "any-eq?"
				matchAll := operatorName == "eq?" || operatorName == "not-eq?"
				if p[2].Type == PredicateStepCapture {
					q.textPredicates[i] = append(q.textPredicates[i], textPredicate{
						kind:          textPredicateEqCapture,
						captureIndex:  p[1].ValueID,
						otherCapture:  p[2].ValueID,
						isPositive:    isPositive,
						matchAllNodes: matchAll,
					})
				} else {
					q.textPredicates[i] = append(q.textPredicates[i], textPredicate{
						kind:          textPredicateEqString,
						captureIndex:  p[1].ValueID,
						value:         stringValues[p[2].ValueID],
						isPositive:    isPositive,
						matchAllNodes: matchAll,
					})
				}

			case "match?", "not-match?", "any-match?", "any-not-match?":
				if len(p) != 3 {
					return predicateError(row, fmt.Sprintf(
						"Wrong number of arguments to #match? predicate. Expected 2, got %d.", len(p)-1))
				}
				if p[1].Type != PredicateStepCapture {
					return predicateError(row, fmt.Sprintf(
						"First argument to #match? predicate must be a capture name. Got literal \"%s\".",
						stringValues[p[1].ValueID]))
				}
				if p[2].Type == PredicateStepCapture {
					return predicateError(row, fmt.Sprintf(
						"Second argument to #match? predicate must be a literal. Got capture @%s.",
						captureNames[p[2].ValueID]))
				}
				isPositive := operatorName == "match?" || operatorName == "any-match?"
				matchAll := operatorName == "match?" || operatorName == "not-match?"
				pattern := stringValues[p[2].ValueID]
				regex, err := regexp.Compile(pattern)
				if err != nil {
					return predicateError(row, fmt.Sprintf("Invalid regex '%s'", pattern))
				}
				q.textPredicates[i] = append(q.textPredicates[i], textPredicate{
					kind:          textPredicateMatchString,
					captureIndex:  p[1].ValueID,
					regex:         regex,
					isPositive:    isPositive,
					matchAllNodes: matchAll,
				})

			case "set!":
				property, err := parseProperty(row, operatorName, captureNames, stringValues, p[1:])
				if err != nil {
					return err
				}
				q.propertySettings[i] = append(q.propertySettings[i], property)

			case "is?", "is-not?":
				property, err := parseProperty(row, operatorName, captureNames, stringValues, p[1:])
				if err != nil {
					return err
				}
				q.propertyPredicates[i] = append(q.propertyPredicates[i], PropertyPredicate{
					Property: property,
					Positive: operatorName == "is?",
				})

			case "any-of?", "not-any-of?":
				if len(p) < 2 {
					return predicateError(row, fmt.Sprintf(
						"Wrong number of arguments to #any-of? predicate. Expected at least 1, got %d.", len(p)-1))
				}
				if p[1].Type != PredicateStepCapture {
					return predicateError(row, fmt.Sprintf(
						"First argument to #any-of? predicate must be a capture name. Got literal \"%s\".",
						stringValues[p[1].ValueID]))
				}
				values := make([]string, 0, len(p)-2)
				for _, arg := range p[2:] {
					if arg.Type == PredicateStepCapture {
						return predicateError(row, fmt.Sprintf(
							"Arguments to #any-of? predicate must be literals. Got capture @%s.",
							captureNames[arg.ValueID]))
					}
					values = append(values, stringValues[arg.ValueID])
				}
				q.textPredicates[i] = append(q.textPredicates[i], textPredicate{
					kind:         textPredicateAnyString,
					captureIndex: p[1].ValueID,
					values:       values,
					isPositive:   operatorName == "any-of?",
				})

			default:
				args := make([]QueryPredicateArg, 0, len(p)-1)
				for _, a := range p[1:] {
					if a.Type == PredicateStepCapture {
						args = append(args, QueryPredicateArg{Kind: QueryPredicateArgCapture, CaptureIndex: a.ValueID})
					} else {
						args = append(args, QueryPredicateArg{Kind: QueryPredicateArgString, Value: stringValues[a.ValueID]})
					}
				}
				q.generalPredicates[i] = append(q.generalPredicates[i], QueryPredicate{
					Operator: operatorName,
					Args:     args,
				})
			}
		}
	}
	return nil
}

// parseProperty is Query::parse_property.
func parseProperty(row int, functionName string, captureNames, stringValues []string, args []PredicateStep) (QueryProperty, error) {
	if len(args) == 0 || len(args) > 3 {
		return QueryProperty{}, predicateError(row, fmt.Sprintf(
			"Wrong number of arguments to %s predicate. Expected 1 to 3, got %d.", functionName, len(args)))
	}
	property := QueryProperty{CaptureIndex: -1}
	hasKey := false
	for _, arg := range args {
		switch {
		case arg.Type == PredicateStepCapture:
			if property.CaptureIndex >= 0 {
				return QueryProperty{}, predicateError(row, fmt.Sprintf(
					"Invalid arguments to %s predicate. Unexpected second capture name @%s",
					functionName, captureNames[arg.ValueID]))
			}
			property.CaptureIndex = int(arg.ValueID)
		case !hasKey:
			property.Key = stringValues[arg.ValueID]
			hasKey = true
		case !property.HasValue:
			property.Value = stringValues[arg.ValueID]
			property.HasValue = true
		default:
			return QueryProperty{}, predicateError(row, fmt.Sprintf(
				"Invalid arguments to %s predicate. Unexpected third argument @%s",
				functionName, stringValues[arg.ValueID]))
		}
	}
	if !hasKey {
		return QueryProperty{}, predicateError(row, fmt.Sprintf(
			"Invalid arguments to %s predicate. Missing key argument", functionName))
	}
	return property, nil
}

// PropertySettings reports a pattern's `#set!` properties, as the Rust
// binding's property_settings.
func (q *Query) PropertySettings(patternIndex int) []QueryProperty {
	return q.propertySettings[patternIndex]
}

// PropertyPredicates reports a pattern's `#is?` and `#is-not?` properties, as
// the Rust binding's property_predicates.
func (q *Query) PropertyPredicates(patternIndex int) []PropertyPredicate {
	return q.propertyPredicates[patternIndex]
}

// GeneralPredicates reports the predicates of a pattern that the library
// leaves to the caller, as the Rust binding's general_predicates.
func (q *Query) GeneralPredicates(patternIndex int) []QueryPredicate {
	return q.generalPredicates[patternIndex]
}

// TextProvider returns the text of a node, as the Rust binding's TextProvider.
type TextProvider func(node Node) []byte

// SourceText is the TextProvider over a whole document.
func SourceText(source []byte) TextProvider {
	return func(node Node) []byte {
		start, end := node.StartByte(), node.EndByte()
		if start > uint32(len(source)) {
			start = uint32(len(source))
		}
		if end > uint32(len(source)) {
			end = uint32(len(source))
		}
		return source[start:end]
	}
}

// NodesForCaptureIndex reports the nodes a match captured under one capture
// id, as QueryMatch::nodes_for_capture_index.
func (m *QueryMatch) NodesForCaptureIndex(captureIndex uint32) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for _, c := range m.Captures {
			if c.Index == captureIndex && !yield(c.Node) {
				return
			}
		}
	}
}

// SatisfiesTextPredicates is QueryMatch::satisfies_text_predicates: whether
// the text predicates of the match's pattern hold.
func (m *QueryMatch) SatisfiesTextPredicates(query *Query, text TextProvider) bool {
	for i := range query.textPredicates[m.PatternIndex] {
		p := &query.textPredicates[m.PatternIndex][i]
		switch p.kind {
		case textPredicateEqCapture:
			i1, i2 := 0, 0
			next := func(captureIndex uint32, from int) (Node, int, bool) {
				for j := from; j < len(m.Captures); j++ {
					if m.Captures[j].Index == captureIndex {
						return m.Captures[j].Node, j + 1, true
					}
				}
				return Node{}, len(m.Captures), false
			}
			for {
				node1, n1, ok1 := next(p.captureIndex, i1)
				node2, n2, ok2 := next(p.otherCapture, i2)
				if !ok1 || !ok2 {
					if ok1 || ok2 {
						return false
					}
					break
				}
				i1, i2 = n1, n2
				isPositiveMatch := bytes.Equal(text(node1), text(node2))
				if isPositiveMatch != p.isPositive && p.matchAllNodes {
					return false
				}
				if isPositiveMatch == p.isPositive && !p.matchAllNodes {
					goto nextPredicate
				}
			}
		case textPredicateEqString:
			for _, c := range m.Captures {
				if c.Index != p.captureIndex {
					continue
				}
				isPositiveMatch := string(text(c.Node)) == p.value
				if isPositiveMatch != p.isPositive && p.matchAllNodes {
					return false
				}
				if isPositiveMatch == p.isPositive && !p.matchAllNodes {
					goto nextPredicate
				}
			}
		case textPredicateMatchString:
			for _, c := range m.Captures {
				if c.Index != p.captureIndex {
					continue
				}
				isPositiveMatch := p.regex.Match(text(c.Node))
				if isPositiveMatch != p.isPositive && p.matchAllNodes {
					return false
				}
				if isPositiveMatch == p.isPositive && !p.matchAllNodes {
					goto nextPredicate
				}
			}
		case textPredicateAnyString:
			for _, c := range m.Captures {
				if c.Index != p.captureIndex {
					continue
				}
				t := text(c.Node)
				found := false
				for _, v := range p.values {
					if string(t) == v {
						found = true
						break
					}
				}
				if found != p.isPositive {
					return false
				}
			}
		}
	nextPredicate:
	}
	return true
}

// Matches runs the query over the node and yields each match whose text
// predicates hold, as QueryCursor::matches. The yielded match is valid until
// the next one.
func (c *QueryCursor) Matches(query *Query, node Node, source []byte) iter.Seq[*QueryMatch] {
	return c.MatchesWithOptions(query, node, SourceText(source), nil)
}

// MatchesWithOptions is QueryCursor::matches_with_options.
func (c *QueryCursor) MatchesWithOptions(query *Query, node Node, text TextProvider, options *QueryCursorOptions) iter.Seq[*QueryMatch] {
	return func(yield func(*QueryMatch) bool) {
		c.ExecWithOptions(query, node, options)
		var match QueryMatch
		for c.NextMatch(&match) {
			if !match.SatisfiesTextPredicates(query, text) {
				continue
			}
			if !yield(&match) {
				return
			}
		}
	}
}

// Captures runs the query over the node and yields each capture in document order. Those Captures are with its match and its index in the match's
// captures, as QueryCursor::captures.
func (c *QueryCursor) Captures(query *Query, node Node, source []byte) iter.Seq2[*QueryMatch, uint32] {
	return c.CapturesWithOptions(query, node, SourceText(source), nil)
}

// CapturesWithOptions is QueryCursor::captures_with_options.
func (c *QueryCursor) CapturesWithOptions(query *Query, node Node, text TextProvider, options *QueryCursorOptions) iter.Seq2[*QueryMatch, uint32] {
	return func(yield func(*QueryMatch, uint32) bool) {
		c.ExecWithOptions(query, node, options)
		var match QueryMatch
		for {
			captureIndex, ok := c.NextCapture(&match)
			if !ok {
				return
			}
			if !match.SatisfiesTextPredicates(query, text) {
				c.RemoveMatch(match.ID)
				continue
			}
			if !yield(&match, captureIndex) {
				return
			}
		}
	}
}
