package treesitter_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/javascript"
	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// The queries that seed both fuzz targets.
var fuzzSeedQueries = []string{
	"(identifier) @id",
	"(function_declaration name: (identifier) @name) @fn",
	"((identifier) @x (#eq? @x \"a\"))",
	"((identifier) @x (#match? @x \"^[A-Z]\"))",
	"(call_expression function: (identifier) @f arguments: (argument_list (_)* @arg))",
	"[(identifier) (number)] @leaf",
	"(binary_expression left: (_) @l . right: (_) @r)",
	"(block (_) @first . (_)* @rest)",
	"(function_declaration !parameters)",
	"(expression/identifier) @e",
	"(MISSING) @missing",
	"\"func\" @kw",
	"(ERROR) @err",
	"(_) @any",
	"(program (_)+ @items)",
	"((comment)* @c . (function_declaration) @f)",
	"(if_statement condition: (parenthesized_expression (identifier)) @cond)",
	"(statement_block . (expression_statement) @first)",
	"(_ name: (identifier) @n)",
	"(identifier)? @maybe",
	"; just a comment",
	"(",
	"(identifier",
	"(identifier) @",
	"((identifier) @a (#eq? @b \"x\"))",
	"(function_declaration (nonsense))",
	"(function_declaration nonsense: (identifier))",
	"(identifier/identifier)",
	"(program (identifier) . )",
	"(_ . )",
}

// FuzzNewQuery compiles arbitrary text as a query. It must return or report
// an error; it must never panic or loop.
func FuzzNewQuery(f *testing.F) {
	for _, q := range fuzzSeedQueries {
		f.Add(q)
	}
	f.Add("[")
	f.Add("]")
	f.Add("(a/b/c)")
	f.Add("(a (b) @x (#set! k v) (#is? @x k) (#is-not? k))")
	f.Add("(a (#unknown! @x y \"z\"))")
	f.Add("(a \"unterminated)")
	f.Add("(a \"esc\\\"aped\\n\\0\")")
	f.Add("(\xff\xfe)")
	js := javascript.Language()
	golangLanguage := golang.Language()
	f.Fuzz(func(t *testing.T, source string) {
		for _, language := range []*ts.Language{js, golangLanguage} {
			query, err := ts.NewQuery(language, source)
			if err != nil {
				var qerr *ts.QueryError
				require.ErrorAs(t, err, &qerr)
				require.NotEqual(t, ts.QueryErrorNone, qerr.Kind)
				require.LessOrEqual(t, qerr.Offset, len(source))
				continue
			}
			require.NotNil(t, query)
			for i := 0; i < query.PatternCount(); i++ {
				query.IsPatternRooted(i)
				query.IsPatternNonLocal(i)
				query.CaptureQuantifiers(i)
				query.PredicatesForPattern(i)
			}
		}
	})
}

// corpusSeeds reads a bounded number of corpus inputs for a grammar.
func corpusSeeds(f *testing.F, dir string, limit int) [][]byte {
	cases, err := corpus.ReadDir(dir)
	if err != nil {
		f.Fatalf("reading the corpus under %s: %v", dir, err)
	}
	if len(cases) == 0 {
		f.Fatalf("no corpus cases under %s", dir)
	}
	var seeds [][]byte
	for _, c := range cases {
		if c.Expect != corpus.ExpectPass {
			continue
		}
		seeds = append(seeds, c.Input)
		if len(seeds) == limit {
			break
		}
	}
	return seeds
}

// FuzzQueryExec runs an arbitrary query over an arbitrary document. A query
// that compiles must run to the end of its matches and captures without a
// panic, and every capture must lie inside the document.
func FuzzQueryExec(f *testing.F) {
	goSeeds := corpusSeeds(f, filepath.Join("grammars", "golang", "testdata", "tree-sitter-go", "test", "corpus"), 24)
	jsSeeds := corpusSeeds(f, filepath.Join("grammars", "javascript", "testdata", "tree-sitter-javascript", "test", "corpus"), 24)
	sample, err := os.ReadFile(filepath.Join("grammars", "golang", "testdata", "sample.go.txt"))
	require.NoError(f, err)
	for i, q := range fuzzSeedQueries {
		f.Add(true, q, string(goSeeds[i%len(goSeeds)]))
		f.Add(false, q, string(jsSeeds[i%len(jsSeeds)]))
	}
	f.Add(true, "(function_declaration name: (identifier) @name) @fn", string(sample))

	languages := map[bool]*ts.Language{true: golang.Language(), false: javascript.Language()}
	f.Fuzz(func(t *testing.T, useGo bool, querySource, document string) {
		language := languages[useGo]
		query, err := ts.NewQuery(language, querySource)
		if err != nil {
			return
		}
		parser := ts.NewParser()
		require.True(t, parser.SetLanguage(language))
		tree := parser.ParseString(nil, []byte(document))
		require.NotNil(t, tree)
		root := tree.RootNode()

		cursor := ts.NewQueryCursor()
		cursor.SetMatchLimit(64)
		matchCount := 0
		for m := range cursor.Matches(query, root, []byte(document)) {
			matchCount++
			require.Less(t, int(m.PatternIndex), query.PatternCount())
			for _, capture := range m.Captures {
				require.Less(t, int(capture.Index), query.CaptureCount())
				require.LessOrEqual(t, capture.Node.StartByte(), capture.Node.EndByte())
				require.LessOrEqual(t, capture.Node.EndByte(), uint32(len(document)))
			}
		}
		lastStart := uint32(0)
		for m, index := range cursor.Captures(query, root, []byte(document)) {
			capture := m.Captures[index]
			require.GreaterOrEqual(t, capture.Node.StartByte(), lastStart, "captures come in document order")
			lastStart = capture.Node.StartByte()
		}

		// A range restriction never reaches outside the document.
		cursor.SetByteRange(uint32(len(document))/3, 2*uint32(len(document))/3)
		for m := range cursor.Matches(query, root, []byte(document)) {
			for _, capture := range m.Captures {
				require.LessOrEqual(t, capture.Node.EndByte(), uint32(len(document)))
			}
		}
	})
}
