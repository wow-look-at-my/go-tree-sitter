package corpus

import (
	"os"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// BenchmarkDecode times what a grammar package pays at init: reading its table
// blob and turning it back into a language.
func BenchmarkDecode(b *testing.B, path string) {
	b.Helper()
	blob, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("reading the table blob: %v", err)
	}
	b.SetBytes(int64(len(blob)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ts.DecodeTables(blob); err != nil {
			b.Fatalf("decoding the table blob: %v", err)
		}
	}
}
