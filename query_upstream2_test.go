package treesitter_test

import (
	"fmt"
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
	assertQueryMatches(t, language, query, `
		function a(b, c, d) {
			if (b) {
				return c;
			} else {
				throw d;
			}
		}
	`, []matchResult{
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

	// Most of the document is an ERROR. A non-rooted pattern must not match
	// the direct children of an ERROR node.
	source := `
            fn a() {}

            <<<<<<<<<< add pub b fn () {}
            // comment 1
            pub fn b() {
            /* comment 2 */
            ==========
            pub fn c() {
            // comment 3
            >>>>>>>>>> add pub c fn () {}
            }
        `
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
	assertQueryMatches(t, language, query, `
		// a
		// b
		fn c() {}

		// d
		// e
		impl F {}
	`, []matchResult{
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

const nextCaptureSource = "function func() {\n  foo.bar.baz()\n}\n"

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

// Upstream: test_query_matches_with_wildcard_at_root_intersecting_byte_range.
// Upstream uses Python. Here JavaScript nests a class, a method and an if
// statement, and a wildcard root matches each by its body or consequence
// field. The class body is a class_body, so the alternation has a third
// branch for it.
func TestQueryMatchesWithWildcardAtRootIntersectingByteRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		[
			(_ body: (class_body))
			(_ body: (statement_block))
			(_ consequence: (statement_block))
		] @indent
	`)
	source := strings.TrimSpace(unindent(`
		class A {
		  b() {
		    if (c) {
		      d
		    } else {
		      e
		    }
		  }
		}
	`))
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	// After the first line of the class.
	offset := uint32(strings.Index(source, "A {") + 3)
	require.Equal(t, []string{"class_declaration"},
		matchesInByteRange(cursor, query, tree, source, offset))

	// After the first line of the method.
	offset = uint32(strings.Index(source, "b() {") + 5)
	require.Equal(t, []string{"class_declaration", "method_definition"},
		matchesInByteRange(cursor, query, tree, source, offset))

	// After the first line of the if statement.
	offset = uint32(strings.Index(source, "(c) {") + 5)
	require.Equal(t, []string{"class_declaration", "method_definition", "if_statement"},
		matchesInByteRange(cursor, query, tree, source, offset))
}

// Upstream: test_query_captures_within_byte_range_assigned_after_iterating.
func TestQueryCapturesWithinByteRangeAssignedAfterIterating(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		(function_item
			name: (identifier) @fn_name)

		(mod_item
			name: (identifier) @mod_name
			body: (declaration_list
				"{" @lbrace
				"}" @rbrace))

		; functions that return Result<()>
		((function_item
			return_type: (generic_type
				type: (type_identifier) @result
				type_arguments: (type_arguments
					(unit_type)))
			body: _ @fallible_fn_body)
		 (#eq? @result "Result"))
	`)
	source := `
        mod m1 {
            mod m2 {
                fn f1() -> Option<()> { Some(()) }
            }
            fn f2() -> Result<()> { Ok(()) }
            fn f3() {}
        }
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	// Take captures, then move to a range that only partly covers some matches.
	var first, rest []capturePair
	for match, index := range cursor.Captures(query, tree.RootNode(), []byte(source)) {
		capture := match.Captures[index]
		pair := c(query.CaptureNameForID(capture.Index), nodeText(capture.Node, source))
		if len(first) < 5 {
			first = append(first, pair)
			if len(first) == 5 {
				cursor.SetByteRange(uint32(strings.Index(source, "Ok")), uint32(len(source)))
			}
			continue
		}
		rest = append(rest, pair)
	}
	require.Equal(t, []capturePair{
		c("mod_name", "m1"),
		c("lbrace", "{"),
		c("mod_name", "m2"),
		c("lbrace", "{"),
		c("fn_name", "f1"),
	}, first)
	require.Equal(t, []capturePair{
		c("fallible_fn_body", "{ Ok(()) }"),
		c("fn_name", "f3"),
		c("rbrace", "}"),
	}, rest)
}

// Upstream: test_query_matches_within_range_of_long_repetition.
func TestQueryMatchesWithinRangeOfLongRepetition(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := mustQuery(t, language, `
		(function_item name: (identifier) @fn-name)
	`)
	source := unindent(`
		fn zero() {}
		fn one() {}
		fn two() {}
		fn three() {}
		fn four() {}
		fn five() {}
		fn six() {}
		fn seven() {}
		fn eight() {}
		fn nine() {}
		fn ten() {}
		fn eleven() {}
		fn twelve() {}
	`)
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetPointRange(ts.Point{Row: 8, Column: 0}, ts.Point{Row: 20, Column: 0})
	require.Equal(t, []matchResult{
		m(0, "fn-name", "eight"),
		m(0, "fn-name", "nine"),
		m(0, "fn-name", "ten"),
		m(0, "fn-name", "eleven"),
		m(0, "fn-name", "twelve"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_matches_contained_within_range.
// Upstream uses JSON. The same source text is a valid JavaScript array of
// objects, with the same bytes and the same bracket and brace tokens.
func TestQueryMatchesContainedWithinRange(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		("[" @l_bracket "]" @r_bracket)
		("{" @l_brace "}" @r_brace)
	`)
	source := unindent(`
		[
		    {"key1": "value1"},
		    {"key2": "value2"},
		    {"key3": "value3"},
		    {"key4": "value4"},
		    {"key5": "value5"},
		    {"key6": "value6"},
		    {"key7": "value7"},
		    {"key8": "value8"},
		    {"key9": "value9"},
		    {"key10": "value10"},
		    {"key11": "value11"},
		    {"key12": "value12"},
		]
	`)
	tree := parseSource(t, language, source)
	expected := []matchResult{
		m(1, "l_brace", "{", "r_brace", "}"),
		m(1, "l_brace", "{", "r_brace", "}"),
	}

	cursor := ts.NewQueryCursor()
	cursor.SetContainingPointRange(ts.Point{Row: 5, Column: 0}, ts.Point{Row: 7, Column: 0})
	require.Equal(t, expected, collectMatches(cursor, query, tree.RootNode(), source))

	cursor = ts.NewQueryCursor()
	cursor.SetContainingByteRange(78, 120)
	require.Equal(t, expected, collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_matches_different_queries_same_cursor.
func TestQueryMatchesDifferentQueriesSameCursor(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query1 := mustQuery(t, language, `
		(array (identifier) @id1)
	`)
	query2 := mustQuery(t, language, `
		(array (identifier) @id1)
		(pair (identifier) @id2)
	`)
	query3 := mustQuery(t, language, `
		(array (identifier) @id1)
		(pair (identifier) @id2)
		(parenthesized_expression (identifier) @id3)
	`)
	source := "[a, {b: b}, (c)];"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	require.Equal(t, []matchResult{m(0, "id1", "a")},
		collectMatches(cursor, query1, tree.RootNode(), source))
	require.Equal(t, []matchResult{
		m(0, "id1", "a"),
		m(1, "id2", "b"),
		m(2, "id3", "c"),
	}, collectMatches(cursor, query3, tree.RootNode(), source))
	require.Equal(t, []matchResult{
		m(0, "id1", "a"),
		m(1, "id2", "b"),
	}, collectMatches(cursor, query2, tree.RootNode(), source))
}

// Upstream: test_query_matches_with_multiple_captures_on_a_node.
func TestQueryMatchesWithMultipleCapturesOnANode(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `(function_declaration
		(identifier) @name1 @name2 @name3
		(statement_block) @body1 @body2)`)
	source := "function foo() { return 1; }"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	require.Equal(t, []matchResult{
		m(0,
			"name1", "foo",
			"name2", "foo",
			"name3", "foo",
			"body1", "{ return 1; }",
			"body2", "{ return 1; }"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	// A disabled capture drops out when a node has several captures.
	query.DisableCapture("name2")
	require.Equal(t, []matchResult{
		m(0,
			"name1", "foo",
			"name3", "foo",
			"body1", "{ return 1; }",
			"body2", "{ return 1; }"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// captureKindRow is one capture as its name, its node kind and its start row.
type captureKindRow struct {
	Name string
	Kind string
	Row  uint32
}

// Upstream: test_query_matches_with_captured_wildcard_at_root. Upstream uses
// Python. Here JavaScript gives the same shapes. This covers a captured
// wildcard root over catch and finally clauses, and a captured wildcard root
// inside an alternation over else clauses. JavaScript has no elif or
// while-else, so an `else if` clause takes the place of elif. The
// expectation comes from the pinned CLI.
func TestQueryMatchesWithCapturedWildcardAtRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		; captured wildcard at the root
		(_ [
			(catch_clause (statement_block) @block)
			(finally_clause (statement_block) @block)
		]) @stmt

		[
			(while_statement (statement_block) @block)
			(if_statement (statement_block) @block)

			; captured wildcard at the root within an alternation
			(_ [
				(else_clause (statement_block) @block)
				(else_clause (if_statement) @block)
			])

			(try_statement (statement_block) @block)
			(for_in_statement (statement_block) @block)
		] @stmt
	`)
	source := strings.TrimSpace(unindent(`
		for (i of j) {
		    while (true) {
		        if (a) {
		            print(b)
		        } else if (c) {
		            print(d)
		        } else {
		            try {
		                print(f)
		            } catch {
		                print(g)
		            } finally {
		                print(h)
		            }
		        }
		    }
		}
	`))
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	got := [][]captureKindRow{}
	for match := range cursor.Matches(query, tree.RootNode(), []byte(source)) {
		captures := []captureKindRow{}
		for _, capture := range match.Captures {
			captures = append(captures, captureKindRow{
				Name: query.CaptureNameForID(capture.Index),
				Kind: capture.Node.Type(),
				Row:  capture.Node.StartPoint().Row,
			})
		}
		got = append(got, captures)
	}
	require.Equal(t, [][]captureKindRow{
		{{"stmt", "for_in_statement", 0}, {"block", "statement_block", 0}},
		{{"stmt", "while_statement", 1}, {"block", "statement_block", 1}},
		{{"stmt", "if_statement", 2}, {"block", "statement_block", 2}},
		{{"stmt", "if_statement", 2}, {"block", "if_statement", 4}},
		{{"stmt", "if_statement", 4}, {"block", "statement_block", 4}},
		{{"stmt", "if_statement", 4}, {"block", "statement_block", 6}},
		{{"stmt", "try_statement", 7}, {"block", "statement_block", 7}},
		{{"stmt", "try_statement", 7}, {"block", "statement_block", 9}},
		{{"stmt", "try_statement", 7}, {"block", "statement_block", 11}},
	}, got)
}

// Upstream: test_query_matches_with_no_captures.
func TestQueryMatchesWithNoCaptures(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(identifier)
		(string) @s
	`)
	assertQueryMatches(t, language, query, `
		a = 'hi';
		b = 'bye';
	`, []matchResult{
		m(0),
		m(1, "s", "'hi'"),
		m(0),
		m(1, "s", "'bye'"),
	})
}

// Upstream: test_query_matches_with_repeated_fields.
func TestQueryMatchesWithRepeatedFields(t *testing.T) {
	language := queryLanguage(t, "c")
	query := mustQuery(t, language, "(field_declaration declarator: (field_identifier) @field)")
	assertQueryMatches(t, language, query, `
		struct S {
			int a, b, c;
		};
	`, []matchResult{
		m(0, "field", "a"),
		m(0, "field", "b"),
		m(0, "field", "c"),
	})
}

// Upstream: test_query_matches_with_deeply_nested_patterns_with_fields.
// Upstream uses Python. JavaScript calls have the same function and
// arguments fields, and member_expression takes the place of attribute. A
// Python keyword argument has no direct analog, so the third pattern reads a
// pair inside an object argument. The expectations are upstream's.
func TestQueryMatchesWithDeeplyNestedPatternsWithFields(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(call_expression
			function: (_) @func
			arguments: (_) @args)
		(call_expression
			function: (member_expression
				object: (_) @receiver
				property: (property_identifier) @method)
			arguments: (arguments))

		; These don't match anything, but they require additional
		; states to keep track of their captures.
		(call_expression
			function: (_) @fn
			arguments: (arguments
				(object
					(pair
						key: (property_identifier) @name
						value: (_) @val) @arg)) @args) @call
		(call_expression
			function: (identifier) @fn
			(#eq? @fn "super")) @super_call
	`)
	assertQueryMatches(t, language, query, `
		a(1).b(2).c(3).d(4).e(5).f(6).g(7).h(8)
	`, []matchResult{
		m(0, "func", "a", "args", "(1)"),
		m(0, "func", "a(1).b", "args", "(2)"),
		m(1, "receiver", "a(1)", "method", "b"),
		m(0, "func", "a(1).b(2).c", "args", "(3)"),
		m(1, "receiver", "a(1).b(2)", "method", "c"),
		m(0, "func", "a(1).b(2).c(3).d", "args", "(4)"),
		m(1, "receiver", "a(1).b(2).c(3)", "method", "d"),
		m(0, "func", "a(1).b(2).c(3).d(4).e", "args", "(5)"),
		m(1, "receiver", "a(1).b(2).c(3).d(4)", "method", "e"),
		m(0, "func", "a(1).b(2).c(3).d(4).e(5).f", "args", "(6)"),
		m(1, "receiver", "a(1).b(2).c(3).d(4).e(5)", "method", "f"),
		m(0, "func", "a(1).b(2).c(3).d(4).e(5).f(6).g", "args", "(7)"),
		m(1, "receiver", "a(1).b(2).c(3).d(4).e(5).f(6)", "method", "g"),
		m(0, "func", "a(1).b(2).c(3).d(4).e(5).f(6).g(7).h", "args", "(8)"),
		m(1, "receiver", "a(1).b(2).c(3).d(4).e(5).f(6).g(7)", "method", "h"),
	})
}

const includeSource = "#include <foo>\n#include <bar>\n#include <baz>\n\n// comment"

// Upstream: test_query_alternation_with_inner_quantifier.
func TestQueryAlternationWithInnerQuantifier(t *testing.T) {
	language := queryLanguage(t, "c")
	expected := []matchResult{
		m(0,
			"capture", "#include <foo>\n",
			"capture", "#include <bar>\n",
			"capture", "#include <baz>\n"),
		m(0, "capture", "// comment"),
	}

	query := mustQuery(t, language, `[
	   (preproc_include)+
	   (comment)
	] @capture`)
	assertQueryMatches(t, language, query, includeSource, expected)

	query = mustQuery(t, language, `[
	   (comment)
	   (preproc_include)+
	] @capture`)
	assertQueryMatches(t, language, query, includeSource, expected)
}

// Upstream: test_query_alternation_with_outer_quantifier.
func TestQueryAlternationWithOuterQuantifier(t *testing.T) {
	language := queryLanguage(t, "c")
	expected := []matchResult{
		m(0,
			"capture", "#include <foo>\n",
			"capture", "#include <bar>\n",
			"capture", "#include <baz>\n",
			"capture", "// comment"),
	}

	query := mustQuery(t, language, `[
		(preproc_include)
		(comment)
	]+ @capture`)
	assertQueryMatches(t, language, query, includeSource, expected)

	query = mustQuery(t, language, `([
		(preproc_include)
		(comment)
	] (_)?)+ @capture`)
	assertQueryMatches(t, language, query, includeSource, expected)
}

// Upstream: test_query_matches_with_alternations_and_predicates.
// Upstream uses Java. Here JavaScript gives the same shape: a block with an
// alternation of declarations and an alternation of argument lists, with
// #eq? predicates across them. The pattern matches structurally, and the
// predicates on the capture that is absent reject it.
func TestQueryMatchesWithAlternationsAndPredicates(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(statement_block
			[
				(lexical_declaration
					(variable_declarator
						(identifier) @def.a
						(string) @lit.a
					)
				)
				(lexical_declaration
					(variable_declarator
						(identifier) @def.b
						(null) @lit.b
					)
				)
			]
			(expression_statement
				(call_expression [
					(arguments
						(identifier) @ref.a
						(string)
					)
					(arguments
						(null)
						(identifier) @ref.b
					)
				])
			)
			(#eq? @def.a @ref.a )
			(#eq? @def.b @ref.b )
		)
	`)
	assertQueryMatches(t, language, query, `
		function test() {
			let a = "foo";
			f(null, b);
		}
	`, []matchResult{})
}

// Upstream: test_query_matches_with_indefinite_step_containing_no_captures.
func TestQueryMatchesWithIndefiniteStepContainingNoCaptures(t *testing.T) {
	// The pattern depends on the field declarations in the body but captures none of them.
	language := queryLanguage(t, "c")
	query := mustQuery(t, language, `(struct_specifier
		name: (type_identifier) @name
		body: (field_declaration_list
			(field_declaration
				type: (union_specifier))))`)
	assertQueryMatches(t, language, query, `
		struct LacksUnionField {
			int a;
			struct {
				B c;
			} d;
			G *h;
		};

		struct HasUnionField {
			int a;
			struct {
				B c;
			} d;
			union {
				bool e;
				float f;
			} g;
			G *h;
		};
	`, []matchResult{m(0, "name", "HasUnionField")})
}

// Upstream: test_query_captures_basic.
func TestQueryCapturesBasic(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(pair
		  key: _ @method.def
		  (function_expression
			name: (identifier) @method.alias))

		(variable_declarator
		  name: _ @function.def
		  value: (function_expression
			name: (identifier) @function.alias))

		":" @delimiter
		"=" @operator
	`)
	source := `
          a({
            bc: function de() {
              const fg = function hi() {}
            },
            jk: function lm() {
              const no = function pq() {}
            },
          });
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	require.Equal(t, []matchResult{
		m(2, "delimiter", ":"),
		m(0, "method.def", "bc", "method.alias", "de"),
		m(3, "operator", "="),
		m(1, "function.def", "fg", "function.alias", "hi"),
		m(2, "delimiter", ":"),
		m(0, "method.def", "jk", "method.alias", "lm"),
		m(3, "operator", "="),
		m(1, "function.def", "no", "function.alias", "pq"),
	}, collectMatches(cursor, query, tree.RootNode(), source))

	require.Equal(t, []capturePair{
		c("method.def", "bc"),
		c("delimiter", ":"),
		c("method.alias", "de"),
		c("function.def", "fg"),
		c("operator", "="),
		c("function.alias", "hi"),
		c("method.def", "jk"),
		c("delimiter", ":"),
		c("method.alias", "lm"),
		c("function.def", "no"),
		c("operator", "="),
		c("function.alias", "pq"),
	}, collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_with_text_conditions.
func TestQueryCapturesWithTextConditions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		((identifier) @constant
		 (#match? @constant "^[A-Z]{2,}$"))

		 ((identifier) @constructor
		  (#match? @constructor "^[A-Z]"))

		((identifier) @function.builtin
		 (#eq? @function.builtin "require"))

		((identifier) @variable.builtin
		  (#any-of? @variable.builtin
					"arguments"
					"module"
					"console"
					"window"
					"document"))

		((identifier) @variable
		 (#not-match? @variable "^(lambda|load)$"))
	`)
	source := `
          toad
          load
          panda
          lambda
          const ab = require('./ab');
          new Cd(EF);
          document;
          module;
          console;
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []capturePair{
		c("variable", "toad"),
		c("variable", "panda"),
		c("variable", "ab"),
		c("function.builtin", "require"),
		c("variable", "require"),
		c("constructor", "Cd"),
		c("variable", "Cd"),
		c("constant", "EF"),
		c("constructor", "EF"),
		c("variable", "EF"),
		c("variable.builtin", "document"),
		c("variable", "document"),
		c("variable.builtin", "module"),
		c("variable", "module"),
		c("variable.builtin", "console"),
		c("variable", "console"),
	}, collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_with_predicates.
func TestQueryCapturesWithPredicates(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		((call_expression (identifier) @foo)
		 (#set! name something)
		 (#set! cool)
		 (#something! @foo omg))

		((property_identifier) @bar
		 (#is? cool)
		 (#is-not? name something))`)

	require.Equal(t, []ts.QueryProperty{
		{Key: "name", Value: "something", HasValue: true, CaptureIndex: -1},
		{Key: "cool", CaptureIndex: -1},
	}, query.PropertySettings(0))
	require.Equal(t, []ts.QueryPredicate{{
		Operator: "something!",
		Args: []ts.QueryPredicateArg{
			{Kind: ts.QueryPredicateArgCapture, CaptureIndex: 0},
			{Kind: ts.QueryPredicateArgString, Value: "omg"},
		},
	}}, query.GeneralPredicates(0))
	require.Empty(t, query.PropertySettings(1))
	require.Empty(t, query.PropertyPredicates(0))
	require.Equal(t, []ts.PropertyPredicate{
		{Property: ts.QueryProperty{Key: "cool", CaptureIndex: -1}, Positive: true},
		{Property: ts.QueryProperty{Key: "name", Value: "something", HasValue: true, CaptureIndex: -1}, Positive: false},
	}, query.PropertyPredicates(1))

	source := "const a = window.b"
	tree := parseSource(t, language, source)
	query = mustQuery(t, language, `((identifier) @variable.builtin
			(#match? @variable.builtin "^(arguments|module|console|window|document)$")
			(#is-not? local))
	`)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []matchResult{m(0, "variable.builtin", "window")},
		collectMatches(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_with_quoted_predicate_args.
func TestQueryCapturesWithQuotedPredicateArgs(t *testing.T) {
	// A double-quoted string can hold escapes like \n and \r, escaped double quotes, and escaped backslashes.
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		((call_expression (identifier) @foo)
		 (#set! one "\"something\ngreat\""))

		((identifier)
		 (#set! two "\\s(\r?\n)*$"))

		((function_declaration)
		 (#set! three "\"something\ngreat\""))
	`)
	require.Equal(t, []ts.QueryProperty{
		{Key: "one", Value: "\"something\ngreat\"", HasValue: true, CaptureIndex: -1},
	}, query.PropertySettings(0))
	require.Equal(t, []ts.QueryProperty{
		{Key: "two", Value: "\\s(\r?\n)*$", HasValue: true, CaptureIndex: -1},
	}, query.PropertySettings(1))
	require.Equal(t, []ts.QueryProperty{
		{Key: "three", Value: "\"something\ngreat\"", HasValue: true, CaptureIndex: -1},
	}, query.PropertySettings(2))
}

// Upstream: test_query_captures_with_duplicates.
func TestQueryCapturesWithDuplicates(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(variable_declarator
			name: (identifier) @function
			value: (function_expression))

		(identifier) @variable
	`)
	source := `
          var x = function() {};
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []capturePair{
		c("function", "x"),
		c("variable", "x"),
	}, collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_with_many_nested_results_without_fields.
func TestQueryCapturesWithManyNestedResultsWithoutFields(t *testing.T) {
	language := queryLanguage(t, "javascript")

	// Key-value pairs whose values are anonymous functions.
	query := mustQuery(t, language, `
		(pair
		  key: _ @method-def
		  (arrow_function))

		":" @colon
		"," @comma
	`)

	// The pair for key `y` does not match, but its value holds many pairs that do.
	methodCount := 50
	var sb strings.Builder
	sb.WriteString("x = { y: {\n")
	for i := range methodCount {
		fmt.Fprintf(&sb, "    method%d: $ => null,\n", i)
	}
	sb.WriteString("}};\n")
	source := sb.String()

	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	captures := collectCaptures(cursor, query, tree.RootNode(), source)
	require.GreaterOrEqual(t, len(captures), 13)
	require.Equal(t, []capturePair{
		c("colon", ":"),
		c("method-def", "method0"),
		c("colon", ":"),
		c("comma", ","),
		c("method-def", "method1"),
		c("colon", ":"),
		c("comma", ","),
		c("method-def", "method2"),
		c("colon", ":"),
		c("comma", ","),
		c("method-def", "method3"),
		c("colon", ":"),
		c("comma", ","),
	}, captures[0:13])

	// No match drops because of the buffer.
	require.Len(t, captures, 1+3*methodCount)
}

// Upstream: test_query_captures_with_many_nested_results_with_fields.
func TestQueryCapturesWithManyNestedResultsWithFields(t *testing.T) {
	language := queryLanguage(t, "javascript")

	// Expressions like `a ? a.b : null`.
	query := mustQuery(t, language, `
		((ternary_expression
			condition: (identifier) @left
			consequence: (member_expression
				object: (identifier) @right)
			alternative: (null))
		 (#eq? @left @right))
	`)

	// The outer expression does not match, but its consequence is an object that holds many matches.
	count := 50
	var sb strings.Builder
	sb.WriteString("a ? {")
	for i := range count {
		fmt.Fprintf(&sb, "  x: y%d ? y%d.z : null,\n", i, i)
	}
	sb.WriteString("} : null;\n")
	source := sb.String()

	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	captures := collectCaptures(cursor, query, tree.RootNode(), source)
	expected := []capturePair{}
	for i := range 10 {
		expected = append(expected, c("left", fmt.Sprintf("y%d", i)), c("right", fmt.Sprintf("y%d", i)))
	}
	require.GreaterOrEqual(t, len(captures), 20)
	require.Equal(t, expected, captures[0:20])

	// No match drops because of the buffer.
	require.Len(t, captures, 2*count)
}

// Upstream: test_query_captures_with_too_many_nested_results.
func TestQueryCapturesWithTooManyNestedResults(t *testing.T) {
	language := queryLanguage(t, "javascript")

	// The second pattern captures the root call_expression and depends on
	// its last child. Matches that finish between those points must stay
	// in a buffer, so captures come back in order. When the buffer is
	// full, the outer match is abandoned.
	query := mustQuery(t, language, `
		;; easy 👇
		(call_expression
		  function: (member_expression
			property: (property_identifier) @method-name))

		;; hard 👇
		(call_expression
		  function: (member_expression
			property: (property_identifier) @template-tag)
		  arguments: (template_string)) @template-call
	`)
	source := strings.TrimSpace("\n" +
		"        a(b => {\n" +
		"            b.c0().d0 `😄`;\n" +
		"            b.c1().d1 `😄`;\n" +
		"            b.c2().d2 `😄`;\n" +
		"            b.c3().d3 `😄`;\n" +
		"            b.c4().d4 `😄`;\n" +
		"            b.c5().d5 `😄`;\n" +
		"            b.c6().d6 `😄`;\n" +
		"            b.c7().d7 `😄`;\n" +
		"            b.c8().d8 `😄`;\n" +
		"            b.c9().d9 `😄`;\n" +
		"        }).e().f ``;\n" +
		"        ")

	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	cursor.SetMatchLimit(32)
	captures := collectCaptures(cursor, query, tree.RootNode(), source)
	require.GreaterOrEqual(t, len(captures), 40)
	require.Equal(t, []capturePair{
		c("template-call", "b.c0().d0 `😄`"),
		c("method-name", "c0"),
		c("method-name", "d0"),
		c("template-tag", "d0"),
	}, captures[0:4])
	require.Equal(t, []capturePair{
		c("template-call", "b.c9().d9 `😄`"),
		c("method-name", "c9"),
		c("method-name", "d9"),
		c("template-tag", "d9"),
	}, captures[36:40])
	require.Equal(t, []capturePair{
		c("method-name", "e"),
		c("method-name", "f"),
	}, captures[40:])
}

// Upstream: test_query_captures_with_definite_pattern_containing_many_nested_matches.
func TestQueryCapturesWithDefinitePatternContainingManyNestedMatches(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(array
		  "[" @l-bracket
		  "]" @r-bracket)

		"." @dot
	`)

	// The '[' comes back before every '.', although its pattern finishes
	// only at the ']'. The '[' is definite, so it can come back early.
	source := `
        [
            a.b.c.d.e.f.g.h.i,
            a.b.c.d.e.f.g.h.i,
            a.b.c.d.e.f.g.h.i,
            a.b.c.d.e.f.g.h.i,
            a.b.c.d.e.f.g.h.i,
        ]
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	expected := []capturePair{c("l-bracket", "[")}
	for range 40 {
		expected = append(expected, c("dot", "."))
	}
	expected = append(expected, c("r-bracket", "]"))
	require.Equal(t, expected, collectCaptures(cursor, query, tree.RootNode(), source))
}

// Upstream: test_query_captures_ordered_by_both_start_and_end_positions.
func TestQueryCapturesOrderedByBothStartAndEndPositions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(call_expression) @call
		(member_expression) @member
		(identifier) @variable
	`)
	source := `
          a.b(c.d().e).f;
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []capturePair{
		c("member", "a.b(c.d().e).f"),
		c("call", "a.b(c.d().e)"),
		c("member", "a.b"),
		c("variable", "a"),
		c("member", "c.d().e"),
		c("call", "c.d()"),
		c("member", "c.d"),
		c("variable", "c"),
	}, collectCaptures(cursor, query, tree.RootNode(), source))
}

// capturedTextsRemovingOn drains the captures and removes the match of any
// capture whose text is the given text.
func capturedTextsRemovingOn(cursor *ts.QueryCursor, query *ts.Query, tree *ts.Tree, source, remove string) []string {
	texts := []string{}
	for match, index := range cursor.Captures(query, tree.RootNode(), []byte(source)) {
		text := nodeText(match.Captures[index].Node, source)
		if text == remove {
			cursor.RemoveMatch(match.ID)
			continue
		}
		texts = append(texts, text)
	}
	return texts
}

// Upstream: test_query_captures_with_matches_removed.
func TestQueryCapturesWithMatchesRemoved(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(binary_expression
			left: (identifier) @left
			operator: _ @op
			right: (identifier) @right)
	`)
	source := `
          a === b && c > d && e < f;
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []string{"c", ">", "d", "e", "<", "f"},
		capturedTextsRemovingOn(cursor, query, tree, source, "a"))
}

// Upstream: test_query_captures_with_matches_removed_before_they_finish.
func TestQueryCapturesWithMatchesRemovedBeforeTheyFinish(t *testing.T) {
	// A namespace_import always has "*", "as" and an identifier.
	language := queryLanguage(t, "javascript")
	query := mustQuery(t, language, `
		(namespace_import
		  "*" @star
		  "as" @as
		  (identifier) @identifier)
	`)
	source := `
          import * as name from 'module-name';
        `
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	require.Equal(t, []string{"*"},
		capturedTextsRemovingOn(cursor, query, tree, source, "as"))
}
