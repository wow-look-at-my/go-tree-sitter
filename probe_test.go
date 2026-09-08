package treesitter_test

import (
	"runtime"
	"testing"
	"time"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

func TestGoGrammarFinishes(t *testing.T) {
	done := make(chan string, 1)
	go func() {
		parser := ts.NewParser()
		if !parser.SetLanguage(golang.Language()) {
			done <- "refused"
			return
		}
		tree := parser.ParseString(nil, []byte("package main\n"))
		if tree == nil {
			done <- "no tree"
			return
		}
		done <- tree.String()
	}()

	select {
	case out := <-done:
		t.Log(out)
	case <-time.After(3 * time.Second):
		buf := make([]byte, 1<<16)
		n := runtime.Stack(buf, true)
		t.Fatalf("the parse did not finish\n%s", buf[:n])
	}
}
