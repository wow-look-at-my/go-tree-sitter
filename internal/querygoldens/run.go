// run.go holds Stats.

package querygoldens

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Stats counts the outcome of a golden run.
type Stats struct {
	Compared int
	Passed   int
	Failed   int
}

// load reads the queries and the inputs of a grammar from the current
// directory, which a grammar package test runs in.
func load(t *testing.T, name string) (Grammar, []Query, []Input) {
	t.Helper()
	g, err := Find(name)
	if err != nil {
		t.Fatal(err)
	}
	queries, err := Queries(g, ".")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := Inputs(g, ".")
	if err != nil {
		t.Fatal(err)
	}
	return g, queries, inputs
}

// compile compiles one query and checks it against its compile golden.
func compile(t *testing.T, language *ts.Language, q Query) (*ts.Query, bool) {
	t.Helper()
	source, err := os.ReadFile(q.Path)
	if err != nil {
		t.Errorf("%s: %v", q.Stem, err)
		return nil, false
	}
	golden, err := os.ReadFile(filepath.Join(GoldensDir, q.Stem, "compile.golden"))
	if err != nil {
		t.Errorf("%s: no compile golden (run ts-goldens query): %v", q.Stem, err)
		return nil, false
	}
	expected, err := ParseCompileGolden(string(golden), string(source))
	if err != nil {
		t.Errorf("%s: %v", q.Stem, err)
		return nil, false
	}
	query, err := ts.NewQuery(language, string(source))
	if err := CheckCompile(expected, err); err != nil {
		t.Errorf("%s: %v", q.Stem, err)
		return nil, false
	}
	return query, expected.Accepted
}

// RunCompile compiles every query of a grammar and checks that Go accepts
// exactly the queries the CLI accepted, with the same error otherwise.
func RunCompile(t *testing.T, language *ts.Language, name string) {
	t.Helper()
	_, queries, _ := load(t, name)
	accepted := 0
	for _, q := range queries {
		if _, ok := compile(t, language, q); ok {
			accepted++
		}
	}
	fmt.Fprintf(os.Stderr, "query compile parity (%s): %d queries, %d accepted by the CLI\n",
		name, len(queries), accepted)
}

// Run renders every query over every input and compares the result with its
// golden byte for byte.
func Run(t *testing.T, language *ts.Language, name string) Stats {
	t.Helper()
	_, queries, inputs := load(t, name)
	var stats Stats
	want := map[string]bool{}
	for _, q := range queries {
		want[filepath.Join(GoldensDir, q.Stem, "compile.golden")] = true
		query, ok := compile(t, language, q)
		if !ok {
			continue
		}
		for _, mode := range Modes {
			for _, in := range inputs {
				path := filepath.Join(GoldensDir, q.Stem, string(mode), filepath.FromSlash(in.Golden)+".golden")
				want[path] = true
				stats.Compared++
				if compare(t, language, query, in, mode, path) {
					stats.Passed++
				} else {
					stats.Failed++
				}
			}
		}
	}
	checkStray(t, want)
	fmt.Fprintf(os.Stderr, "query goldens (%s): %d files compared, %d pass, %d fail\n",
		name, stats.Compared, stats.Passed, stats.Failed)
	if stats.Compared == 0 {
		t.Fatalf("no query goldens compared for %s", name)
	}
	return stats
}

func compare(t *testing.T, language *ts.Language, query *ts.Query, in Input, mode Mode, path string) bool {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("missing golden (run ts-goldens query): %v", err)
		return false
	}
	got, _, err := Render(language, query, in.Sections, mode)
	if err != nil {
		t.Errorf("%s: %v", path, err)
		return false
	}
	if bytes.Equal(got, want) {
		return true
	}
	t.Errorf("%s differs from the CLI\n%s", path, FirstDifference(want, got))
	return false
}

// FirstDifference describes the first line where got leaves want, with the
// section name it falls in.
func FirstDifference(want, got []byte) string {
	wl := strings.SplitAfter(string(want), "\n")
	gl := strings.SplitAfter(string(got), "\n")
	section := ""
	for i := 0; ; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g || i >= len(wl) && i >= len(gl) {
			if w == g {
				return "no difference"
			}
			return fmt.Sprintf("line %d (section %s)\n want: %q\n  got: %q", i+1, section, w, g)
		}
		if !strings.HasPrefix(w, " ") {
			section = strings.TrimSuffix(w, "\n")
		}
	}
}

// checkStray fails on a golden file no query and input names, which a stale
// recording leaves behind.
func checkStray(t *testing.T, want map[string]bool) {
	t.Helper()
	var stray []string
	err := filepath.WalkDir(GoldensDir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && !want[p] {
			stray = append(stray, p)
		}
		return nil
	})
	if err != nil {
		t.Errorf("walking %s: %v", GoldensDir, err)
	}
	sort.Strings(stray)
	for _, p := range stray {
		t.Errorf("stray golden %s: no query and input produce it", p)
	}
}
