// Package goldens compares parse results against files produced by the
// reference tree-sitter command line tool.
package goldens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Normalize collapses runs of whitespace so that the pretty printed form the
// reference tool writes can be compared with a flat expression.
func Normalize(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// Run parses every source file in dir and compares each tree with its golden.
func Run(t *testing.T, language *ts.Language, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	found := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, ".golden") {
			continue
		}
		found++
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read source: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(dir, name+".golden"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}

			parser := ts.NewParser()
			if !parser.SetLanguage(language) {
				t.Fatal("the parser refused the language")
			}
			tree := parser.ParseString(nil, source)
			if tree == nil {
				t.Fatal("the parser returned no tree")
			}

			got := Normalize(tree.String())
			if got != Normalize(string(want)) {
				t.Errorf("tree does not match the golden file\n got: %s\nwant: %s",
					got, Normalize(string(want)))
			}
			if tree.RootNode().HasError() {
				t.Errorf("the tree contains an error node")
			}
		})
	}
	if found == 0 {
		t.Fatalf("no source files under %s", dir)
	}
}
