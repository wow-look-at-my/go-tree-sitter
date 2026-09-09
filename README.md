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

`Node` also carries `NamedChild`, `Parent`, `NextSibling`, `ChildByFieldName`, and byte and point ranges. `Tree.Walk` returns a cursor for a full traversal.

## Status

Each grammar runs the upstream grammar repository's own `test/corpus` suite, and every case in it passes. The real `tree-sitter test` command reports the same case counts on the same commits. That is what proves the reader is not dropping cases.

The corpus reader is a port of the tree-sitter CLI's own reader. A case is therefore read and compared the way upstream reads and compares it.

## Grammars

Go, C, C++, Rust, Bash, JavaScript, TypeScript and TSX.

A grammar that ships a hand-written `scanner.c` needs that scanner ported by hand. No machine translation reads C well enough to do it. `grammars/*/scanner.go` is that port. The upstream corpus says whether it is right. TSX shares TypeScript's scanner, the way upstream compiles both from one header.

`ts-translate` writes one file per grammar. That file is `tables.zst`. It is COMMITTED. A module carries no submodule and gets no generate step. So `go get` hands a consumer the C the tables come from, and no way to translate it. CI runs `go generate` and fails on a dirty tree. A committed blob that no longer matches its submodule is therefore a red build, never a stale answer.

The blob holds the lexer too. A tree-sitter lexer arrives as a C function of a few thousand `goto`s. Translating that into Go source puts about a megabyte of generated code in the tree, per grammar. It is a state machine, so it travels as data instead. `lexprog.go` defines sixteen instructions and runs them. A whole parse costs about 12% more than the compiled form did, which `BenchmarkParse` measures. Everything checked in here is source somebody wrote.

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
