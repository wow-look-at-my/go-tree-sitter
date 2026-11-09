// parse.go holds parseCommand.

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func init() { commands["parse"] = parseCommand }

// parseCommand stores the reference parse of each file beside it.
func parseCommand(args []string) error {
	flags := flag.NewFlagSet("parse", flag.ExitOnError)
	grammar := flags.String("grammar", "", "path of the upstream grammar checkout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *grammar == "" || flags.NArg() == 0 {
		return errors.New("usage: ts-goldens [parse] -grammar DIR FILE...")
	}

	for _, path := range flags.Args() {
		cmd := exec.Command("tree-sitter", "parse", "--no-ranges", "-p", *grammar, path)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil && len(out) == 0 {
			return fmt.Errorf("%s: %v", path, err)
		}
		if err := os.WriteFile(path+".golden", out, 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %s.golden (%d bytes)\n", path, len(out))
	}
	return nil
}
