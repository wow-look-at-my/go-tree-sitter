// Command ts-goldens records the reference parse of a source file.
//
// It runs the upstream tree-sitter command line tool against a grammar
// directory and stores the result beside the source file.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func main() {
	grammar := flag.String("grammar", "", "path of the upstream grammar checkout")
	flag.Parse()
	if *grammar == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: ts-goldens -grammar DIR FILE...")
		os.Exit(2)
	}

	for _, path := range flag.Args() {
		cmd := exec.Command("tree-sitter", "parse", "--no-ranges", "-p", *grammar, path)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil && len(out) == 0 {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		if err := os.WriteFile(path+".golden", out, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s.golden (%d bytes)\n", path, len(out))
	}
}
