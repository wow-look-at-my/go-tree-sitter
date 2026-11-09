package treesitter_test

import (
	"strings"
	"testing"

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
