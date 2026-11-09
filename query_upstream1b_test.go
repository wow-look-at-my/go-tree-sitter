package treesitter_test

import _ "embed"

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// This file holds the second half of the port in query_upstream1_test.go.

// test_query_matches_with_named_wildcard
func TestQueryMatchesWithNamedWildcard(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (return_statement (_) @the-return-value)
            (binary_expression operator: _ @the-operator)
            `)

	source := "return a + b - c;"
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()

	require.Equal(t, []matchResult{
		m(0, "the-return-value", "a + b - c"),
		m(1, "the-operator", "+"),
		m(1, "the-operator", "-"),
	}, collectMatches(cursor, query, tree.RootNode(), source))
}

// test_query_matches_with_wildcard_at_the_root
func TestQueryMatchesWithWildcardAtTheRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (_
                (comment) @doc
                .
                (function_declaration
                    name: (identifier) @name))
            `)

	assertQueryMatches(t, language, query,
		"/* one */ var x; /* two */ function y() {} /* three */ class Z {}",
		[]matchResult{m(0, "doc", "/* two */", "name", "y")})

	query = p1MustQuery(t, language, `
                (_ (string) @a)
                (_ (number) @b)
                (_ (true) @c)
                (_ (false) @d)
            `)

	assertQueryMatches(t, language, query, "['hi', x(true), {y: false}]",
		[]matchResult{
			m(0, "a", "'hi'"),
			m(2, "c", "true"),
			m(3, "d", "false"),
		})
}

// test_query_matches_with_wildcard_within_wildcard
func TestQueryMatchesWithWildcardWithinWildcard(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (_ (_) @child) @parent
            `)

	assertQueryMatches(t, language, query, "/* a */ b; c;",
		[]matchResult{
			m(0, "parent", "/* a */ b; c;", "child", "/* a */"),
			m(0, "parent", "/* a */ b; c;", "child", "b;"),
			m(0, "parent", "b;", "child", "b"),
			m(0, "parent", "/* a */ b; c;", "child", "c;"),
			m(0, "parent", "c;", "child", "c"),
		})
}

// test_query_matches_with_immediate_siblings. Rewritten against javascript:
// formal_parameters stands in for python's dotted_name, array for list, and
// statement_block for block. The capture texts follow the javascript source.
func TestQueryMatchesWithImmediateSiblings(t *testing.T) {
	language := queryLanguage(t, "javascript")

	query := p1MustQuery(t, language, `
            (formal_parameters
                (identifier) @parent
                .
                (identifier) @child)
            (formal_parameters
                (identifier) @last-child
                .)
            (array
                .
                (_) @first-element)
            `)

	assertQueryMatches(t, language, query, "function f(a, b, c, d) { return [w, [1, y], z] }",
		[]matchResult{
			m(0, "parent", "a", "child", "b"),
			m(0, "parent", "b", "child", "c"),
			m(0, "parent", "c", "child", "d"),
			m(1, "last-child", "d"),
			m(2, "first-element", "w"),
			m(2, "first-element", "1"),
		})

	query = p1MustQuery(t, language, `
            (statement_block . (_) @first-stmt)
            (statement_block (_) @stmt)
            (statement_block (_) @last-stmt .)
            `)

	assertQueryMatches(t, language, query, `
            if (a) {
                b();
                c();
                if (d()) { e(); f() }
                g();
            }
            `,
		[]matchResult{
			m(0, "first-stmt", "b();"),
			m(1, "stmt", "b();"),
			m(1, "stmt", "c();"),
			m(1, "stmt", "if (d()) { e(); f() }"),
			m(0, "first-stmt", "e();"),
			m(1, "stmt", "e();"),
			m(1, "stmt", "f()"),
			m(2, "last-stmt", "f()"),
			m(1, "stmt", "g();"),
			m(2, "last-stmt", "g();"),
		})
}

// test_query_matches_with_anchor_after_zero_quantifier
func TestQueryMatchesWithAnchorAfterZeroQuantifier(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language,
		"(program (comment)* @doc . (function_declaration name: (identifier) @name))")

	assertQueryMatches(t, language, query, "\nclass X {}\nfunction foo() {}\n",
		[]matchResult{m(0, "name", "foo")})

	assertQueryMatches(t, language, query, "\n// c\nfunction foo() {}\n",
		[]matchResult{m(0, "doc", "// c", "name", "foo")})
}

// test_query_matches_with_anchor_after_nested_zero_quantifier
func TestQueryMatchesWithAnchorAfterNestedZeroQuantifier(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, `
            (_
              (field_definition
                property: (_) @name
                value: (_)? @value
              ) @field
              .
              ";" @semicolon
            )
            `)

	assertQueryMatches(t, language, query, "class Foo { bar; baz = 0; }",
		[]matchResult{
			m(0, "field", "bar", "name", "bar", "semicolon", ";"),
			m(0, "field", "baz = 0", "name", "baz", "value", "0", "semicolon", ";"),
		})
}

// test_query_matches_with_last_child_anchor_after_optional
func TestQueryMatchesWithLastChildAnchorAfterOptional(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p1MustQuery(t, language,
		"(preproc_if (preproc_def)+ @def . (preproc_else)? @else .)")

	assertQueryMatches(t, language, query, "\n#if X\n#define A\n// c\n#endif\n",
		[]matchResult{})

	assertQueryMatches(t, language, query, "\n#if X\n#define A\n#endif\n",
		[]matchResult{m(0, "def", "#define A\n")})
}

// test_query_matches_with_anchors_on_both_sides_of_zero_quantifier
func TestQueryMatchesWithAnchorsOnBothSidesOfZeroQuantifier(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language,
		"(program (lexical_declaration) @a . (comment)* . (function_declaration) @b)")

	assertQueryMatches(t, language, query,
		testQueryMatchesWithAnchorsOnBothSidesOfZeroQuantifierJs,
		[]matchResult{m(0, "a", "const b = 2;", "b", "function foo() {}")})

	assertQueryMatches(t, language, query,
		testQueryMatchesWithAnchorsOnBothSidesOfZeroQuantifierJs2,
		[]matchResult{m(0, "a", "const b = 2;", "b", "function foo() {}")})
}

// test_query_matches_with_leading_anchor_before_zero_quantifier
func TestQueryMatchesWithLeadingAnchorBeforeZeroQuantifier(t *testing.T) {
	language := queryLanguage(t, "c")
	query := p1MustQuery(t, language,
		"(translation_unit . (comment)* (function_definition) @f)")

	assertQueryMatches(t, language, query, "\nint main() {}\n",
		[]matchResult{m(0, "f", "int main() {}")})

	assertQueryMatches(t, language, query, "\nint a;\nint main() {}\n",
		[]matchResult{})

	assertQueryMatches(t, language, query, "\n// c\nint main() {}\n",
		[]matchResult{m(0, "f", "int main() {}")})
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

	assertQueryMatches(t, language, query, testQueryMatchesWithNegatedFieldsJs,
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

	assertQueryMatches(t, language, query, testQueryMatchesOptionalCaptureBeforeUncapturedRequiredSiblingRs,
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

	assertQueryMatches(t, language, query, testQueryMatchesWithTopLevelRepetitionsSh,
		[]matchResult{
			m(0, "doc", "// a", "doc", "// b", "doc", "// c"),
			m(0, "doc", "// e"),
		})
}

// test_query_matches_with_non_terminal_repetitions_within_root
func TestQueryMatchesWithNonTerminalRepetitionsWithinRoot(t *testing.T) {
	language := queryLanguage(t, "javascript")
	query := p1MustQuery(t, language, "(_ (expression_statement (identifier) @id)+)")

	assertQueryMatches(t, language, query, testQueryMatchesWithNonTerminalRepetitionsWithinRootCpp,
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

	assertQueryMatches(t, language, query, testQueryMatchesWithLeadingZeroOrMoreRepeatedLeafNodesJs,
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

	assertQueryMatches(t, language, query, testMatchesWithAnchorSiblingInsideParentRs,
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

	assertQueryMatches(t, language, query, testMatchesWithAnchorSiblingWithQuantifierInsideParentRs,
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

	assertQueryMatches(t, language, query, testMatchesWithAnchorSiblingWithQuantifierCapturedInsideParentRs,
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

//go:embed testdata/test-query-matches-with-anchors-on-both-sides-of-zero-quantifier.js
var testQueryMatchesWithAnchorsOnBothSidesOfZeroQuantifierJs string

//go:embed testdata/test-query-matches-with-anchors-on-both-sides-of-zero-quantifier-2.js
var testQueryMatchesWithAnchorsOnBothSidesOfZeroQuantifierJs2 string

//go:embed testdata/test-query-matches-with-negated-fields.js
var testQueryMatchesWithNegatedFieldsJs string

//go:embed testdata/test-query-matches-optional-capture-before-uncaptured-required-sibling.rs
var testQueryMatchesOptionalCaptureBeforeUncapturedRequiredSiblingRs string

//go:embed testdata/test-query-matches-with-top-level-repetitions.sh
var testQueryMatchesWithTopLevelRepetitionsSh string

//go:embed testdata/test-query-matches-with-non-terminal-repetitions-within-root.cpp
var testQueryMatchesWithNonTerminalRepetitionsWithinRootCpp string

//go:embed testdata/test-query-matches-with-leading-zero-or-more-repeated-leaf-nodes.js
var testQueryMatchesWithLeadingZeroOrMoreRepeatedLeafNodesJs string

//go:embed testdata/test-matches-with-anchor-sibling-inside-parent.rs
var testMatchesWithAnchorSiblingInsideParentRs string

//go:embed testdata/test-matches-with-anchor-sibling-with-quantifier-inside-parent.rs
var testMatchesWithAnchorSiblingWithQuantifierInsideParentRs string

//go:embed testdata/test-matches-with-anchor-sibling-with-quantifier-captured-inside-parent.rs
var testMatchesWithAnchorSiblingWithQuantifierCapturedInsideParentRs string
