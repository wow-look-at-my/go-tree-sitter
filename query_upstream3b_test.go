package treesitter_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// The second half of part 3 of the upstream query suite. The init in
// query_upstream3_test.go registers these tests.

// test_capture_quantifiers. The python row "(module) @mod" becomes the
// javascript "(program) @mod": both capture the root.
func TestCaptureQuantifiers(t *testing.T) {
	type expectation struct {
		pattern    int
		capture    string
		quantifier ts.Quantifier
	}
	rows := []struct {
		description string
		pattern     string
		expected    []expectation
	}{
		{"Top level capture", `
                (program) @mod
            `, []expectation{{0, "mod", ts.QuantifierOne}}},
		{"Nested list capture capture", `
                (array (_)* @elems) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "elems", ts.QuantifierZeroOrMore},
		}},
		{"Nested non-empty list capture capture", `
                (array (_)+ @elems) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "elems", ts.QuantifierOneOrMore},
		}},
		{"capture nested in optional pattern", `
                (array (call_expression (arguments (_) @arg))? @call) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "call", ts.QuantifierZeroOrOne},
			{0, "arg", ts.QuantifierZeroOrOne},
		}},
		{"optional capture nested in non-empty list pattern", `
                (array (call_expression (arguments (_)? @arg))+ @call) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "call", ts.QuantifierOneOrMore},
			{0, "arg", ts.QuantifierZeroOrMore},
		}},
		{"non-empty list capture nested in optional pattern", `
                (array (call_expression (arguments (_)+ @args))? @call) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "call", ts.QuantifierZeroOrOne},
			{0, "args", ts.QuantifierZeroOrMore},
		}},
		{"capture is the same in all alternatives", `[
                (function_declaration name:(identifier) @name)
                (call_expression function:(identifier) @name)
            ]`, []expectation{{0, "name", ts.QuantifierOne}}},
		{"capture appears in some alternatives", `[
                (function_declaration name:(identifier) @name)
                (function_expression)
            ] @fun`, []expectation{
			{0, "fun", ts.QuantifierOne},
			{0, "name", ts.QuantifierZeroOrOne},
		}},
		{"capture has different quantifiers in alternatives", `[
                (call_expression arguments: (arguments (_)+ @args))
                (new_expression  arguments: (arguments (_)? @args))
            ] @call`, []expectation{
			{0, "call", ts.QuantifierOne},
			{0, "args", ts.QuantifierZeroOrMore},
		}},
		{"siblings have different captures with different quantifiers", `
                (call_expression (arguments (identifier)? @self (_)* @args)) @call
            `, []expectation{
			{0, "call", ts.QuantifierOne},
			{0, "self", ts.QuantifierZeroOrOne},
			{0, "args", ts.QuantifierZeroOrMore},
		}},
		{"siblings have same capture with different quantifiers", `
                (call_expression (arguments (identifier) @args (_)* @args)) @call
            `, []expectation{
			{0, "call", ts.QuantifierOne},
			{0, "args", ts.QuantifierOneOrMore},
		}},
		{"combined nesting, alternatives, and siblings", `
                (array
                    (call_expression
                        (arguments [
                            (identifier) @self
                            (_)+ @args
                        ])
                    )+ @call
                ) @array
            `, []expectation{
			{0, "array", ts.QuantifierOne},
			{0, "call", ts.QuantifierOneOrMore},
			{0, "self", ts.QuantifierZeroOrMore},
			{0, "args", ts.QuantifierZeroOrMore},
		}},
		{"multiple patterns", `
                (function_declaration name: (identifier) @x)
                (statement_identifier) @y
                (property_identifier)+ @z
                (array (identifier)* @x)
            `, []expectation{
			{0, "x", ts.QuantifierOne},
			{1, "x", ts.QuantifierZero},
			{2, "x", ts.QuantifierZero},
			{3, "x", ts.QuantifierZeroOrMore},
			{0, "y", ts.QuantifierZero},
			{1, "y", ts.QuantifierOne},
			{2, "y", ts.QuantifierZero},
			{3, "y", ts.QuantifierZero},
			{0, "z", ts.QuantifierZero},
			{1, "z", ts.QuantifierZero},
			{2, "z", ts.QuantifierOneOrMore},
			{3, "z", ts.QuantifierZero},
		}},
		{"multiple alternatives", `
            [
                (array (identifier) @x)
                (function_declaration name: (identifier)+ @x)
            ]
            [
                (array (identifier) @x)
                (function_declaration name: (identifier)+ @x)
            ]
            `, []expectation{
			{0, "x", ts.QuantifierOneOrMore},
			{1, "x", ts.QuantifierOneOrMore},
		}},
	}
	language := queryLanguage(t, "javascript")
	for _, row := range rows {
		query := p3MustQuery(t, language, row.pattern)
		for _, e := range row.expected {
			index, ok := query.CaptureIndexForName(e.capture)
			require.True(t, ok, "%s: no capture @%s", row.description, e.capture)
			actual := query.CaptureQuantifiers(e.pattern)[index]
			require.Equal(t, e.quantifier, actual,
				"Description: %s, expected quantifier of @%s in pattern %d to be %v instead of %v",
				row.description, e.capture, e.pattern, e.quantifier, actual)
		}
	}
}

// test_query_quantified_captures
func TestQueryQuantifiedCaptures(t *testing.T) {
	docComments := "/// foo\n/// bar\n/// baz\n\nvoid main() {}\n\n/// qux\n/// quux\n// quuz\n"
	rows := []struct {
		description string
		code        string
		pattern     string
		captures    []capturePair
	}{
		{
			description: "doc comments where all must match the prefix",
			code:        docComments,
			pattern: `
                ((comment)+ @comment.documentation
                  (#match? @comment.documentation "^///"))
            `,
			captures: []capturePair{
				c("comment.documentation", "/// foo"),
				c("comment.documentation", "/// bar"),
				c("comment.documentation", "/// baz"),
			},
		},
		{
			description: "doc comments where one must match the prefix",
			code:        docComments,
			pattern: `
                ((comment)+ @comment.documentation
                  (#any-match? @comment.documentation "^///"))
            `,
			captures: []capturePair{
				c("comment.documentation", "/// foo"),
				c("comment.documentation", "/// bar"),
				c("comment.documentation", "/// baz"),
				c("comment.documentation", "/// qux"),
				c("comment.documentation", "/// quux"),
				c("comment.documentation", "// quuz"),
			},
		},
		{
			description: "multiple quantifiers should not hang query parsing",
			code:        "// foo\n// bar\n// baz\n",
			pattern: `
                ((comment) ?+ @comment)
            `,
			captures: []capturePair{
				c("comment", "// foo"),
				c("comment", "// foo"),
				c("comment", "// foo"),
				c("comment", "// bar"),
				c("comment", "// baz"),
			},
		},
	}
	language := queryLanguage(t, "c")
	for _, row := range rows {
		tree := parseSource(t, language, row.code)
		query := p3MustQuery(t, language, row.pattern)
		cursor := ts.NewQueryCursor()
		require.Equal(t, row.captures, collectCaptures(cursor, query, tree.RootNode(), row.code), row.description)
	}
}

// test_query_max_start_depth
func TestQueryMaxStartDepth(t *testing.T) {
	source := "if (a1 && a2) {\n    if (b1 && b2) { }\n    if (c) { }\n}\nif (d) {\n    if (e1 && e2) { }\n    if (f) { }\n}\n"
	deepPattern := `
                (if_statement
                    condition: (parenthesized_expression
                        (binary_expression)
                    )
                ) @capture
            `
	rows := []struct {
		description string
		depth       uint32
		pattern     string
		matches     []matchResult
	}{
		{"depth 0: match translation unit", 0, `
                (translation_unit) @capture
            `, []matchResult{
			m(0, "capture", "if (a1 && a2) {\n    if (b1 && b2) { }\n    if (c) { }\n}\nif (d) {\n    if (e1 && e2) { }\n    if (f) { }\n}\n"),
		}},
		{"depth 0: match none", 0, `
                (if_statement) @capture
            `, []matchResult{}},
		{"depth 1: match 2 if statements at the top level", 1, `
                (if_statement) @capture
            `, []matchResult{
			m(0, "capture", "if (a1 && a2) {\n    if (b1 && b2) { }\n    if (c) { }\n}"),
			m(0, "capture", "if (d) {\n    if (e1 && e2) { }\n    if (f) { }\n}"),
		}},
		{"depth 1 with deep pattern: match the only the first if statement", 1, deepPattern, []matchResult{
			m(0, "capture", "if (a1 && a2) {\n    if (b1 && b2) { }\n    if (c) { }\n}"),
		}},
		{"depth 3 with deep pattern: match all if statements with a binexpr condition", 3, deepPattern, []matchResult{
			m(0, "capture", "if (a1 && a2) {\n    if (b1 && b2) { }\n    if (c) { }\n}"),
			m(0, "capture", "if (b1 && b2) { }"),
			m(0, "capture", "if (e1 && e2) { }"),
		}},
	}
	language := queryLanguage(t, "c")
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	for _, row := range rows {
		query := p3MustQuery(t, language, row.pattern)
		cursor.SetMaxStartDepth(row.depth)
		require.Equal(t, row.matches, collectMatches(cursor, query, tree.RootNode(), source), row.description)
	}
}

// test_query_error_does_not_oob
func TestQueryErrorDoesNotOob(t *testing.T) {
	qerr := queryErrorOf(t, queryLanguage(t, "javascript"), "(clas")
	require.Equal(t, ts.QueryError{
		Row:     0,
		Offset:  1,
		Column:  1,
		Kind:    ts.QueryErrorNodeType,
		Message: `"clas"`,
	}, *qerr)
}

// test_consecutive_zero_or_modifiers
func TestConsecutiveZeroOrModifiers(t *testing.T) {
	language := queryLanguage(t, "javascript")
	zeroSource := ""
	threeSource := "/**/ /**/ /**/"
	zeroTree := parseSource(t, language, zeroSource)
	threeTree := parseSource(t, language, threeSource)

	for _, test := range []string{
		"(comment)*** @capture",
		"(comment)??? @capture",
		"(comment)*?* @capture",
		"(comment)?*? @capture",
	} {
		query := p3MustQuery(t, language, test)

		cursor := ts.NewQueryCursor()
		found := false
		for range cursor.Matches(query, zeroTree.RootNode(), []byte(zeroSource)) {
			found = true
			break
		}
		require.True(t, found, test)

		cursor = ts.NewQueryCursor()
		len3, len1 := false, false
		for match := range cursor.Matches(query, threeTree.RootNode(), []byte(threeSource)) {
			if len(match.Captures) == 3 {
				len3 = true
			}
			if len(match.Captures) == 1 {
				len1 = true
			}
		}
		require.Equal(t, strings.Contains(test, "*"), len3, test)
		require.Equal(t, strings.Contains(test, "???"), len1, test)
	}
}

// test_query_max_start_depth_more
func TestQueryMaxStartDepthMore(t *testing.T) {
	source := "{\n    { }\n    {\n        { }\n    }\n}\n"
	outer := "{\n    { }\n    {\n        { }\n    }\n}"
	rows := []struct {
		depth   uint32
		matches []matchResult
	}{
		{0, []matchResult{m(0, "capture", outer)}},
		{1, []matchResult{
			m(0, "capture", outer),
			m(0, "capture", "{ }"),
			m(0, "capture", "{\n        { }\n    }"),
		}},
		{2, []matchResult{
			m(0, "capture", outer),
			m(0, "capture", "{ }"),
			m(0, "capture", "{\n        { }\n    }"),
			m(0, "capture", "{ }"),
		}},
	}
	language := queryLanguage(t, "c")
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	query := p3MustQuery(t, language, "(compound_statement) @capture")

	var node ts.Node
	for match := range cursor.Matches(query, tree.RootNode(), []byte(source)) {
		node = match.Captures[0].Node
		break
	}
	require.Equal(t, "compound_statement", node.Type())

	for _, row := range rows {
		cursor.SetMaxStartDepth(row.depth)
		require.Equal(t, row.matches, collectMatches(cursor, query, node, source), "depth %d", row.depth)
	}
}

// test_query_with_first_child_in_group_is_anchor
func TestQueryWithFirstChildInGroupIsAnchor(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p3MustQuery(t, language, `
            (parameter_list
              .
              ((parameter_declaration) @constant
                (#match? @constant "^int")))`)
	assertQueryMatches(t, language, query, "void fun(int a, char b, int c) { };", []matchResult{m(0, "constant", "int a")})
}

// test_query_wildcard_with_immediate_first_child
func TestQueryWildcardWithImmediateFirstChild(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, "(_ . (identifier) @firstChild)")
	assertQueryMatches(t, language, query, "function name(one, two, three) { }", []matchResult{
		m(0, "firstChild", "name"),
		m(0, "firstChild", "one"),
	})
}

// test_query_on_empty_source_code
func TestQueryOnEmptySourceCode(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, "(program) @program")
	assertQueryMatches(t, language, query, "", []matchResult{m(0, "program", "")})
}

// test_query_execution_with_timeout
func TestQueryExecutionWithTimeout(t *testing.T) {
	language := queryLanguage(t, "javascript")
	count := 10_000
	source := []byte(strings.Repeat("function foo() { while (true) { } }\n", count))
	tree := parseSource(t, language, string(source))
	query := p3MustQuery(t, language, "(function_declaration) @function")

	startTime := time.Now()
	options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
		return time.Since(startTime).Microseconds() > 1000
	}}
	cursor := ts.NewQueryCursor()
	matches := 0
	for range cursor.MatchesWithOptions(query, tree.RootNode(), ts.SourceText(source), options) {
		matches++
	}
	require.Less(t, matches, count)

	matches = 0
	for range cursor.Matches(query, tree.RootNode(), source) {
		matches++
	}
	require.Equal(t, count, matches)
}

// test_query_progress_callback_lives_as_long_as_matches
func TestQueryProgressCallbackLivesAsLongAsMatches(t *testing.T) {
	language := queryLanguage(t, "javascript")
	source := []byte(strings.Repeat("function foo() {}\n", 1000))
	tree := parseSource(t, language, string(source))
	query := p3MustQuery(t, language, "(function_declaration) @function")

	callbackWasCalled := false
	options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
		callbackWasCalled = true
		return false
	}}
	cursor := ts.NewQueryCursor()
	matches := 0
	for range cursor.MatchesWithOptions(query, tree.RootNode(), ts.SourceText(source), options) {
		matches++
	}
	require.Equal(t, 1000, matches)
	require.True(t, callbackWasCalled)
}

// test_query_captures_progress_callback_stops_behind_an_open_match
func TestQueryCapturesProgressCallbackStopsBehindAnOpenMatch(t *testing.T) {
	language := queryLanguage(t, "javascript")
	numbers := make([]string, 1000)
	for i := range numbers {
		numbers[i] = strconv.Itoa(i)
	}
	source := []byte("[" + strings.Join(numbers, ",") + "];")
	tree := parseSource(t, language, string(source))

	// The first pattern stays in progress until the array ends, so every number capture finishes behind it.
	query := p3MustQuery(t, language, "(array (number) @first (string)) (number) @number")

	calls := 0
	options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
		calls++
		return true
	}}
	cursor := ts.NewQueryCursor()
	captures := 0
	for range cursor.CapturesWithOptions(query, tree.RootNode(), ts.SourceText(source), options) {
		captures++
	}
	require.Less(t, captures, 1000)
	require.Equal(t, 1, calls)
}

// test_query_captures_progress_callback_discards_in_progress_matches. Upstream
// uses json; the same document is a javascript array, with the same array,
// object, string, number and "]" nodes.
func TestQueryCapturesProgressCallbackDiscardsInProgressMatches(t *testing.T) {
	language := queryLanguage(t, "javascript")
	element := []any{[]any{map[string]int{"a": 1}, 1, 2}}
	for range 12 {
		element = append(element, true)
	}
	elements := make([]any, 50)
	for i := range elements {
		elements[i] = element
	}
	source, err := json.Marshal(elements)
	require.NoError(t, err)
	tree := parseSource(t, language, string(source))
	query := p3MustQuery(t, language, `
        (array (object) @object (string))
        (array (array (number) @number) "]" @close)
        (number) @n
        `)

	for cancelAt := 1; cancelAt <= 20; cancelAt++ {
		calls := 0
		options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
			calls++
			return calls >= cancelAt
		}}
		cursor := ts.NewQueryCursor()
		for match := range cursor.CapturesWithOptions(query, tree.RootNode(), ts.SourceText(source), options) {
			// Once the callback asked to stop, only finished matches come back.
			if calls >= cancelAt && match.PatternIndex == 1 {
				require.Len(t, match.Captures, 2, "cancelled at callback %d", cancelAt)
			}
		}
	}
}

// test_query_progress_callback_halts_for_good. The cursor API stands in for
// the Rust iterator, so the call after the cancellation reaches the cursor.
func TestQueryProgressCallbackHaltsForGood(t *testing.T) {
	language := queryLanguage(t, "javascript")
	source := strings.Repeat("function foo() {}\n", 1000)
	tree := parseSource(t, language, source)
	query := p3MustQuery(t, language, "(function_declaration) @function")

	stop := true
	options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
		return stop
	}}
	cursor := ts.NewQueryCursor()
	cursor.ExecWithOptions(query, tree.RootNode(), options)
	var match ts.QueryMatch
	count := 0
	for cursor.NextMatch(&match) {
		count++
	}
	require.Less(t, count, 1000)

	// A call after a cancellation does not resume the query.
	stop = false
	require.False(t, cursor.NextMatch(&match))
}

// test_query_progress_callback_keeps_up_with_in_progress_states
func TestQueryProgressCallbackKeepsUpWithInProgressStates(t *testing.T) {
	language := queryLanguage(t, "javascript")
	depth := 1000
	source := []byte(strings.Repeat("[", depth) + strings.Repeat("]", depth) + ";")
	tree := parseSource(t, language, string(source))

	// No array has a string, so every enclosing array keeps a state in progress.
	query := p3MustQuery(t, language, "(array (string) @string)")

	calls := 0
	options := &ts.QueryCursorOptions{ProgressCallback: func(*ts.QueryCursorState) bool {
		calls++
		return false
	}}
	cursor := ts.NewQueryCursor()
	matches := 0
	for range cursor.MatchesWithOptions(query, tree.RootNode(), ts.SourceText(source), options) {
		matches++
	}
	require.Equal(t, 0, matches)
	require.Greater(t, calls, depth, "%d calls for %d nested arrays", calls, depth)
}

// test_query_execution_with_points_causing_underflow
func TestQueryExecutionWithPointsCausingUnderflow(t *testing.T) {
	language := queryLanguage(t, "rust")
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(language))

	code := "fn main() {\n    println!(\"{:?}\", foo());\n}"
	require.True(t, parser.SetIncludedRanges([]ts.Range{{
		StartByte:  24,
		EndByte:    39,
		StartPoint: ts.Point{Row: 0, Column: 0},
		EndPoint:   ts.Point{Row: 0, Column: 0},
	}}))

	query := p3MustQuery(t, language, "(call_expression) @cap")
	cursor := ts.NewQueryCursor()

	tree := parser.ParseString(nil, []byte(code))
	require.NotNil(t, tree)
	matches := collectMatches(cursor, query, tree.RootNode(), code)

	tree.Edit(ts.InputEdit{
		StartByte:   40,
		OldEndByte:  40,
		NewEndByte:  41,
		StartPoint:  ts.Point{Row: 1, Column: 28},
		OldEndPoint: ts.Point{Row: 1, Column: 28},
		NewEndPoint: ts.Point{Row: 2, Column: 0},
	})

	tree2 := parser.ParseString(tree, []byte(code))
	require.NotNil(t, tree2)
	matches2 := collectMatches(cursor, query, tree2.RootNode(), code)

	require.Equal(t, matches, matches2)
}

// test_wildcard_behavior_before_anchor. Upstream uses a python tuple; the rust
// tuple_expression has the same children, with an optional trailing comma.
func TestWildcardBehaviorBeforeAnchor(t *testing.T) {
	language := queryLanguage(t, "rust")
	source := "fn f() {\n    (a, b);\n    (c, d,);\n}\n"

	// This query targets any named node right before a closing parenthesis.
	query := p3MustQuery(t, language, `(tuple_expression (_) @last . ")" .) @match`)
	assertQueryMatches(t, language, query, source, []matchResult{
		m(0, "match", "(a, b)", "last", "b"),
		m(0, "match", "(c, d,)", "last", "d"),
	})

	// This query targets any node right before a closing parenthesis.
	query = p3MustQuery(t, language, `(tuple_expression _ @last . ")" .) @match`)
	assertQueryMatches(t, language, query, source, []matchResult{
		m(0, "match", "(a, b)", "last", "b"),
		m(0, "match", "(c, d,)", "last", ","),
	})
}

// test_pattern_alternatives_follow_last_child_constraint
func TestPatternAlternativesFollowLastChildConstraint(t *testing.T) {
	language := queryLanguage(t, "rust")
	code := "\nfn f() {\n    if a {} // <- should NOT match\n    if b {}\n}"
	tree := parseSource(t, language, code)
	cursor := ts.NewQueryCursor()

	query := p3MustQuery(t, language, `(block
        [
            (type_cast_expression)
            (expression_statement)
        ] @last
        .
        )`)
	matches := collectMatches(cursor, query, tree.RootNode(), code)

	flippedQuery := p3MustQuery(t, language, `(block
        [
            (expression_statement)
            (type_cast_expression)
        ] @last
        .
        )`)
	flippedMatches := collectMatches(cursor, flippedQuery, tree.RootNode(), code)

	require.Equal(t, []matchResult{m(0, "last", "if b {}")}, matches)
	require.Equal(t, matches, flippedMatches)
}

// test_wildcard_parent_allows_fallible_child_patterns
func TestWildcardParentAllowsFallibleChildPatterns(t *testing.T) {
	language := queryLanguage(t, "javascript")
	source := "\nfunction foo() {\n    \"bar\"\n}\n    "
	query := p3MustQuery(t, language, `(function_declaration
          (_
            (expression_statement)
          )
        ) @part`)
	assertQueryMatches(t, language, query, source, []matchResult{
		m(0, "part", "function foo() {\n    \"bar\"\n}"),
	})
}

// test_unfinished_captures_are_not_definite_with_pending_anchors
func TestUnfinishedCapturesAreNotDefiniteWithPendingAnchors(t *testing.T) {
	language := queryLanguage(t, "javascript")
	source := "\nconst foo = [\n  1, 2, 3\n]\n"
	tree := parseSource(t, language, source)
	query := p3MustQuery(t, language, `(array (_) @foo . "]")`)

	captures := collectCaptures(ts.NewQueryCursor(), query, tree.RootNode(), source)
	matches := collectMatches(ts.NewQueryCursor(), query, tree.RootNode(), source)

	require.Equal(t, []capturePair{c("foo", "3")}, captures)
	require.Len(t, matches, 1)
	require.Equal(t, captures, matches[0].Captures)
}

// test_query_with_predicate_causing_oob_access
func TestQueryWithPredicateCausingOobAccess(t *testing.T) {
	p3MustQuery(t, queryLanguage(t, "rust"), `(call_expression
     function: (scoped_identifier
       path: (scoped_identifier (identifier) @_regex (#any-of? @_regex "Regex" "RegexBuilder") .))
     (#set! injection.language "regex"))`)
}

// test_query_allows_error_nodes_with_children
func TestQueryAllowsErrorNodesWithChildren(t *testing.T) {
	language := queryLanguage(t, "cpp")
	code := "SomeStruct foo{.bar{}};"
	tree := parseSource(t, language, code)
	query := p3MustQuery(t, language, "(initializer_list (ERROR) @error)")
	cursor := ts.NewQueryCursor()
	require.Equal(t, []matchResult{m(0, "error", ".bar")}, collectMatches(cursor, query, tree.RootNode(), code))
}

// test_last_child_anchor_looks_past_hidden_node
func TestLastChildAnchorLooksPastHiddenNode(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p3MustQuery(t, language, "(translation_unit (_) @last .)")
	assertQueryMatches(t, language, query, "enum E { A };\nint x;\nint y;\n", []matchResult{m(0, "last", "int y;")})
}
