package treesitter_test

import _ "embed"

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

func init() {
	upstreamQueryTestsPorted = append(upstreamQueryTestsPorted,
		"test_query_matches_with_simple_alternatives",
		"test_query_matches_with_alternatives_in_repetitions",
		"test_query_matches_with_alternatives_at_root",
		"test_query_matches_with_alternatives_under_fields",
		"test_query_matches_in_language_with_simple_aliases",
		"test_query_matches_with_different_tokens_with_the_same_string_value",
		"test_query_matches_with_too_many_permutations_to_track",
		"test_query_sibling_patterns_dont_match_children_of_an_error",
		"test_query_matches_with_alternatives_and_too_many_permutations_to_track",
		"test_repetitions_before_with_alternatives",
		"test_query_matches_with_anonymous_tokens",
		"test_query_matches_with_supertypes",
		"test_query_matches_within_byte_range",
		"test_query_matches_within_point_range",
		"test_query_captures_within_byte_range",
		"test_query_cursor_next_capture_with_byte_range",
		"test_query_cursor_next_capture_with_point_range",
		"test_query_matches_with_unrooted_patterns_intersecting_byte_range",
		"test_query_matches_with_wildcard_at_root_intersecting_byte_range",
		"test_query_captures_within_byte_range_assigned_after_iterating",
		"test_query_matches_within_range_of_long_repetition",
		"test_query_matches_contained_within_range",
		"test_query_matches_different_queries_same_cursor",
		"test_query_matches_with_multiple_captures_on_a_node",
		"test_query_matches_with_captured_wildcard_at_root",
		"test_query_matches_with_no_captures",
		"test_query_matches_with_repeated_fields",
		"test_query_matches_with_deeply_nested_patterns_with_fields",
		"test_query_alternation_with_inner_quantifier",
		"test_query_alternation_with_outer_quantifier",
		"test_query_matches_with_alternations_and_predicates",
		"test_query_matches_with_indefinite_step_containing_no_captures",
		"test_query_captures_basic",
		"test_query_captures_with_text_conditions",
		"test_query_captures_with_predicates",
		"test_query_captures_with_quoted_predicate_args",
		"test_query_captures_with_duplicates",
		"test_query_captures_with_many_nested_results_without_fields",
		"test_query_captures_with_many_nested_results_with_fields",
		"test_query_captures_with_too_many_nested_results",
		"test_query_captures_with_definite_pattern_containing_many_nested_matches",
		"test_query_captures_ordered_by_both_start_and_end_positions",
		"test_query_captures_with_matches_removed",
		"test_query_captures_with_matches_removed_before_they_finish",
	)
}

// mustQuery compiles a query that must be valid.
func mustQuery(t testing.TB, language *ts.Language, source string) *ts.Query {
	t.Helper()
	query, err := ts.NewQuery(language, source)
	require.NoError(t, err)
	return query
}

// matchesInByteRange runs the query over the byte range and lists the kind
// of the first capture of each match.
func matchesInByteRange(cursor *ts.QueryCursor, query *ts.Query, tree *ts.Tree, source string, offset uint32) []string {
	cursor.SetByteRange(offset, offset)
	kinds := []string{}
	for match := range cursor.Matches(query, tree.RootNode(), []byte(source)) {
		if len(match.Captures) > 0 {
			kinds = append(kinds, match.Captures[0].Node.Type())
		}
	}
	return kinds
}

// Upstream: test_query_matches_with_simple_alternatives.
func TestQueryMatchesWithSimpleAlternatives(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(pair
			key: [(property_identifier) (string)] @key
			value: [(function_expression) @val1 (arrow_function) @val2])
	`)
	assertQueryMatches(t, language, query, `
		a = {
			b: c,
			'd': e => f,
			g: {
				h: function i() {},
				'x': null,
				j: _ => k
			},
			'l': function m() {},
		};
	`, []matchResult{
		m(0, "key", "'d'", "val2", "e => f"),
		m(0, "key", "h", "val1", "function i() {}"),
		m(0, "key", "j", "val2", "_ => k"),
		m(0, "key", "'l'", "val1", "function m() {}"),
	})
}

// Upstream: test_query_matches_with_alternatives_in_repetitions.
func TestQueryMatchesWithAlternativesInRepetitions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(array
			[(identifier) (string)] @el
			.
			(
				","
				.
				[(identifier) (string)] @el
			)*)
	`)
	assertQueryMatches(t, language, query, `
		a = [b, 'c', d, 1, e, 'f', 'g', h];
	`, []matchResult{
		m(0, "el", "b", "el", "'c'", "el", "d"),
		m(0, "el", "e", "el", "'f'", "el", "'g'", "el", "h"),
	})
}

// Upstream: test_query_matches_with_alternatives_at_root.
func TestQueryMatchesWithAlternativesAtRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		[
			"if"
			"else"
			"function"
			"throw"
			"return"
		] @keyword
	`)
	assertQueryMatches(t, language, query, testQueryMatchesWithAlternativesAtRootCpp, []matchResult{
		m(0, "keyword", "function"),
		m(0, "keyword", "if"),
		m(0, "keyword", "return"),
		m(0, "keyword", "else"),
		m(0, "keyword", "throw"),
	})
}

// Upstream: test_query_matches_with_alternatives_under_fields.
func TestQueryMatchesWithAlternativesUnderFields(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(assignment_expression
			left: [
				(identifier) @variable
				(member_expression property: (property_identifier) @variable)
			])
	`)
	assertQueryMatches(t, language, query, `
		a = b;
		b = c.d;
		e.f = g;
		h.i = j.k;
	`, []matchResult{
		m(0, "variable", "a"),
		m(0, "variable", "b"),
		m(0, "variable", "f"),
		m(0, "variable", "i"),
	})
}

// Upstream: test_query_matches_in_language_with_simple_aliases.
// Upstream uses HTML, where several tokens alias to tag_name. Here JavaScript
// aliases a JSX attribute name and an identifier to property_identifier. The
// expectation comes from the pinned CLI.
func TestQueryMatchesInLanguageWithSimpleAliases(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, "(property_identifier) @prop")
	assertQueryMatches(t, language, query, `x = <div data-y="1" z={a.b} />;`, []matchResult{
		m(0, "prop", "data-y"),
		m(0, "prop", "z"),
		m(0, "prop", "b"),
	})
}

// Upstream: test_query_matches_with_different_tokens_with_the_same_string_value.
func TestQueryMatchesWithDifferentTokensWithTheSameStringValue(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		"<" @less
		">" @greater
	`)
	assertQueryMatches(t, language, query, "const A: B<C> = d < e || f > g;", []matchResult{
		m(0, "less", "<"),
		m(1, "greater", ">"),
		m(0, "less", "<"),
		m(1, "greater", ">"),
	})
}

// Upstream: test_query_matches_with_too_many_permutations_to_track.
func TestQueryMatchesWithTooManyPermutationsToTrack(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(array (identifier) @pre (identifier) @post)
	`)
	source := "[" + strings.Repeat("hello, ", 50) + "];"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetMatchLimit(32)

	// Some permutations drop. A subset of the results comes back.
	got := collectMatches(cursor, query, tree.RootNode(), source)
	require.NotEmpty(t, got)
	require.Equal(t, m(0, "pre", "hello", "post", "hello"), got[0])
	require.True(t, cursor.DidExceedMatchLimit())
}

// Upstream: test_query_sibling_patterns_dont_match_children_of_an_error.
func TestQuerySiblingPatternsDontMatchChildrenOfAnError(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		("{" @open "}" @close)

		[
		  (line_comment)
		  (block_comment)
		] @comment

		("<" @first "<" @second)
	`)

	// Most of the document is an ERROR. A non-rooted pattern must not match the direct children of an ERROR node.
	source := testQuerySiblingPatternsDontMatchChildrenOfAnErrorRs
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []matchResult{
		m(0, "open", "{", "close", "}"),
		m(1, "comment", "// comment 1"),
		m(1, "comment", "/* comment 2 */"),
		m(1, "comment", "// comment 3"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_matches_with_alternatives_and_too_many_permutations_to_track.
func TestQueryMatchesWithAlternativesAndTooManyPermutationsToTrack(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(
			(comment) @doc
			; not immediate
			(class_declaration) @class
		)

		(call_expression
			function: [
				(identifier) @function
				(member_expression property: (property_identifier) @method)
			])
	`)
	source := strings.Repeat("/* hi */ a.b(); ", 50)
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetMatchLimit(32)

	expected := []matchResult{}
	for range 50 {
		expected = append(expected, m(1, "method", "b"))
	}
	require.Equal(t, expected, collectMatches(cursor, query, tree.RootNode(), source))
	require.True(t, cursor.DidExceedMatchLimit())
}

// Upstream: test_repetitions_before_with_alternatives.
func TestRepetitionsBeforeWithAlternatives(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		(
			(line_comment)* @comment
			.
			[
				(struct_item name: (_) @name)
				(function_item name: (_) @name)
				(enum_item name: (_) @name)
				(impl_item type: (_) @name)
			]
		)
	`)
	assertQueryMatches(t, language, query, testRepetitionsBeforeWithAlternativesRs, []matchResult{
		m(0, "comment", "// a", "comment", "// b", "name", "c"),
		m(0, "comment", "// d", "comment", "// e", "name", "F"),
	})
}

// Upstream: test_query_matches_with_anonymous_tokens.
func TestQueryMatchesWithAnonymousTokens(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		";" @punctuation
		"&&" @operator
		"\"" @quote
	`)
	assertQueryMatches(t, language, query, `foo(a && "b");`, []matchResult{
		m(1, "operator", "&&"),
		m(2, "quote", `"`),
		m(2, "quote", `"`),
		m(0, "punctuation", ";"),
	})
}

// Upstream: test_query_matches_with_supertypes. Upstream uses Python. Here
// the JavaScript supertypes expression and primary_expression take the same
// places: as a child. That JavaScript is under a field, and as the supertype
// in a `supertype/subtype` node. The expectation comes from the pinned CLI.
func TestQueryMatchesWithSupertypes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(arguments (expression) @arg)

		(pair
			value: (expression) @kw_arg)

		(assignment_expression
		  left: (identifier) @var_def)

		(primary_expression/identifier) @var_ref
	`)
	assertQueryMatches(t, language, query, `
		a = b.c(
			[d],
			// a comment
			{e: f}
		)
	`, []matchResult{
		m(2, "var_def", "a"),
		m(3, "var_ref", "b"),
		m(0, "arg", "[d]"),
		m(3, "var_ref", "d"),
		m(0, "arg", "{e: f}"),
		m(1, "kw_arg", "f"),
		m(3, "var_ref", "f"),
	})
}

// Upstream: test_query_matches_within_byte_range.
func TestQueryMatchesWithinByteRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, "(identifier) @element")
	source := "[a, b, c, d, e, f, g]"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	cursor.SetByteRange(0, 8)
	require.Equal(t, []matchResult{
		m(0, "element", "a"),
		m(0, "element", "b"),
		m(0, "element", "c"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	cursor.SetByteRange(5, 15)
	require.Equal(t, []matchResult{
		m(0, "element", "c"),
		m(0, "element", "d"),
		m(0, "element", "e"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	// An end byte of zero means there is no end.
	cursor.SetByteRange(12, 0)
	require.Equal(t, []matchResult{
		m(0, "element", "e"),
		m(0, "element", "f"),
		m(0, "element", "g"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_matches_within_point_range.
func TestQueryMatchesWithinPointRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, "(identifier) @element")
	source := unindent(`
		[
		  a, b,
		  c, d,
		  e, f,
		  g, h,
		  i, j,
		  k, l,
		]
	`)
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	cursor.SetPointRange(ts.Point{Row: 1, Column: 0}, ts.Point{Row: 2, Column: 3})
	require.Equal(t, []matchResult{
		m(0, "element", "a"),
		m(0, "element", "b"),
		m(0, "element", "c"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	cursor.SetPointRange(ts.Point{Row: 2, Column: 0}, ts.Point{Row: 3, Column: 3})
	require.Equal(t, []matchResult{
		m(0, "element", "c"),
		m(0, "element", "d"),
		m(0, "element", "e"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	// A zero end point means there is no end.
	cursor.SetPointRange(ts.Point{Row: 4, Column: 1}, ts.Point{Row: 0, Column: 0})
	require.Equal(t, []matchResult{
		m(0, "element", "g"),
		m(0, "element", "h"),
		m(0, "element", "i"),
		m(0, "element", "j"),
		m(0, "element", "k"),
		m(0, "element", "l"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_within_byte_range.
func TestQueryCapturesWithinByteRange(t *testing.T) {
	language := queryLanguage(t, "c")
	query := mustQuery(t, language, `
		(call_expression
			function: (identifier) @function
			arguments: (argument_list (string_literal) @string.arg))

		(string_literal) @string
	`)
	source := `DEFUN ("safe-length", Fsafe_length, Ssafe_length, 1, 1, 0)`
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetByteRange(3, 27)
	require.Equal(t, []capturePair{
		c("function", "DEFUN"),
		c("string.arg", `"safe-length"`),
		c("string", `"safe-length"`),
	}, collectCaptures(cursor, query, tree.RootNode(), source))
}

// nextCaptureQuery and nextCaptureSource serve both next_capture range tests.
const nextCaptureQuery = `(function_declaration name: (identifier) @function)
	(member_expression property: (property_identifier) @property)
	((identifier) @variable)`

//go:embed testdata/next-capture-source.cpp
var nextCaptureSource string

// Upstream: test_query_cursor_next_capture_with_byte_range.
// Rewritten from Python to JavaScript.
func TestQueryCursorNextCaptureWithByteRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, nextCaptureQuery)
	source := nextCaptureSource
	tree := parseSource(t, language, source)
	start := uint32(strings.Index(source, "\n") + 1)
	cursor := ts.NewQueryCursor()
	cursor.SetByteRange(start, start+5)
	require.Equal(t, []capturePair{c("variable", "foo")},
		collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_cursor_next_capture_with_point_range.
// Rewritten from Python to JavaScript.
func TestQueryCursorNextCaptureWithPointRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, nextCaptureQuery)
	source := nextCaptureSource
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetPointRange(ts.Point{Row: 1, Column: 0}, ts.Point{Row: 1, Column: 5})
	require.Equal(t, []capturePair{c("variable", "foo")},
		collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_matches_with_unrooted_patterns_intersecting_byte_range.
func TestQueryMatchesWithUnrootedPatternsIntersectingByteRange(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		("{" @left "}" @right)
		("<" @left ">" @right)
	`)
	source := "mod a { fn a<B: C, D: E>(f: B) { g(f) } }"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	// Within the type parameter list.
	offset := uint32(strings.Index(source, "D: E>"))
	cursor.SetByteRange(offset, offset)
	require.Equal(t, []matchResult{
		m(1, "left", "<", "right", ">"),
		m(0, "left", "{", "right", "}"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	// From the type parameter list to the function body.
	endOffset := uint32(strings.Index(source, "g(f)"))
	cursor.SetByteRange(offset, endOffset)
	require.Equal(t, []matchResult{
		m(1, "left", "<", "right", ">"),
		m(0, "left", "{", "right", "}"),
		m(0, "left", "{", "right", "}"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

//go:embed testdata/test-query-matches-with-alternatives-at-root.cpp
var testQueryMatchesWithAlternativesAtRootCpp string

//go:embed testdata/test-query-sibling-patterns-dont-match-children-of-an-error.rs
var testQuerySiblingPatternsDontMatchChildrenOfAnErrorRs string

//go:embed testdata/test-repetitions-before-with-alternatives.rs
var testRepetitionsBeforeWithAlternativesRs string
