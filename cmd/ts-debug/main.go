// Command ts-debug parses one file with a bundled grammar and prints its tree.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/clang"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

func pick(name string) *ts.Language {
	switch name {
	case "go":
		return golang.Language()
	case "c":
		return clang.Language()
	}
	return nil
}

func main() {
	name := flag.String("language", "go", "grammar to use")
	limit := flag.Duration("limit", 10*time.Second, "how long to allow the parse to run")
	flag.Parse()

	language := pick(*name)
	if language == nil || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: ts-debug -language NAME FILE")
		os.Exit(2)
	}

	source, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	done := make(chan string, 1)
	go func() {
		parser := ts.NewParser()
		if !parser.SetLanguage(language) {
			done <- "the parser refused the language"
			return
		}
		tree := parser.ParseString(nil, source)
		if tree == nil {
			done <- "the parser returned no tree"
			return
		}
		done <- tree.String()
	}()

	select {
	case out := <-done:
		fmt.Println(out)
	case <-time.After(*limit):
		fmt.Fprintln(os.Stderr, "the parse did not finish in time")
		os.Exit(1)
	}
}
