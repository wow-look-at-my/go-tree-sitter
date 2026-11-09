package querygoldens

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

// It runs the CLI that TS_GOLDENS_CLI names, or tree-sitter from PATH.
func TestRecord(t *testing.T) {
	if os.Getenv("TS_GOLDENS_RECORD") != "1" {
		t.Skip("set TS_GOLDENS_RECORD=1 to record the query goldens")
	}
	cli := os.Getenv("TS_GOLDENS_CLI")
	if cli == "" {
		cli = "tree-sitter"
	}
	recorder := Recorder{CLI: cli, Root: "../..", Log: os.Stderr}
	for _, g := range Grammars {
		require.NoError(t, recorder.Record(g), g.Name)
	}
}

// The CLI cannot load a language outside the ABI range, so the Language kind
// is checked here against the Rust binding's text.
func TestLanguageErrorKind(t *testing.T) {
	language := *golang.Language()
	language.ABIVersion = 99
	_, err := ts.NewQuery(&language, "(identifier) @x")
	var qerr *ts.QueryError
	require.True(t, errors.As(err, &qerr))
	assert.Equal(t, ts.QueryErrorLanguage, qerr.Kind)
	assert.Equal(t, 0, qerr.Offset)
	assert.Equal(t, 0, qerr.Row)
	assert.Equal(t, 0, qerr.Column)
	assert.Equal(t, "Incompatible language version 99. Expected minimum 13, maximum 15", qerr.Error())
}

func TestCLIError(t *testing.T) {
	stderr := "\x1b[33mWarning:\x1b[0m You have not configured any parser directories!\n\n" +
		"\x1b[31mError:\x1b[0m Query compilation failed\n\nCaused by:\n" +
		"    Query error at 2:3. Impossible pattern:\n      (package_clause))\n      ^\n"
	text, err := CLIError(stderr)
	require.NoError(t, err)
	assert.Equal(t, "Query error at 2:3. Impossible pattern:\n  (package_clause))\n  ^\n", text)

	_, err = CLIError("Error: something else\n")
	assert.Error(t, err)
	_, err = CLIError("Error: Query compilation failed\n")
	assert.Error(t, err)
}

func TestParseCompileGolden(t *testing.T) {
	source := "(identifier)\r\n\r\n  (bogus_node)\r\n"
	e, err := ParseCompileGolden("Query error at 3:6. Invalid node type \"bogus_node\"\n", source)
	require.NoError(t, err)
	assert.Equal(t, ts.QueryErrorNodeType, e.Kind)
	assert.Equal(t, 2, e.Row)
	assert.Equal(t, 5, e.Column)
	assert.Equal(t, 19, e.Offset)
	assert.Equal(t, "bogus_node", source[e.Offset:e.Offset+10])

	e, err = ParseCompileGolden(Accepted, source)
	require.NoError(t, err)
	assert.True(t, e.Accepted)
	_, err = ParseCompileGolden("garbage\n", source)
	assert.Error(t, err)
}

func TestOffsetAtEOF(t *testing.T) {
	assert.Equal(t, 2, offsetOf("a\n", 1, 0))
	assert.Equal(t, 2, offsetOf("a", 1, 0))
	assert.Equal(t, 1, offsetOf("ab", 0, 1))
}

func TestCheckCompile(t *testing.T) {
	assert.NoError(t, CheckCompile(Expected{Accepted: true}, nil))
	assert.Error(t, CheckCompile(Expected{Accepted: true}, errors.New("x")))
	want := Expected{Kind: ts.QueryErrorField, Row: 1, Column: 2, Offset: 5, Text: "t"}
	assert.Error(t, CheckCompile(want, nil))
	assert.Error(t, CheckCompile(want, errors.New("not a query error")))
	err := CheckCompile(want, &ts.QueryError{Kind: ts.QueryErrorSyntax})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind Syntax, want Field")
}

func TestSplit(t *testing.T) {
	out := []byte("a\n  pattern: 0\nb\nc\n    pattern:  1\n")
	parts, err := Split(out, []string{"a", "b", "c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a\n  pattern: 0\n", "b\n", "c\n    pattern:  1\n"},
		[]string{string(parts[0]), string(parts[1]), string(parts[2])})

	_, err = Split(out, []string{"b"})
	assert.Error(t, err)
	_, err = Split(out, []string{"a", "x"})
	assert.Error(t, err)
	_, err = Split(out, []string{"a"})
	assert.Error(t, err)
}

func TestFirstDifference(t *testing.T) {
	want := []byte("s\n  pattern: 1\nt\n  pattern: 2\n")
	got := []byte("s\n  pattern: 1\nt\n  pattern: 3\n")
	assert.Equal(t, "line 4 (section t)\n want: \"  pattern: 2\\n\"\n  got: \"  pattern: 3\\n\"",
		FirstDifference(want, got))
	assert.Equal(t, "no difference", FirstDifference(want, want))
}

func TestFindRejectsAnUnknownGrammar(t *testing.T) {
	_, err := Find("cobol")
	assert.Error(t, err)
}

// An input directory that is missing fails, as a checkout without
// submodules must.
func TestMissingSubmoduleFails(t *testing.T) {
	g := Grammar{Name: "go", Queries: []string{"nowhere"}, Corpus: "nowhere"}
	_, err := Queries(g, t.TempDir())
	assert.Error(t, err)
	_, err = Inputs(g, t.TempDir())
	assert.Error(t, err)
	g.Examples = "nowhere"
	_, err = Inputs(g, t.TempDir())
	assert.Error(t, err)
}

func TestRenderFormats(t *testing.T) {
	language := golang.Language()
	query, err := ts.NewQuery(language, "(function_declaration name: (identifier) @name) @fn")
	require.NoError(t, err)
	sections := []Section{{Name: "a.go", Source: []byte("package p\n\nfunc a() {\n}\n")}}
	out, exceeded, err := Render(language, query, sections, Matches)
	require.NoError(t, err)
	assert.False(t, exceeded)
	assert.Equal(t, "a.go\n  pattern: 0\n    capture: fn, start: (2, 0), end: (3, 1)\n"+
		"    capture: 1 - name, start: (2, 5), end: (2, 6), text: `a`\n", string(out))

	out, _, err = Render(language, query, sections, Captures)
	require.NoError(t, err)
	assert.Equal(t, "a.go\n"+
		"    pattern:  0, capture: 0 - fn, start: (2, 0), end: (3, 1), text: `func a() {\n}`\n"+
		"    pattern:  0, capture: 1 - name, start: (2, 5), end: (2, 6), text: `a`\n", string(out))
}
