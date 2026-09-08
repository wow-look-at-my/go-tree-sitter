package rust_test

import (
	"fmt"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/rust"
)

func TestProbe(t *testing.T) {
	for _, src := range []string{
		"// Comment\n",
		"/// Outer\n",
		"//! Inner\n",
		"/* block */\n",
		"fn f() { // c\n }\n",
	} {
		parser := ts.NewParser()
		parser.SetLanguage(rust.Language())
		tree := parser.ParseString(nil, []byte(src))
		fmt.Printf("PROBE %q -> %s\n", src, tree.RootNode().String())
	}
}
