package treesitter_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/bash"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/clang"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/cpp"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/javascript"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/rust"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/tsx"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/typescript"
)

// These helpers port crates/cli/src/tests/helpers/query_helpers.rs
// (tree-sitter v0.27.2) for the upstream query test suite.

// capturePair is one formatted capture: its name and its text.
type capturePair struct {
	Name string
	Text string
}

// matchResult is one formatted match: its pattern index and its captures.
type matchResult struct {
	Pattern  int
	Captures []capturePair
}

// m builds a matchResult from a pattern index and name, text pairs.
func m(pattern int, pairs ...string) matchResult {
	if len(pairs)%2 != 0 {
		panic("m: the pairs come as name, text")
	}
	result := matchResult{Pattern: pattern, Captures: []capturePair{}}
	for i := 0; i < len(pairs); i += 2 {
		result.Captures = append(result.Captures, capturePair{Name: pairs[i], Text: pairs[i+1]})
	}
	return result
}

// c builds a capturePair.
func c(name, text string) capturePair { return capturePair{Name: name, Text: text} }

// queryLanguage names a grammar the way the upstream fixtures do.
func queryLanguage(t testing.TB, name string) *ts.Language {
	t.Helper()
	switch name {
	case "javascript":
		return javascript.Language()
	case "c":
		return clang.Language()
	case "cpp":
		return cpp.Language()
	case "rust":
		return rust.Language()
	case "go":
		return golang.Language()
	case "bash":
		return bash.Language()
	case "typescript":
		return typescript.Language()
	case "tsx":
		return tsx.Language()
	}
	t.Fatalf("no grammar named %q is bundled", name)
	return nil
}

// parseSource parses a document with a fresh parser.
func parseSource(t testing.TB, language *ts.Language, source string) *ts.Tree {
	t.Helper()
	parser := ts.NewParser()
	require.True(t, parser.SetLanguage(language), "the parser refused the language")
	tree := parser.ParseString(nil, []byte(source))
	require.NotNil(t, tree, "the parser returned no tree")
	return tree
}

// nodeText is Node::utf8_text over the whole document.
func nodeText(node ts.Node, source string) string {
	return source[node.StartByte():node.EndByte()]
}

// formatCaptures is format_captures.
func formatCaptures(captures []ts.QueryCapture, query *ts.Query, source string) []capturePair {
	result := []capturePair{}
	for _, capture := range captures {
		result = append(result, capturePair{
			Name: query.CaptureNameForID(capture.Index),
			Text: nodeText(capture.Node, source),
		})
	}
	return result
}

// collectMatches is collect_matches: it drains the cursor's matches of the
// query over the node and formats each.
func collectMatches(cursor *ts.QueryCursor, query *ts.Query, node ts.Node, source string) []matchResult {
	result := []matchResult{}
	for match := range cursor.Matches(query, node, []byte(source)) {
		result = append(result, matchResult{
			Pattern:  int(match.PatternIndex),
			Captures: formatCaptures(match.Captures, query, source),
		})
	}
	return result
}

// collectCaptures is collect_captures: it drains the cursor's captures of
// the query over the node and formats each.
func collectCaptures(cursor *ts.QueryCursor, query *ts.Query, node ts.Node, source string) []capturePair {
	result := []capturePair{}
	for match, index := range cursor.Captures(query, node, []byte(source)) {
		capture := match.Captures[index]
		result = append(result, capturePair{
			Name: query.CaptureNameForID(capture.Index),
			Text: nodeText(capture.Node, source),
		})
	}
	return result
}

// assertQueryMatches is assert_query_matches.
func assertQueryMatches(t testing.TB, language *ts.Language, query *ts.Query, source string, expected []matchResult) {
	t.Helper()
	tree := parseSource(t, language, source)
	cursor := ts.NewQueryCursor()
	got := collectMatches(cursor, query, tree.RootNode(), source)
	require.Equal(t, expected, got)
	require.False(t, cursor.DidExceedMatchLimit())
}

// queryErrorOf requires that compiling the query fails and returns the error.
func queryErrorOf(t testing.TB, language *ts.Language, source string) *ts.QueryError {
	t.Helper()
	_, err := ts.NewQuery(language, source)
	require.Error(t, err, "the query compiled: %s", source)
	var qerr *ts.QueryError
	require.ErrorAs(t, err, &qerr)
	return qerr
}

// unindent is the unindent crate: it strips the common leading whitespace
// of the lines, and the first and last line when they are blank.
func unindent(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		n := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	if indent < 0 {
		indent = 0
	}
	for i, line := range lines {
		if len(line) >= indent {
			lines[i] = line[indent:]
		} else {
			lines[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// upstreamQueryTestCount is the number of `fn test_` functions in crates/cli/src/tests/query_test.rs at v0.27.2.
const upstreamQueryTestCount = 131

// upstreamQueryTestsPorted and upstreamQueryTestsSkipped are filled by the
// files that port the suite.
var (
	upstreamQueryTestsPorted  []string
	upstreamQueryTestsSkipped = map[string]string{}
)

// The suite is accounted for: every upstream test is ported or skipped with
// a reason, and none is both.
func TestUpstreamQuerySuiteIsAccountedFor(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range upstreamQueryTestsPorted {
		require.False(t, seen[name], "ported twice: %s", name)
		seen[name] = true
	}
	for name, reason := range upstreamQueryTestsSkipped {
		require.False(t, seen[name], "ported and skipped: %s", name)
		require.NotEmpty(t, reason, "skipped without a reason: %s", name)
		seen[name] = true
	}
	require.Equal(t, upstreamQueryTestCount, len(seen),
		"upstream has %d tests; ported %d, skipped %d",
		upstreamQueryTestCount, len(upstreamQueryTestsPorted), len(upstreamQueryTestsSkipped))
	fmt.Printf("upstream query tests: %d ported, %d skipped\n",
		len(upstreamQueryTestsPorted), len(upstreamQueryTestsSkipped))
}
