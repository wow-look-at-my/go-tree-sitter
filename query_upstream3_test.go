package treesitter_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// This file ports a later part of crates/cli/src/tests/query_test.rs
// (tree-sitter v0.27.2). This covers the tests from
// test_query_captures_and_matches_iterators_are_fused to the end of the file.

func init() {
	upstreamQueryTestsPorted = append(upstreamQueryTestsPorted,
		"test_query_captures_and_matches_iterators_are_fused",
		"test_query_text_callback_returns_chunks",
		"test_query_start_end_byte_for_pattern",
		"test_query_capture_names",
		"test_query_lifetime_is_separate_from_nodes_lifetime",
		"test_query_with_no_patterns",
		"test_query_comments",
		"test_query_disable_pattern",
		"test_query_deep_clone",
		"test_query_alternative_predicate_prefix",
		"test_query_is_pattern_guaranteed_at_step",
		"test_query_is_pattern_rooted",
		"test_query_is_pattern_non_local",
		"test_capture_quantifiers",
		"test_query_quantified_captures",
		"test_query_max_start_depth",
		"test_query_error_does_not_oob",
		"test_consecutive_zero_or_modifiers",
		"test_query_max_start_depth_more",
		"test_query_with_first_child_in_group_is_anchor",
		"test_query_wildcard_with_immediate_first_child",
		"test_query_on_empty_source_code",
		"test_query_execution_with_timeout",
		"test_query_progress_callback_lives_as_long_as_matches",
		"test_query_captures_progress_callback_stops_behind_an_open_match",
		"test_query_captures_progress_callback_discards_in_progress_matches",
		"test_query_progress_callback_halts_for_good",
		"test_query_progress_callback_keeps_up_with_in_progress_states",
		"test_query_execution_with_points_causing_underflow",
		"test_wildcard_behavior_before_anchor",
		"test_pattern_alternatives_follow_last_child_constraint",
		"test_wildcard_parent_allows_fallible_child_patterns",
		"test_unfinished_captures_are_not_definite_with_pending_anchors",
		"test_query_with_predicate_causing_oob_access",
		"test_query_allows_error_nodes_with_children",
		"test_last_child_anchor_looks_past_hidden_node",
	)
	upstreamQueryTestsSkipped["test_query_random"] = "needs Pattern::random_pattern_in_tree, a randomized pattern generator"
	upstreamQueryTestsSkipped["test_grammar_with_aliased_literal_query"] = "needs generate_parser to build a grammar from JSON"
	upstreamQueryTestsSkipped["test_query_compiler_oob_access"] = "a UBSAN check against the java grammar, which is not bundled"
	upstreamQueryTestsSkipped["test_query_with_anonymous_error_node"] = "needs get_test_fixture_language(\"anonymous_error\")"
	upstreamQueryTestsSkipped["test_query_assertion_on_unreachable_node_with_child"] = "needs generate_parser to build a grammar from grammar.js"
	upstreamQueryTestsSkipped["test_query_supertype_with_anonymous_node"] = "needs generate_parser to build a grammar from grammar.js"
	upstreamQueryTestsSkipped["test_last_child_anchor_looks_past_hidden_repeat"] = "needs get_test_fixture_language(\"last_child_anchor_past_hidden_repeat\")"
}

func p3MustQuery(t testing.TB, language *ts.Language, source string) *ts.Query {
	t.Helper()
	query, err := ts.NewQuery(language, source)
	require.NoError(t, err, "query: %s", source)
	return query
}

// test_query_captures_and_matches_iterators_are_fused. The cursor keeps
// reporting the end after it reached the end once.
func TestQueryCapturesAndMatchesIteratorsAreFused(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, "\n            (comment) @comment\n            ")
	source := "\n          // one\n          // two\n          // three\n          /* unfinished\n        "
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	cursor.Exec(query, tree.RootNode())
	var match ts.QueryMatch
	for range 3 {
		index, ok := cursor.NextCapture(&match)
		require.True(t, ok)
		require.Equal(t, uint32(0), match.Captures[index].Index)
	}
	for range 3 {
		_, ok := cursor.NextCapture(&match)
		require.False(t, ok)
	}

	cursor.Exec(query, tree.RootNode())
	for range 3 {
		require.True(t, cursor.NextMatch(&match))
		require.Equal(t, uint32(0), match.Captures[0].Index)
	}
	for range 3 {
		require.False(t, cursor.NextMatch(&match))
	}
}

// test_query_text_callback_returns_chunks. The Go TextProvider returns one
// slice, so the provider joins the 3-byte chunks of the node's range.
func TestQueryTextCallbackReturnsChunks(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, `
            ((identifier) @leading_upper
             (#match? @leading_upper "^[A-Z][A-Z_]*[a-z]"))
            ((identifier) @all_upper
             (#match? @all_upper "^[A-Z][A-Z_]*$"))
            ((identifier) @all_lower
             (#match? @all_lower "^[a-z][a-z_]*$"))
            `)
	source := "SOMETHING[a] = transform(AnotherThing[b].property[c], PARAMETER);"

	var sourceChunks [][]byte
	for i := 0; i < len(source); i += 3 {
		sourceChunks = append(sourceChunks, []byte(source[i:min(i+3, len(source))]))
	}
	chunksInRange := func(start, end int) [][]byte {
		var result [][]byte
		offset := 0
		for _, chunk := range sourceChunks {
			endOffset := offset + len(chunk)
			if offset < end && start < endOffset {
				endInChunk := min(end-offset, len(chunk))
				startInChunk := max(start, offset) - offset
				result = append(result, chunk[startInChunk:endInChunk])
			}
			offset = endOffset
		}
		return result
	}
	join := func(chunks [][]byte) []byte {
		var b []byte
		for _, chunk := range chunks {
			b = append(b, chunk...)
		}
		return b
	}
	require.Equal(t, "SOMETHING", string(join(chunksInRange(0, 9))))
	require.Equal(t, "transform", string(join(chunksInRange(15, 24))))

	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	provider := func(node ts.Node) []byte {
		return join(chunksInRange(int(node.StartByte()), int(node.EndByte())))
	}
	got := []capturePair{}
	for match, index := range cursor.CapturesWithOptions(query, tree.RootNode(), provider, nil) {
		capture := match.Captures[index]
		got = append(got, c(query.CaptureNameForID(capture.Index), nodeText(capture.Node, source)))
	}
	require.Equal(t, []capturePair{
		c("all_upper", "SOMETHING"),
		c("all_lower", "a"),
		c("all_lower", "transform"),
		c("leading_upper", "AnotherThing"),
		c("all_lower", "b"),
		c("all_lower", "c"),
		c("all_upper", "PARAMETER"),
	}, got)
}

// test_query_start_end_byte_for_pattern
func TestQueryStartEndByteForPattern(t *testing.T) {
	language := queryLanguage(t, "javascript")
	patterns1 := "\"+\" @operator\n\"-\" @operator\n\"*\" @operator\n\"=\" @operator\n\"=>\" @operator\n"
	patterns2 := "(identifier) @a\n(string) @b\n"
	patterns3 := "((identifier) @b (#match? @b i))\n(function_declaration name: (identifier) @c)\n(method_definition name: (property_identifier) @d)\n"
	query := p3MustQuery(t, language, patterns1+patterns2+patterns3)

	require.Equal(t, uint32(0), query.StartByteForPattern(0))
	require.Equal(t, uint32(len("\"+\" @operator\n")), query.EndByteForPattern(0))
	require.Equal(t, uint32(len(patterns1)), query.StartByteForPattern(5))
	require.Equal(t, uint32(len(patterns1)+len("(identifier) @a\n")), query.EndByteForPattern(5))
	require.Equal(t, uint32(len(patterns1)+len(patterns2)), query.StartByteForPattern(7))
	require.Equal(t, uint32(len(patterns1)+len(patterns2)+len("((identifier) @b (#match? @b i))\n")), query.EndByteForPattern(7))
}

// test_query_capture_names
func TestQueryCaptureNames(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, `
            (if_statement
              condition: (parenthesized_expression (binary_expression
                left: _ @left-operand
                operator: "||"
                right: _ @right-operand))
              consequence: (statement_block) @body)

            (while_statement
              condition: _ @loop-condition)
            `)
	require.Equal(t, []string{"left-operand", "right-operand", "body", "loop-condition"}, query.CaptureNames())
}

// test_query_lifetime_is_separate_from_nodes_lifetime. A node outlives the
// query and the cursor that found it.
func TestQueryLifetimeIsSeparateFromNodesLifetime(t *testing.T) {
	queryText := "(call_expression) @call"
	source := "a(1); b(2);"
	language := queryLanguage(t, "javascript")
	tree := parseSource(t, language, source)

	takeFirstNodeFromMatches := func(node ts.Node) ts.Node {
		query := p3MustQuery(t, queryLanguage(t, "javascript"), queryText)
		cursor := ts.NewQueryCursor()
		for match := range cursor.Matches(query, node, []byte(source)) {
			return match.Captures[0].Node
		}
		t.Fatal("no match")
		return ts.Node{}
	}
	require.Equal(t, "call_expression", takeFirstNodeFromMatches(tree.RootNode()).Type())

	takeFirstNodeFromCaptures := func(node ts.Node) ts.Node {
		query := p3MustQuery(t, queryLanguage(t, "javascript"), queryText)
		cursor := ts.NewQueryCursor()
		for match := range cursor.Captures(query, node, []byte(source)) {
			return match.Captures[0].Node
		}
		t.Fatal("no capture")
		return ts.Node{}
	}
	require.Equal(t, "call_expression", takeFirstNodeFromCaptures(tree.RootNode()).Type())
}

// test_query_with_no_patterns
func TestQueryWithNoPatterns(t *testing.T) {
	query := p3MustQuery(t, queryLanguage(t, "javascript"), "")
	require.Empty(t, query.CaptureNames())
	require.Equal(t, 0, query.PatternCount())
}

// test_query_comments
func TestQueryComments(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, `
                ; this is my first comment
                ; i have two comments here
                (function_declaration
                    ; there is also a comment here
                    ; and here
                    name: (identifier) @fn-name)`)
	assertQueryMatches(t, language, query, "function one() { }", []matchResult{m(0, "fn-name", "one")})
}

// test_query_disable_pattern
func TestQueryDisablePattern(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, `
                (function_declaration
                    name: (identifier) @name)
                (function_declaration
                    body: (statement_block) @body)
                (class_declaration
                    name: (identifier) @name)
                (class_declaration
                    body: (class_body) @body)
            `)
	query.DisablePattern(0)
	query.DisablePattern(2)
	assertQueryMatches(t, language, query, "class A { constructor() {} } function b() { return 1; }", []matchResult{
		m(3, "body", "{ constructor() {} }"),
		m(1, "body", "{ return 1; }"),
	})
}

// test_query_deep_clone. Copy is the deep clone.
func TestQueryDeepClone(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p3MustQuery(t, language, `
                (function_declaration
                    name: (identifier) @name)
                (function_declaration
                    body: (statement_block) @body)
            `)
	clone := query.Copy()
	clone.DisablePattern(1)

	source := "function foo() { return 1; }"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []matchResult{m(0, "name", "foo")}, collectMatches(cursor, clone, tree.RootNode(), source))
	require.Equal(t, []matchResult{
		m(0, "name", "foo"),
		m(1, "body", "{ return 1; }"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// test_query_alternative_predicate_prefix
func TestQueryAlternativePredicatePrefix(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p3MustQuery(t, language, `
            ((call_expression
              function: (identifier) @keyword
              arguments: (argument_list
                          (string_literal) @function))
             (.eq? @keyword "DEFUN"))
        `)
	source := `
            DEFUN ("identity", Fidentity, Sidentity, 1, 1, 0,
                   doc: /* Return the argument unchanged.  */
                   attributes: const)
              (Lisp_Object arg)
            {
              return arg;
            }
        `
	assertQueryMatches(t, language, query, source, []matchResult{
		m(0, "keyword", "DEFUN", "function", `"identity"`),
	})
}

// test_query_is_pattern_guaranteed_at_step. The python row is rewritten
// against javascript, which has the same expression_statement and string.
// The json, ruby and java rows are left out: their answers depend on those
// grammars' rules.
func TestQueryIsPatternGuaranteedAtStep(t *testing.T) {
	type result struct {
		substring  string
		isDefinite bool
	}
	rows := []struct {
		description string
		language    string
		pattern     string
		results     []result
	}{
		{
			description: "no guaranteed steps",
			language:    "javascript",
			pattern:     `(expression_statement (string))`,
			results:     []result{{"expression_statement", false}, {"string", false}},
		},
		{
			description: "all guaranteed steps",
			language:    "javascript",
			pattern:     `(object "{" "}")`,
			results:     []result{{"object", false}, {"{", true}, {"}", true}},
		},
		{
			description: "a fallible step that is optional",
			language:    "javascript",
			pattern:     `(object "{" (identifier)? @foo "}")`,
			results:     []result{{"object", false}, {"{", true}, {"(identifier)?", false}, {"}", true}},
		},
		{
			description: "multiple fallible steps that are optional",
			language:    "javascript",
			pattern:     `(object "{" (identifier)? @id1 ("," (identifier) @id2)? "}")`,
			results: []result{
				{"object", false},
				{"{", true},
				{"(identifier)? @id1", false},
				{`","`, false},
				{"}", true},
			},
		},
		{
			description: "guaranteed step after fallibe step",
			language:    "javascript",
			pattern:     `(pair (property_identifier) ":")`,
			results:     []result{{"pair", false}, {"property_identifier", false}, {":", true}},
		},
		{
			description: "fallible step in between two guaranteed steps",
			language:    "javascript",
			pattern: `(ternary_expression
                condition: (_)
                "?"
                consequence: (call_expression)
                ":"
                alternative: (_))`,
			results: []result{
				{"condition:", false},
				{`"?"`, false},
				{"consequence:", false},
				{`":"`, true},
				{"alternative:", true},
			},
		},
		{
			description: "one guaranteed step after a repetition",
			language:    "javascript",
			pattern:     `(object "{" (_) "}")`,
			results:     []result{{"object", false}, {"{", false}, {"(_)", false}, {"}", true}},
		},
		{
			description: "a guaranteed step with a field",
			language:    "javascript",
			pattern:     `(binary_expression left: (expression) right: (_))`,
			results:     []result{{"binary_expression", false}, {"(expression)", false}, {"(_)", true}},
		},
		{
			description: "multiple guaranteed steps with fields",
			language:    "javascript",
			pattern:     `(function_declaration name: (identifier) body: (statement_block))`,
			results:     []result{{"function_declaration", false}, {"identifier", true}, {"statement_block", true}},
		},
		{
			description: "nesting, one guaranteed step",
			language:    "javascript",
			pattern: `
                (function_declaration
                    name: (identifier)
                    body: (statement_block "{" (expression_statement) "}"))`,
			results: []result{
				{"function_declaration", false},
				{"identifier", false},
				{"statement_block", false},
				{"{", false},
				{"expression_statement", false},
				{"}", true},
			},
		},
		{
			description: "nesting, no guaranteed steps",
			language:    "javascript",
			pattern: `
            (call_expression
                function: (member_expression
                  property: (property_identifier) @template-tag)
                arguments: (template_string)) @template-call
            `,
			results: []result{{"property_identifier", false}, {"template_string", false}},
		},
		{
			description: "a guaranteed step after a nested node",
			language:    "javascript",
			pattern: `
            (subscript_expression
                object: (member_expression
                    object: (identifier) @obj
                    property: (property_identifier) @prop)
                "[")
            `,
			results: []result{{"identifier", false}, {"property_identifier", false}, {"[", true}},
		},
		{
			description: "a step that is fallible due to a predicate",
			language:    "javascript",
			pattern: `
            (subscript_expression
                object: (member_expression
                    object: (identifier) @obj
                    property: (property_identifier) @prop)
                "["
                (#match? @prop "foo"))
            `,
			results: []result{{"identifier", false}, {"property_identifier", false}, {"[", true}},
		},
		{
			description: "alternation where one branch has guaranteed steps",
			language:    "javascript",
			pattern: `
            [
                (unary_expression (identifier))
                (call_expression
                  function: (_)
                  arguments: (_))
                (binary_expression right: (call_expression))
            ]
            `,
			results: []result{
				{"identifier", false},
				{"right:", false},
				{"function:", true},
				{"arguments:", true},
			},
		},
		{
			description: "long, but not too long to analyze",
			language:    "javascript",
			pattern: `
            (object "{" (pair) (pair) (pair) (pair) "}")
            `,
			results: []result{
				{`"{"`, false},
				{"(pair)", false},
				{`(pair) "}"`, false},
				{`"}"`, true},
			},
		},
		{
			description: "too long to analyze",
			language:    "javascript",
			pattern: `
            (object "{" (pair) (pair) (pair) (pair) (pair) (pair) (pair) (pair) (pair) (pair) (pair) (pair) "}")
            `,
			results: []result{
				{`"{"`, false},
				{"(pair)", false},
				{`(pair) "}"`, false},
				{`"}"`, false},
			},
		},
	}
	for _, row := range rows {
		query := p3MustQuery(t, queryLanguage(t, row.language), row.pattern)
		for _, r := range row.results {
			offset := strings.Index(row.pattern, r.substring)
			require.GreaterOrEqual(t, offset, 0, "%s: no %q in the pattern", row.description, r.substring)
			require.Equal(t, r.isDefinite, query.IsPatternGuaranteedAtStep(uint32(offset)),
				"Description: %s, Pattern: %q, substring: %q", row.description,
				strings.Join(strings.Fields(row.pattern), " "), r.substring)
		}
	}
}

// test_query_is_pattern_rooted. Upstream uses python. Rootedness depends only
// on the shape of the pattern, so the rows use the javascript names for the
// same constructs.
func TestQueryIsPatternRooted(t *testing.T) {
	rows := []struct {
		description string
		pattern     string
		isRooted    bool
	}{
		{"simple token", `(identifier)`, true},
		{"simple non-terminal", `(function_declaration name: (identifier))`, true},
		{"alternative of many tokens", `["if" "function" (identifier) (comment)]`, true},
		{"alternative of many non-terminals", `[
                (function_declaration name: (identifier))
                (class_declaration name: (identifier))
                (statement_block)
            ]`, true},
		{"two siblings", `("{" "}")`, false},
		{"top-level repetition", `(comment)*`, false},
		{"alternative where one option has two siblings", `[
                (statement_block)
                (class_declaration)
                ("(" ")")
                (function_declaration)
            ]`, false},
		{"alternative where one option has a top-level repetition", `[
                (statement_block)
                (class_declaration)
                (comment)*
                (function_declaration)
            ]`, false},
	}
	language := queryLanguage(t, "javascript")
	for _, row := range rows {
		query := p3MustQuery(t, language, row.pattern)
		require.Equal(t, row.isRooted, query.IsPatternRooted(0),
			"Description: %s, Pattern: %q", row.description, strings.Join(strings.Fields(row.pattern), " "))
	}
}

// test_query_is_pattern_non_local. The python rows are rewritten against
// javascript: an argument list, a statement block and a program each repeat
// their children there too.
func TestQueryIsPatternNonLocal(t *testing.T) {
	rows := []struct {
		description string
		pattern     string
		isNonLocal  bool
	}{
		{"simple token", `(identifier)`, false},
		{"siblings that can occur in an argument list", `((identifier) (identifier))`, true},
		{"siblings that can occur in a statement block", `((return_statement) (return_statement))`, true},
		{"siblings that can occur in a source file", `((function_declaration) (class_declaration))`, true},
		{"siblings that can't occur in any repetition", `("{" "}")`, false},
		{"siblings that can't occur in any repetition, wildcard root", `(_ "{" "}") @foo`, false},
		{"siblings that can occur in a class body, wildcard root", `(_ (method_definition) (method_definition)) @foo`, true},
		{"top-level repetitions that can occur in a class body", `(method_definition)+ @foo`, true},
		{"top-level repetitions that can occur in a statement block", `(return_statement)+ @foo`, true},
		{"rooted pattern that can occur in a statement block", `(return_statement) @foo`, false},
	}
	language := queryLanguage(t, "javascript")
	for _, row := range rows {
		query := p3MustQuery(t, language, row.pattern)
		require.Equal(t, row.isNonLocal, query.IsPatternNonLocal(0),
			"Description: %s, Pattern: %q", row.description, row.pattern)
	}
}

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
	trues := make([]string, 12)
	for i := range trues {
		trues[i] = "true"
	}
	element := `[[{"a":1},1,2],` + strings.Join(trues, ",") + "]"
	elements := make([]string, 50)
	for i := range elements {
		elements[i] = element
	}
	source := []byte("[" + strings.Join(elements, ",") + "]")
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
