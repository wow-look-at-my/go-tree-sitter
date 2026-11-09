// query.go holds queryCommand.

package main

import (
	"errors"
	"flag"
	"os"

	"github.com/wow-look-at-my/go-tree-sitter/internal/querygoldens"
)

func init() { commands["query"] = queryCommand }

// queryCommand replaces the query goldens of one grammar, or of all of them.
func queryCommand(args []string) error {
	flags := flag.NewFlagSet("query", flag.ExitOnError)
	lang := flags.String("lang", "", `grammar name (go, c, cpp, rust, bash, javascript, typescript, tsx) or "all"`)
	cli := flags.String("cli", "tree-sitter", "tree-sitter executable, version 0.27.2")
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *lang == "" || flags.NArg() != 0 {
		return errors.New("usage: ts-goldens query -lang NAME|all [-cli PATH] [-root DIR]")
	}
	recorder := querygoldens.Recorder{CLI: *cli, Root: *root, Log: os.Stdout}
	grammars := querygoldens.Grammars
	if *lang != "all" {
		g, err := querygoldens.Find(*lang)
		if err != nil {
			return err
		}
		grammars = []querygoldens.Grammar{g}
	}
	for _, g := range grammars {
		if err := recorder.Record(g); err != nil {
			return err
		}
	}
	return nil
}
