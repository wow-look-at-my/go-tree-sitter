package rust_test

import (
	"fmt"
	"os"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/rust"
)

func TestProbe(t *testing.T) {
	out := ""
	for _, src := range []string{
		"// c\n",
		"// c\nuse x;\n",
		"use x;\n// c\n",
		"fn f() {}\n",
		"fn f() {//c\n}\n",
		"fn f() { //c\n}\n",
		"fn f() {\n// c\n}\n",
		"fn f() { /*c*/ }\n",
		"fn f() { 1; //c\n}\n",
	} {
		parser := ts.NewParser()
		parser.SetLanguage(rust.Language())
		tree := parser.ParseString(nil, []byte(src))
		out += fmt.Sprintf("PROBE %-24q -> %s\n", src, tree.RootNode().String())
	}
	os.WriteFile("/tmp/probe.txt", []byte(out), 0o644)
}
