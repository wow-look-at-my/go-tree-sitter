package bash_test

import (
	"fmt"
	"os"
	"testing"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/bash"
)

func TestProbe(t *testing.T) {
	out := ""
	for _, src := range []string{
		"echo $\n",
		"echo $$\n",
		"echo $$",
		"$$\n",
		"echo $$ x\n",
	} {
		parser := ts.NewParser()
		parser.SetLanguage(bash.Language())
		tree := parser.ParseString(nil, []byte(src))
		out += fmt.Sprintf("PROBE %-14q -> %s\n", src, tree.RootNode().String())
	}
	os.WriteFile("/tmp/probe.txt", []byte(out), 0o644)
}
