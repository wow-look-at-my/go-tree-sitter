// Command ts-fetch puts a grammar's upstream sources where ts-translate can
// read them. ts-translate fetches for itself when it is told which repository
// to read, so this command is for a caller that wants the two steps apart.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wow-look-at-my/go-tree-sitter/internal/grammarsrc"
)

func main() {
	repo := flag.String("repo", "", "upstream repository, as owner/name")
	rev := flag.String("rev", "", "commit to fetch")
	dir := flag.String("dir", "", "directory the submodule occupies")
	flag.Parse()
	if *repo == "" || *rev == "" || *dir == "" {
		fmt.Fprintln(os.Stderr, "ts-fetch: -repo, -rev and -dir are all required")
		os.Exit(2)
	}
	if err := grammarsrc.Fetch(*repo, *rev, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "ts-fetch: %v\n", err)
		os.Exit(1)
	}
}
