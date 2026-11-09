package treesitter_test

import _ "embed"

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

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
	source := testQueryCapturesBasicJs
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

//go:embed testdata/test-query-captures-basic.js
var testQueryCapturesBasicJs string
