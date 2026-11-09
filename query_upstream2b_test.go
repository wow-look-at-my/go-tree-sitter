package treesitter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

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
