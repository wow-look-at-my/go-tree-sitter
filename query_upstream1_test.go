package treesitter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// This file ports the first part of crates/cli/src/tests/query_test.rs
// (tree-sitter v0.27.2). That file is from
// test_query_errors_on_invalid_syntax through
// test_query_matches_with_repeated_internal_nodes.

func init() {
	upstreamQueryTestsPorted = append(upstreamQueryTestsPorted,
		"test_query_errors_on_invalid_syntax",
		"test_query_errors_on_anchor_at_group_edge",
		"test_query_errors_on_invalid_symbols",
		"test_query_errors_on_invalid_predicates",
		"test_query_errors_on_impossible_patterns",
		"test_query_verifies_possible_patterns_with_aliased_parent_nodes",
		"test_query_matches_with_simple_pattern",
		"test_query_matches_with_multiple_on_same_root",
		"test_query_matches_with_multiple_patterns_different_roots",
		"test_query_matches_with_multiple_patterns_same_root",
		"test_query_matches_with_nesting_and_no_fields",
		"test_query_matches_with_many_results",
		"test_query_matches_with_many_overlapping_results",
		"test_query_matches_capturing_error_nodes",
		"test_query_matches_capturing_missing_nodes",
		"test_query_matches_with_extra_children",
		"test_query_matches_with_named_wildcard",
		"test_query_matches_with_wildcard_at_the_root",
		"test_query_matches_with_wildcard_within_wildcard",
		"test_query_matches_with_immediate_siblings",
		"test_query_matches_with_anchor_after_zero_quantifier",
		"test_query_matches_with_anchor_after_nested_zero_quantifier",
		"test_query_matches_with_last_child_anchor_after_optional",
		"test_query_matches_with_anchors_on_both_sides_of_zero_quantifier",
		"test_query_matches_with_leading_anchor_before_zero_quantifier",
		"test_query_matches_with_last_named_child",
		"test_query_matches_with_negated_fields",
		"test_query_matches_with_field_at_root",
		"test_query_matches_with_repeated_leaf_nodes",
		"test_query_matches_optional_capture_before_uncaptured_required_sibling",
		"test_query_matches_with_optional_nodes_inside_of_repetitions",
		"test_query_matches_with_top_level_repetitions",
		"test_query_matches_with_non_terminal_repetitions_within_root",
		"test_query_matches_with_nested_repetitions",
		"test_query_matches_with_multiple_repetition_patterns_that_intersect_other_pattern",
		"test_query_matches_with_trailing_repetitions_of_last_child",
		"test_query_matches_with_leading_zero_or_more_repeated_leaf_nodes",
		"test_matches_with_anchor_sibling_inside_parent",
		"test_matches_with_anchor_sibling_with_quantifier_inside_parent",
		"test_matches_with_anchor_sibling_with_quantifier_captured_inside_parent",
		"test_matches_anchored_quantified_sibling_inside_parent",
		"test_query_matches_with_trailing_optional_nodes",
		"test_query_matches_with_nested_optional_nodes",
		"test_query_matches_with_repeated_internal_nodes",
	)
}

// p1Lines joins the lines of an expected error message.
func p1Lines(lines ...string) string { return strings.Join(lines, "\n") }

// p1QueryErrorMessage requires that the query fails and returns the message.
func p1QueryErrorMessage(t *testing.T, language *ts.Language, source string) string {
	t.Helper()
	return queryErrorOf(t, language, source).Message
}

// p1RequireQueryError requires that the query fails with exactly this error.
func p1RequireQueryError(t *testing.T, language *ts.Language, source string, want ts.QueryError) {
	t.Helper()
	require.Equal(t, want, *queryErrorOf(t, language, source))
}

// p1MustQuery compiles a query that must compile.
func p1MustQuery(t *testing.T, language *ts.Language, source string) *ts.Query {
	t.Helper()
	query, err := ts.NewQuery(language, source)
	require.NoError(t, err)
	return query
}

// test_query_errors_on_invalid_syntax. The python case is rewritten against
// javascript: an unclosed tree inside an alternation.
func TestQueryErrorsOnInvalidSyntax(t *testing.T) {
	language := queryLanguage(t, "javascript")

	p1MustQuery(t, language, "(if_statement)")
	p1MustQuery(t, language, "(if_statement condition:(parenthesized_expression (identifier)))")

	require.Equal(t, p1Lines(
		"(if_statement",
		"             ^",
	), p1QueryErrorMessage(t, language, "(if_statement"))
	require.Equal(t, p1Lines(
		"  (if_statement))",
		"                ^",
	), p1QueryErrorMessage(t, language, "; comment 1\n; comment 2\n  (if_statement))"))

	require.Equal(t, p1Lines(
		"(if_statement identifier)",
		"              ^",
	), p1QueryErrorMessage(t, language, "(if_statement identifier)"))
	require.Equal(t, p1Lines(
		"(if_statement condition:)",
		"                        ^",
	), p1QueryErrorMessage(t, language, "(if_statement condition:)"))

	require.Equal(t, p1Lines(
		`(identifier) "h `,
		`             ^`,
	), p1QueryErrorMessage(t, language, `(identifier) "h `))

	require.Equal(t, p1Lines(
		"((identifier) ()",
		"               ^",
	), p1QueryErrorMessage(t, language, "((identifier) ()"))

	require.Equal(t, p1Lines(
		"((identifier) [])",
		"               ^",
	), p1QueryErrorMessage(t, language, "((identifier) [])"))

	require.Equal(t, p1Lines(
		"((identifier) (#a?)",
		"                   ^",
	), p1QueryErrorMessage(t, language, "((identifier) (#a?)"))

	require.Equal(t, p1Lines(
		"((identifier) (#a))",
		"                 ^",
	), p1QueryErrorMessage(t, language, "((identifier) (#a))"))

	require.Equal(t, p1Lines(
		"((identifier) @x (#eq? @x a",
		"                           ^",
	), p1QueryErrorMessage(t, language, "((identifier) @x (#eq? @x a"))

	require.Equal(t, p1Lines(
		"(statement_block .)",
		"                  ^",
	), p1QueryErrorMessage(t, language, "(statement_block .)"))

	require.Equal(t, p1Lines(
		"(statement_block ! (if_statement))",
		"                   ^",
	), p1QueryErrorMessage(t, language, "(statement_block ! (if_statement))"))

	c := queryLanguage(t, "c")
	require.Equal(t, p1Lines(
		`(parameter_list [ ")" @foo)`,
		`                          ^`,
	), p1QueryErrorMessage(t, c, `(parameter_list [ ")" @foo)`))

	require.Equal(t, p1Lines(
		"[(unary_expression (_) @operand) (binary_expression (_) @operand]",
		"                                                                ^",
	), p1QueryErrorMessage(t, language, "[(unary_expression (_) @operand) (binary_expression (_) @operand]"))

	require.Equal(t, p1Lines(
		"(MISSING (function_declarator (identifier))) ",
		"         ^",
	), p1QueryErrorMessage(t, c, "(MISSING (function_declarator (identifier))) "))

	require.Equal(t, p1Lines(
		"(MISSING function_declarator function_declarator) ",
		"                             ^",
	), p1QueryErrorMessage(t, c, "(MISSING function_declarator function_declarator) "))

	p1RequireQueryError(t, language, "(statement / export_statement)", ts.QueryError{
		Row: 0, Offset: 11, Column: 11, Kind: ts.QueryErrorSyntax,
		Message: p1Lines(
			"(statement / export_statement)",
			"           ^",
		),
	})
}

// test_query_errors_on_anchor_at_group_edge
func TestQueryErrorsOnAnchorAtGroupEdge(t *testing.T) {
	language := queryLanguage(t, "javascript")

	p1MustQuery(t, language, "((_) . (_))")
	p1MustQuery(t, language, "(program (_) (_) .)")
	p1MustQuery(t, language, "(program (_)* @x . (_))")

	p1RequireQueryError(t, language, "((_) .)", ts.QueryError{
		Row: 0, Offset: 5, Column: 5, Kind: ts.QueryErrorSyntax,
		Message: p1Lines(
			"((_) .)",
			"     ^",
		),
	})
	p1RequireQueryError(t, language, "(program ((_)+ .)? (_))", ts.QueryError{
		Row: 0, Offset: 15, Column: 15, Kind: ts.QueryErrorSyntax,
		Message: p1Lines(
			"(program ((_)+ .)? (_))",
			"               ^",
		),
	})
}

// test_query_errors_on_invalid_symbols
func TestQueryErrorsOnInvalidSymbols(t *testing.T) {
	language := queryLanguage(t, "javascript")

	cases := []struct {
		query string
		want  ts.QueryError
	}{
		{`">>>>"`, ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `">>>>"`}},
		{`"te\"st"`, ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `"te\"st"`}},
		{`"\\" @cap`, ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `"\\"`}},
		{"(clas)", ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `"clas"`}},
		{"(if_statement (arrayyyyy))", ts.QueryError{Offset: 15, Column: 15, Kind: ts.QueryErrorNodeType, Message: `"arrayyyyy"`}},
		{"(if_statement condition: (non_existent3))", ts.QueryError{Offset: 26, Column: 26, Kind: ts.QueryErrorNodeType, Message: `"non_existent3"`}},
		{"(if_statement condit: (identifier))", ts.QueryError{Offset: 14, Column: 14, Kind: ts.QueryErrorField, Message: `"condit"`}},
		{"(if_statement conditioning: (identifier))", ts.QueryError{Offset: 14, Column: 14, Kind: ts.QueryErrorField, Message: `"conditioning"`}},
		{"(if_statement !alternativ)", ts.QueryError{Offset: 15, Column: 15, Kind: ts.QueryErrorField, Message: `"alternativ"`}},
		{"(if_statement !alternatives)", ts.QueryError{Offset: 15, Column: 15, Kind: ts.QueryErrorField, Message: `"alternatives"`}},
		{"fakefield: (identifier)", ts.QueryError{Offset: 0, Column: 0, Kind: ts.QueryErrorField, Message: `"fakefield"`}},
		{"(ERR)", ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `"ERR"`}},
		{"(MISS)", ts.QueryError{Offset: 1, Column: 1, Kind: ts.QueryErrorNodeType, Message: `"MISS"`}},
	}
	for _, tc := range cases {
		p1RequireQueryError(t, language, tc.query, tc.want)
	}
}

// test_query_errors_on_invalid_predicates
func TestQueryErrorsOnInvalidPredicates(t *testing.T) {
	language := queryLanguage(t, "javascript")

	p1RequireQueryError(t, language, "((identifier) @id (@id))", ts.QueryError{
		Kind: ts.QueryErrorSyntax, Row: 0, Column: 19, Offset: 19,
		Message: p1Lines(
			"((identifier) @id (@id))",
			"                   ^",
		),
	})
	p1RequireQueryError(t, language, "((identifier) @id (#eq? @id))", ts.QueryError{
		Kind: ts.QueryErrorPredicate, Row: 0, Column: 0, Offset: 0,
		Message: "Wrong number of arguments to #eq? predicate. Expected 2, got 1.",
	})
	p1RequireQueryError(t, language, "((identifier) @id (#eq? @id @ok))", ts.QueryError{
		Kind: ts.QueryErrorCapture, Row: 0, Column: 29, Offset: 29,
		Message: `"ok"`,
	})
}

// test_query_errors_on_impossible_patterns. The ruby case (a field whose
// grammar never admits the child type) is rewritten against javascript.
func TestQueryErrorsOnImpossiblePatterns(t *testing.T) {
	js := queryLanguage(t, "javascript")

	p1RequireQueryError(t, js,
		"(binary_expression left: (expression (identifier)) left: (expression (identifier)))",
		ts.QueryError{
			Kind: ts.QueryErrorStructure, Row: 0, Offset: 37, Column: 37,
			Message: p1Lines(
				"(binary_expression left: (expression (identifier)) left: (expression (identifier)))",
				"                                     ^",
			),
		})

	p1MustQuery(t, js, "(function_declaration name: (identifier) (statement_block))")
	p1RequireQueryError(t, js, "(function_declaration name: (statement_block))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 22, Column: 22,
		Message: p1Lines(
			"(function_declaration name: (statement_block))",
			"                      ^",
		),
	})

	p1MustQuery(t, js, "(new_expression constructor:(member_expression))")
	p1RequireQueryError(t, js, "(new_expression constructor:(binary_expression))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 16, Column: 16,
		Message: p1Lines(
			"(new_expression constructor:(binary_expression))",
			"                ^",
		),
	})

	p1MustQuery(t, js, "[\n"+
		"                (function_expression (identifier))\n"+
		"                (function_declaration (identifier))\n"+
		"                (generator_function_declaration (identifier))\n"+
		"            ]")
	p1RequireQueryError(t, js, "[\n"+
		"                    (function_expression (identifier))\n"+
		"                    (function_declaration (object))\n"+
		"                    (generator_function_declaration (identifier))\n"+
		"                ]",
		ts.QueryError{
			Kind: ts.QueryErrorStructure, Row: 2, Offset: 99, Column: 42,
			Message: p1Lines(
				"                    (function_declaration (object))",
				"                                          ^",
			),
		})

	p1RequireQueryError(t, js, "(identifier (identifier))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 12, Column: 12,
		Message: p1Lines(
			"(identifier (identifier))",
			"            ^",
		),
	})
	p1RequireQueryError(t, js, "(true (true))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 6, Column: 6,
		Message: p1Lines(
			"(true (true))",
			"      ^",
		),
	})

	p1MustQuery(t, js, "(if_statement\n"+
		"                condition: (parenthesized_expression (expression) @cond))")

	p1RequireQueryError(t, js, "(if_statement condition: (expression))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 14, Column: 14,
		Message: p1Lines(
			"(if_statement condition: (expression))",
			"              ^",
		),
	})
	p1RequireQueryError(t, js, "(identifier/identifier)", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 0, Column: 0,
		Message: p1Lines(
			"(identifier/identifier)",
			"^",
		),
	})

	if js.ABIVersion >= 15 {
		p1RequireQueryError(t, js, "(statement/identifier)", ts.QueryError{
			Kind: ts.QueryErrorStructure, Row: 0, Offset: 0, Column: 0,
			Message: p1Lines(
				"(statement/identifier)",
				"^",
			),
		})
		p1RequireQueryError(t, js, "(statement/pattern)", ts.QueryError{
			Kind: ts.QueryErrorStructure, Row: 0, Offset: 0, Column: 0,
			Message: p1Lines(
				"(statement/pattern)",
				"^",
			),
		})
	}
}

// test_query_verifies_possible_patterns_with_aliased_parent_nodes. Rewritten
// against c: pointer_declarator holds a field_identifier only through the
// alias of pointer_field_declarator, as ruby's destructured_parameter does.
func TestQueryVerifiesPossiblePatternsWithAliasedParentNodes(t *testing.T) {
	language := queryLanguage(t, "c")

	p1MustQuery(t, language, "(pointer_declarator (field_identifier))")

	p1RequireQueryError(t, language, "(pointer_declarator (string_literal))", ts.QueryError{
		Kind: ts.QueryErrorStructure, Row: 0, Offset: 20, Column: 20,
		Message: p1Lines(
			"(pointer_declarator (string_literal))",
			"                    ^",
		),
	})
}

// test_query_matches_with_simple_pattern
func TestQueryMatchesWithSimplePattern(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, "(function_declaration name: (identifier) @fn-name)")

	assertQueryMatches(t, language, query,
		"function one() { two(); function three() {} }",
		[]matchResult{
			m(0, "fn-name", "one"),
			m(0, "fn-name", "three"),
		})
}

// test_query_matches_with_multiple_on_same_root
func TestQueryMatchesWithMultipleOnSameRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `(class_declaration
                name: (identifier) @the-class-name
                (class_body
                    (method_definition
                        name: (property_identifier) @the-method-name)))`)

	assertQueryMatches(t, language, query, `
            class Person {
                // the constructor
                constructor(name) { this.name = name; }

                // the getter
                getFullName() { return this.name; }
            }
            `,
		[]matchResult{
			m(0, "the-class-name", "Person", "the-method-name", "constructor"),
			m(0, "the-class-name", "Person", "the-method-name", "getFullName"),
		})
}

// test_query_matches_with_multiple_patterns_different_roots
func TestQueryMatchesWithMultiplePatternsDifferentRoots(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
                (function_declaration name:(identifier) @fn-def)
                (call_expression function:(identifier) @fn-ref)
            `)

	assertQueryMatches(t, language, query, `
            function f1() {
                f2(f3());
            }
            `,
		[]matchResult{
			m(0, "fn-def", "f1"),
			m(1, "fn-ref", "f2"),
			m(1, "fn-ref", "f3"),
		})
}

// test_query_matches_with_multiple_patterns_same_root
func TestQueryMatchesWithMultiplePatternsSameRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
              (pair
                key: (property_identifier) @method-def
                value: (function_expression))

              (pair
                key: (property_identifier) @method-def
                value: (arrow_function))
            `)

	assertQueryMatches(t, language, query, `
            a = {
                b: () => { return c; },
                d: function() { return d; }
            };
            `,
		[]matchResult{
			m(1, "method-def", "b"),
			m(0, "method-def", "d"),
		})
}

// test_query_matches_with_nesting_and_no_fields
func TestQueryMatchesWithNestingAndNoFields(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
                (array
                    (array
                        (identifier) @x1
                        (identifier) @x2))
            `)

	assertQueryMatches(t, language, query, `
            [[a]];
            [[c, d], [e, f, g, h]];
            [[h], [i]];
            `,
		[]matchResult{
			m(0, "x1", "c", "x2", "d"),
			m(0, "x1", "e", "x2", "f"),
			m(0, "x1", "e", "x2", "g"),
			m(0, "x1", "f", "x2", "g"),
			m(0, "x1", "e", "x2", "h"),
			m(0, "x1", "f", "x2", "h"),
			m(0, "x1", "g", "x2", "h"),
		})
}

// test_query_matches_with_many_results
func TestQueryMatchesWithManyResults(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, "(array (identifier) @element)")

	expected := []matchResult{}
	for range 50 {
		expected = append(expected, m(0, "element", "hello"))
	}
	assertQueryMatches(t, language, query, strings.Repeat("[hello];\n", 50), expected)
}

// test_query_matches_with_many_overlapping_results
func TestQueryMatchesWithManyOverlappingResults(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (call_expression
                function: (member_expression
                    property: (property_identifier) @method))
            (call_expression
                function: (identifier) @function)
            ((identifier) @constant
             (#match? @constant "[A-Z\\d_]+"))
            `)

	const count = 1024
	source := "a" + strings.Repeat("\n  .foo(bar(BAZ))", count)

	expected := []matchResult{}
	for range count {
		expected = append(expected,
			m(0, "method", "foo"),
			m(1, "function", "bar"),
			m(2, "constant", "BAZ"),
		)
	}
	assertQueryMatches(t, language, query, source, expected)
}

// test_query_matches_capturing_error_nodes
func TestQueryMatchesCapturingErrorNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (ERROR (identifier) @the-error-identifier) @the-error
            `)

	assertQueryMatches(t, language, query, "function a(b,, c, d :e:) {}",
		[]matchResult{m(0, "the-error", ":e:", "the-error-identifier", "e")})
}

// test_query_matches_capturing_missing_nodes
func TestQueryMatchesCapturingMissingNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (MISSING
              ; Comments should be valid
            ) @missing
            (MISSING
              ; Comments should be valid
              ";"
              ; Comments should be valid
              ) @missing-semicolon
            `)

	assertQueryMatches(t, language, query, `
            x = function(a) { b; } function(c) { d; }
            //                    ^ MISSING semicolon here
            `,
		[]matchResult{
			m(0, "missing", ""),
			m(1, "missing-semicolon", ""),
		})

	language = queryLanguage(t, "c")
	query = p1MustQuery(t, language, `(MISSING field_identifier) @missing-field-ident
            (MISSING identifier) @missing-ident
            (MISSING) @missing-anything`)

	assertQueryMatches(t, language, query, `
            int main() {
              if (a.) {
              //    ^ MISSING field_identifier here
                b();
                c();

                if (*) d();
                //   ^ MISSING identifier here
              }
            }
            `,
		[]matchResult{
			m(0, "missing-field-ident", ""),
			m(2, "missing-anything", ""),
			m(1, "missing-ident", ""),
			m(2, "missing-anything", ""),
		})
}

// test_query_matches_with_extra_children. Rewritten against javascript: an
// extra (a comment) is a child of the node it sits in. A top-level pattern
// does not see a comment inside an argument list.
func TestQueryMatchesWithExtraChildren(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (program(comment) @top_level_comment)
            (arguments (comment) @comment_in_args)
            `)

	assertQueryMatches(t, language, query, `
            // top-level
            puts(
                // not-top-level
                a, bar.baz
            )

            puts(no)
            `,
		[]matchResult{
			m(0, "top_level_comment", "// top-level"),
			m(1, "comment_in_args", "// not-top-level"),
		})
}

// test_query_matches_with_last_named_child
func TestQueryMatchesWithLastNamedChild(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p1MustQuery(t, language, `(compound_statement
                (_)
                (_)
                (expression_statement
                    (identifier) @last_id) .)`)

	assertQueryMatches(t, language, query, `
            void one() { a; b; c; }
            void two() { d; e; }
            void three() { f; g; h; i; }
            `,
		[]matchResult{m(0, "last_id", "c"), m(0, "last_id", "i")})
}

// test_query_matches_with_negated_fields
func TestQueryMatchesWithNegatedFields(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (import_specifier
                !alias
                name: (identifier) @import_name)

            (export_specifier
                !alias
                name: (identifier) @export_name)

            (export_statement
                !decorator
                !source
                (_) @exported)

            ; This negated field list is an extension of a previous
            ; negated field list. The order of the children and negated
            ; fields doesn't matter.
            (export_statement
                !decorator
                !source
                (_) @exported_expr
                !declaration)

            ; This negated field list is a prefix of a previous
            ; negated field list.
            (export_statement
                !decorator
                (_) @export_child .)
            `)

	assertQueryMatches(t, language, query, `
            import {a as b, c} from 'p1';
            export {g, h as i} from 'p2';

            @foo
            export default 1;

            export var j = 1;

            export default k;
            `,
		[]matchResult{
			m(0, "import_name", "c"),
			m(1, "export_name", "g"),
			m(4, "export_child", "'p2'"),
			m(2, "exported", "var j = 1;"),
			m(4, "export_child", "var j = 1;"),
			m(2, "exported", "k"),
			m(3, "exported_expr", "k"),
			m(4, "export_child", "k"),
		})
}

// test_query_matches_with_field_at_root
func TestQueryMatchesWithFieldAtRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, "name: (identifier) @name")

	assertQueryMatches(t, language, query, `
            a();
            function b() {}
            class c extends d {}
            `,
		[]matchResult{m(0, "name", "b"), m(0, "name", "c")})
}

// test_query_matches_with_repeated_leaf_nodes
func TestQueryMatchesWithRepeatedLeafNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (
                (comment)+ @doc
                .
                (class_declaration
                    name: (identifier) @name)
            )

            (
                (comment)+ @doc
                .
                (function_declaration
                    name: (identifier) @name)
            )
            `)

	assertQueryMatches(t, language, query, `
            // one
            // two
            a();

            // three
            {
                // four
                // five
                // six
                class B {}

                // seven
                c();

                // eight
                function d() {}
            }
            `,
		[]matchResult{
			m(0, "doc", "// four", "doc", "// five", "doc", "// six", "name", "B"),
			m(1, "doc", "// eight", "name", "d"),
		})
}

// test_query_matches_optional_capture_before_uncaptured_required_sibling
func TestQueryMatchesOptionalCaptureBeforeUncapturedRequiredSibling(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := p1MustQuery(t, language, "(block (line_comment)? @doc (line_comment))")

	assertQueryMatches(t, language, query, `
            fn f() {
                // a
                // b
            }
            `,
		[]matchResult{m(0, "doc", "// a")})
}

// test_query_matches_with_optional_nodes_inside_of_repetitions
func TestQueryMatchesWithOptionalNodesInsideOfRepetitions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `(array (","? (number) @num)+)`)

	assertQueryMatches(t, language, query, `
            var a = [1, 2, 3, 4]
            `,
		[]matchResult{m(0, "num", "1", "num", "2", "num", "3", "num", "4")})
}

// test_query_matches_with_top_level_repetitions
func TestQueryMatchesWithTopLevelRepetitions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (comment)+ @doc
            `)

	assertQueryMatches(t, language, query, `
            // a
            // b
            // c

            d()

            // e
            `,
		[]matchResult{
			m(0, "doc", "// a", "doc", "// b", "doc", "// c"),
			m(0, "doc", "// e"),
		})
}

// test_query_matches_with_non_terminal_repetitions_within_root
func TestQueryMatchesWithNonTerminalRepetitionsWithinRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, "(_ (expression_statement (identifier) @id)+)")

	assertQueryMatches(t, language, query, `
            function f() {
                d;
                e;
                f;
                g;
            }
            a;
            b;
            c;
            `,
		[]matchResult{
			m(0, "id", "d", "id", "e", "id", "f", "id", "g"),
			m(0, "id", "a", "id", "b", "id", "c"),
		})
}

// test_query_matches_with_nested_repetitions
func TestQueryMatchesWithNestedRepetitions(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (variable_declaration
                (","? (variable_declarator name: (identifier) @x))+)+
            `)

	assertQueryMatches(t, language, query, `
            var a = b, c, d
            var e, f

            // more
            var g
            `,
		[]matchResult{
			m(0, "x", "a", "x", "c", "x", "d", "x", "e", "x", "f"),
			m(0, "x", "g"),
		})
}

// test_query_matches_with_multiple_repetition_patterns_that_intersect_other_pattern
func TestQueryMatchesWithMultipleRepetitionPatternsThatIntersectOtherPattern(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (call_expression
                function: (member_expression
                    property: (property_identifier) @name)) @ref.method

            ((comment)* @doc (function_declaration))
            ((comment)* @doc (generator_function_declaration))
            ((comment)* @doc (class_declaration))
            ((comment)* @doc (lexical_declaration))
            ((comment)* @doc (variable_declaration))
            ((comment)* @doc (method_definition))

            (comment) @comment
            `)

	source := "theObject\n" + strings.Repeat("  // the comment\n", 64) + "\n.theMethod()"

	expected := []matchResult{}
	for range 64 {
		expected = append(expected, m(7, "comment", "// the comment"))
	}
	expected = append(expected, m(0, "ref.method", source, "name", "theMethod"))
	assertQueryMatches(t, language, query, source, expected)
}

// test_query_matches_with_trailing_repetitions_of_last_child
func TestQueryMatchesWithTrailingRepetitionsOfLastChild(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (unary_expression (primary_expression)+ @operand)
            `)

	assertQueryMatches(t, language, query, `
            a = typeof (!b && ~c);
            `,
		[]matchResult{
			m(0, "operand", "b"),
			m(0, "operand", "c"),
			m(0, "operand", "(!b && ~c)"),
		})
}

// test_query_matches_with_leading_zero_or_more_repeated_leaf_nodes
func TestQueryMatchesWithLeadingZeroOrMoreRepeatedLeafNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (
                (comment)* @doc
                .
                (function_declaration
                    name: (identifier) @name)
            )
            `)

	assertQueryMatches(t, language, query, `
            function a() {
                // one
                var b;

                function c() {}

                // two
                // three
                var d;

                // four
                // five
                function e() {

                }
            }

            // six
            `,
		[]matchResult{
			m(0, "name", "a"),
			m(0, "name", "c"),
			m(0, "doc", "// four", "doc", "// five", "name", "e"),
		})
}

// test_matches_with_anchor_sibling_inside_parent
func TestMatchesWithAnchorSiblingInsideParent(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := p1MustQuery(t, language, `
            (source_file
                (line_comment)
                .
                (function_item
                    name: (identifier) @name)
            )`)

	assertQueryMatches(t, language, query, `
            // A
            fn a() {}

            // B
            fn b() {}
            `,
		[]matchResult{m(0, "name", "a"), m(0, "name", "b")})
}

// test_matches_with_anchor_sibling_with_quantifier_inside_parent
func TestMatchesWithAnchorSiblingWithQuantifierInsideParent(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := p1MustQuery(t, language, `
            (source_file
                (line_comment)+
                .
                (function_item
                    name: (identifier) @name)
            )`)

	assertQueryMatches(t, language, query, `
            // A
            fn a() {}

            // B
            fn b() {}
            `,
		[]matchResult{m(0, "name", "a"), m(0, "name", "b")})
}

// test_matches_with_anchor_sibling_with_quantifier_captured_inside_parent
func TestMatchesWithAnchorSiblingWithQuantifierCapturedInsideParent(t *testing.T) {
	language := queryLanguage(t, "rust")
	query := p1MustQuery(t, language, `
            (source_file
                (line_comment)+ @doc
                .
                (function_item
                    name: (identifier) @name)
            )`)

	assertQueryMatches(t, language, query, `
            // A
            fn a() {}

            // B
            fn b() {}
            `,
		[]matchResult{
			m(0, "doc", "// A", "name", "a"),
			m(0, "doc", "// B", "name", "b"),
		})
}

// test_matches_anchored_quantified_sibling_inside_parent
func TestMatchesAnchoredQuantifiedSiblingInsideParent(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p1MustQuery(t, language, "(translation_unit (comment)* @comment . (declaration) @decl)")

	assertQueryMatches(t, language, query, `
void foo() {}

// this one has
// two comments
extern int baz;

// this one has a comment
extern int bar;
`,
		[]matchResult{
			m(0, "comment", "// this one has", "comment", "// two comments", "decl", "extern int baz;"),
			m(0, "comment", "// this one has a comment", "decl", "extern int bar;"),
		})
}

// test_query_matches_with_trailing_optional_nodes
func TestQueryMatchesWithTrailingOptionalNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (class_declaration
                name: (identifier) @class
                (class_heritage
                  (identifier) @superclass)?)
            `)

	assertQueryMatches(t, language, query, "class A {}",
		[]matchResult{m(0, "class", "A")})

	assertQueryMatches(t, language, query, `
            class A {}
            class B extends C {}
            class D extends (E.F) {}
            `,
		[]matchResult{
			m(0, "class", "A"),
			m(0, "class", "B", "superclass", "C"),
			m(0, "class", "D"),
		})
}

// test_query_matches_with_nested_optional_nodes
func TestQueryMatchesWithNestedOptionalNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (call_expression
                function: (identifier) @outer-fn
                arguments: (arguments
                    (call_expression
                        function: (identifier) @inner-fn
                        arguments: (arguments
                            (number)? @num))?))
            `)

	assertQueryMatches(t, language, query, `
            a(b, c(), d(null, 1, 2))
            e()
            f(g())
            `,
		[]matchResult{
			m(0, "outer-fn", "a", "inner-fn", "c"),
			m(0, "outer-fn", "c"),
			m(0, "outer-fn", "a", "inner-fn", "d", "num", "1"),
			m(0, "outer-fn", "a", "inner-fn", "d", "num", "2"),
			m(0, "outer-fn", "d"),
			m(0, "outer-fn", "e"),
			m(0, "outer-fn", "f", "inner-fn", "g"),
			m(0, "outer-fn", "g"),
		})
}

// test_query_matches_with_repeated_internal_nodes
func TestQueryMatchesWithRepeatedInternalNodes(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (_
                (method_definition
                    (decorator (identifier) @deco)+
                    name: (property_identifier) @name))
            `)

	assertQueryMatches(t, language, query, `
            class A {
                @c
                @d
                e() {}
            }
            `,
		[]matchResult{m(0, "deco", "c", "deco", "d", "name", "e")})
}
