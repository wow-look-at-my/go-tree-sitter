// Command ts-translate turns a tree-sitter generated parser.c into the table
// blob a grammar package embeds, and the loader that embeds it. The parse
// tables and the lexer state machines both travel as data, so the Go it writes
// is the loader alone.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/internal/grammarsrc"
)

// tablesFile is the compressed table blob the generated loader embeds.
const tablesFile = "tables.zst"

func main() {
	pkg := flag.String("package", "", "name of the generated Go package")
	out := flag.String("out", "", "path of the generated Go file")
	standalone := flag.Bool("standalone", false,
		"emit Language here, for a package with no hand written half")
	scanner := flag.String("scanner", "",
		"import path of the package whose Scanner the grammar needs")
	repo := flag.String("repo", "",
		"upstream repository the grammar comes from, as owner/name")
	rev := flag.String("rev", "", "commit of that repository to read")
	flag.Parse()
	if flag.NArg() != 1 || *pkg == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: ts-translate -package NAME -out FILE parser.c")
		os.Exit(2)
	}
	if (*repo == "") != (*rev == "") {
		fmt.Fprintln(os.Stderr, "ts-translate: -repo and -rev go together")
		os.Exit(2)
	}

	// A module zip carries the submodule's gitlink and none of its files, so a
	// consumer's copy of this path is empty. Fetching here rather than in a
	// second directive is what stops a caller from writing one and not the
	// other, which reads as a working grammar until something asks it to parse.
	if *repo != "" {
		if err := fetchGrammar(flag.Arg(0), *repo, *rev); err != nil {
			fmt.Fprintln(os.Stderr, "ts-translate:", err)
			os.Exit(1)
		}
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
	loader := emitLoader(*pkg, scannerExpr(e.file.hasScanner, *scanner), *scanner, *standalone)

	dir := filepath.Dir(*out)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, tablesFile), blob, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(loader), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %s is %d bytes, %d lexer instructions\n",
		*pkg, tablesFile, len(blob), len(tables.Lex.Code))
}

// fetchGrammar puts the sources under parser where this command can read them.
// The caller names its parser.c, which already says where the submodule sits,
// so it does not have to name the directory a second time.
func fetchGrammar(parser, repo, rev string) error {
	dir, err := grammarsrc.DirFor(parser, repo)
	if err != nil {
		return err
	}
	return grammarsrc.Fetch(repo, rev, dir)
}

// scannerExpr names the external scanner the loader installs. A grammar with no
// external tokens installs none.
func scannerExpr(hasScanner bool, scannerPkg string) string {
	switch {
	case !hasScanner:
		return "nil"
	case scannerPkg != "":
		return "grammar.Scanner()"
	default:
		return "scanner{}"
	}
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
