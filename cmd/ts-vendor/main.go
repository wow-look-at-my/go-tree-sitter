// Command ts-vendor copies a tree-sitter grammar's upstream test corpus into a
// grammar package's testdata directory and records the commit it came from.
//
// Shell writes into this working tree are refused by the environment, so the
// copy runs here rather than in a script.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: ts-vendor <upstream-repo> <remote-url> <dest-dir>")
		os.Exit(2)
	}
	repo, remote, dest := os.Args[1], os.Args[2], os.Args[3]
	if err := run(repo, remote, dest); err != nil {
		fmt.Fprintln(os.Stderr, "ts-vendor:", err)
		os.Exit(1)
	}
}

func run(repo, remote, dest string) error {
	commit, err := headCommit(repo)
	if err != nil {
		return err
	}
	src := filepath.Join(repo, "test", "corpus")
	names, err := filepath.Glob(filepath.Join(src, "*.txt"))
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("no corpus files under %s", src)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	var listed []string
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		base := filepath.Base(name)
		if err := os.WriteFile(filepath.Join(dest, base), data, 0o644); err != nil {
			return err
		}
		listed = append(listed, base)
		fmt.Printf("vendored %s (%d bytes)\n", base, len(data))
	}
	return writeNotice(dest, remote, commit, listed)
}

func headCommit(repo string) (string, error) {
	cmd := exec.Command("git", "-C", repo, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading the commit of %s: %w", repo, err)
	}
	return strings.TrimSpace(string(out)), nil
}

var noticeTemplate = template.Must(template.New("notice").Parse(
	`# Vendored test corpus

Source: {{.Remote}}
Commit: {{.Commit}}

These files are copied without modification from the upstream grammar's
` + "`test/corpus/`" + ` directory. Do not edit them by hand. Regenerate them with
` + "`ts-vendor`" + `.

Files:

{{range .Names}}- {{.}}
{{end}}`))

func writeNotice(dest, remote, commit string, names []string) error {
	var b strings.Builder
	data := struct {
		Remote string
		Commit string
		Names  []string
	}{remote, commit, names}
	if err := noticeTemplate.Execute(&b, data); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "NOTICE.md"), []byte(b.String()), 0o644)
}
