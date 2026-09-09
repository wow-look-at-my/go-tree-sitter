// Command ts-translate turns a tree-sitter generated parser.c into the table
// blob a grammar package embeds. The parse tables and the lexer state machines
// both travel as data, so the tree keeps no generated Go.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

func main() {
	out := flag.String("out", "", "path of the table blob to write")
	flag.Parse()
	if flag.NArg() != 1 || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: ts-translate -out tables.zst parser.c")
		os.Exit(2)
	}

	src, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	e := &emitter{file: parseFile(tokenize(src))}
	tables := e.buildTables()
	if err := checkABI(tables.Language.ABIVersion); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", flag.Arg(0), err)
		os.Exit(1)
	}
	blob, err := ts.EncodeTables(tables)
	if err != nil {
		fmt.Fprintln(os.Stderr, "encoding the tables:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, blob, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d bytes, %d lexer instructions\n",
		filepath.Base(*out), len(blob), len(tables.Lex.Code))
}

func collectSets(st lexStmt, names map[string]bool) {
	switch v := st.(type) {
	case stmtIf:
		collectSetsExpr(v.cond, names)
		for _, inner := range v.then {
			collectSets(inner, names)
		}
	}
}

func collectSetsExpr(e expr, names map[string]bool) {
	switch v := e.(type) {
	case exprSetContains:
		names[v.set] = true
	case exprUnary:
		collectSetsExpr(v.x, names)
	case exprBinary:
		collectSetsExpr(v.l, names)
		collectSetsExpr(v.r, names)
	}
}
