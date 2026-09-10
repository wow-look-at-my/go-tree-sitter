package corpus

import (
	"os"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// BenchmarkParse times a whole parse of a source file. The lexer is the hot
// half of that, and it runs as an interpreted program, so this is the number to
// watch when the instruction set changes.
func BenchmarkParse(b *testing.B, language *ts.Language, path string) {
	b.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("reading the source file: %v", err)
	}
	parser := ts.NewParser()
	parser.SetLanguage(language)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if tree := parser.ParseString(nil, src); tree == nil {
			b.Fatal("the parse produced no tree")
		}
	}
}
