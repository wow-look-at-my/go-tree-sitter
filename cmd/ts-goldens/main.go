// Command ts-goldens records reference results with the upstream tree-sitter
// command line tool.
//
//	ts-goldens [parse] -grammar DIR FILE...   parse trees beside each file
//	ts-goldens query -lang NAME|all           query goldens of a grammar
package main

import (
	"fmt"
	"os"
)

// commands maps a subcommand name to its entry point. Each subcommand file registers itself.
var commands = map[string]func(args []string) error{}

func main() {
	args := os.Args[1:]
	run := commands["parse"]
	if len(args) > 0 {
		if cmd, ok := commands[args[0]]; ok {
			run, args = cmd, args[1:]
		}
	}
	if err := run(args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
