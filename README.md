# go-tree-sitter

A tree-sitter runtime written in pure Go. No cgo, no WebAssembly runtime, and no external binary at run time.

Every other Go binding to tree-sitter is a cgo wrapper around the upstream C library. That rules them all out for a consumer such as `slopfix`. It ships as a fat APE, cross-compiled for linux and darwin on amd64 and arm64 with `CGO_ENABLED=0`. This runtime builds under that setting. It therefore cross-compiles to any target Go supports.

## The three parts, because this is the thing readers get wrong

A `grammar.js` file is data. It describes a language and it parses nothing.

`tree-sitter generate` reads that data and emits a parse table, as C source. The table is the language.

A runtime walks that table against real bytes and builds a tree. This repository is the runtime. `grammars/` holds tables translated from the generated C into Go by `cmd/ts-translate`.

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

Each grammar runs the upstream grammar repository's own `test/corpus` suite. tree-sitter-go passes 67 of 67 cases and tree-sitter-c passes 85 of 85. The real `tree-sitter test` command reports the same case counts on the same commits. That is what proves the reader is not dropping cases.

The corpus reader is a port of the tree-sitter CLI's own reader. A case is therefore read and compared the way upstream reads and compares it.

## The limit worth knowing

Only a grammar with no external scanner is covered. Grammars such as cpp, rust, bash and python ship a hand-written `scanner.c` that no machine translation can handle. None of them is built here yet. So nothing in this repository exercises the external-scanner path.

## Tests

The corpora are git submodules. A clone needs them:

```sh
git clone --recurse-submodules https://github.com/wow-look-at-my/go-tree-sitter.git
cd go-tree-sitter
go-toolchain
```

A checkout without the submodules fails the corpus tests loudly rather than reporting a green over zero cases.
