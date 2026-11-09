# go-tree-sitter

A tree-sitter runtime written in pure Go. No cgo, no WebAssembly runtime, and no external binary at run time.

Every other Go binding to tree-sitter is a cgo wrapper around the upstream C library. That rules them all out for a consumer such as `slopfix`. It ships as a fat APE, cross-compiled for linux and darwin on amd64 and arm64 with `CGO_ENABLED=0`. This runtime builds under that setting. It therefore cross-compiles to any target Go supports.

## The three parts, because this is the thing readers get wrong

A `grammar.js` file is data. It describes a language and it parses nothing.

`tree-sitter generate` reads that data and emits a parse table, as C source. The table is the language.

A runtime walks that table against real bytes and builds a tree. This repository is the runtime. `grammars/` holds tables `cmd/ts-translate` reads out of that C.

## Install

```sh
go get github.com/wow-look-at-my/go-tree-sitter
```

## Parse a file

```go
package main

import (
	"fmt"

	ts "github.com/wow-look-at-my/go-tree-sitter"
	"github.com/wow-look-at-my/go-tree-sitter/grammars/golang"
)

func main() {
	parser := ts.NewParser()
	parser.SetLanguage(golang.Language())

	tree := parser.ParseString(nil, []byte("package main\n"))
	root := tree.RootNode()

	fmt.Println(root.Type())       // source_file
	fmt.Println(root.StartPoint()) // {0 0}
	fmt.Println(root.String())     // (source_file (package_clause (package_identifier)))
}
```

`Tree.Walk` returns a cursor for a full traversal.

## Status

Each grammar runs the upstream grammar repository's own `test/corpus` suite, and every case in it passes. The real `tree-sitter test` command reports the same case counts on the same commits. That is what proves the reader is not dropping cases.

The corpus reader is a port of the tree-sitter CLI's own reader. A case is therefore read and compared the way upstream reads and compares it.

## Grammars

Go, C, C++, Rust, Bash, JavaScript, TypeScript and TSX.

A grammar that ships a hand-written `scanner.c` needs that scanner ported by hand. No machine translation reads C well enough to do it. `grammars/*/scanner.go` is that port. The upstream corpus says whether it is right. TSX shares TypeScript's scanner, the way upstream compiles both from one header.

`ts-translate` writes `tables.zst` per grammar, and the `parser.go` that embeds it. Neither is committed. Both are derived. The repository keeps the input, and the generate step produces the rest at build time, into the binary. `grammar.go` beside them is hand written and compiles without them, which is what lets a consumer import a grammar for its `Scanner()`. `Language()` panics until the generate step ran.

The blob holds the lexer too. A tree-sitter lexer arrives as a C function of a few thousand `goto`s. Translating that into Go source put about a megabyte of it in each grammar. It is a state machine, so it travels as data instead. The generated Go is down to the loader that embeds the blob. `lexprog.go` defines multiple instructions and runs them.

A whole parse costs about 12% more than the compiled form did, which `BenchmarkParse` measures.

## Queries

The query engine is a port of `lib/src/query.c` from tree-sitter v0.27.2, commit `3fb7be49145acfb07e33b61ed5689e2b6771b2e9`, plus the predicate layer the Rust binding adds in `lib/binding_rust/lib.rs`. The `query*.go` files hold it, one concern per file, and every function names the upstream function it ports.

```go
query, err := ts.NewQuery(golang.Language(), `(function_declaration name: (identifier) @name)`)
if err != nil {
	log.Fatal(err) // a *ts.QueryError with Kind, Offset, Row and Column
}
cursor := ts.NewQueryCursor()
for match := range cursor.Matches(query, tree.RootNode(), source) {
	for _, capture := range match.Captures {
		fmt.Println(query.CaptureNameForID(capture.Index), capture.Node.Type())
	}
}
```

`Matches` and `Captures` are `iter.Seq` and `iter.Seq2` wrappers that apply the text predicates. Byte and point ranges, containing ranges, the match limit, the maximum start depth and the progress callback all carry their upstream names. Any other predicate comes back as data.

Oracles check the port against upstream:

- The upstream query suite, `crates/cli/src/tests/query_test.rs`, is ported in `query_upstream*_test.go`. Every test is ported or sits in the skip table with its reason. It generates a grammar from source, uses a test fixture grammar, or draws random patterns. A test written for a grammar this repository lacks is rewritten against one it has, and the pinned CLI confirmed each rewrite.
- Goldens from `tree-sitter query` 0.27.2, in match and captures mode, for every bundled `.scm` file over each grammar's samples, small examples and corpus inputs. Every golden file matches byte for byte. Examples over 16 KB are left out to keep the goldens small.
- Compile parity: every bundled query file compiles in both. Hand-written invalid queries under `grammars/golang/testdata/invalid-queries` cover every error kind, with the same kind, offset, row and column as the CLI.

| Check | Count |
| --- | --- |
| Upstream query tests | 131 |
| Ported and passing | 124 |
| Skipped, with a reason | 7 |
| Query golden files, byte for byte | 416 |
| Bundled `.scm` files, compile parity | 22 |
| Hand-written invalid queries | 9 |

`FuzzNewQuery` and `FuzzQueryExec` fuzz compilation and execution. `BenchmarkCompileHighlights` and `BenchmarkHighlightCaptures` in the Rust and TypeScript grammars report allocations per operation.

Known divergences from upstream:

- `#match?` uses Go's `regexp`, which is RE2 syntax, where upstream uses the Rust `regex` crate. The bundled queries use no construct the two read differently. `query_regex_test.go` lists the constructs that differ and refuses a bundled regex that uses one.
- The pattern reader classifies letters and spaces as the C locale does, with ASCII only, because that is the locale the CLI runs under.
- A query with CRLF line endings reports the column the CLI reports. The Rust binding drops each `\r` while it counts lines, so the column drifts by one per earlier line. The byte offset is exact in both.
- A `TextProvider` returns one byte slice per node. The Rust one yields chunks.
- Executing over a null node reports no matches. Upstream reads through the null pointer.

## The limit worth knowing

A grammar reaches this runtime by way of `cmd/ts-translate`, which refuses an ABI outside the range `parser_core.go` accepts. A grammar outside that range fails the build rather than becoming a package that loads and parses nothing.

## Tests

The corpora are git submodules. A clone needs them:

```sh
git clone --recurse-submodules https://github.com/wow-look-at-my/go-tree-sitter.git
cd go-tree-sitter
go-toolchain
```

A checkout without the submodules fails the corpus tests loudly rather than reporting a green over zero cases.

Each grammar's bundled queries also run against its samples, its small examples and its corpus inputs. The output must match `tree-sitter query` 0.27.2 byte for byte. The goldens live in `grammars/*/testdata/queries`, and `internal/querygoldens` describes their layout. To record them again:

```sh
TS_GOLDENS_RECORD=1 TS_GOLDENS_CLI=/path/to/tree-sitter go-toolchain
```

`ts-goldens query -lang all -cli /path/to/tree-sitter` does the same from a built binary.
