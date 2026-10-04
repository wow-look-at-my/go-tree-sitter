package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A module zip carries a .gitmodules file and no submodule contents, so the
// parser.c a directive names is absent everywhere except a git checkout. A
// consumer's `go generate` then has nothing to translate. This fetches the
// grammar from the URL .gitmodules already records.
//
// The clone takes the default branch. A tree-sitter grammar is an input to a
// build-time translation, and a pin here would record in this repository a
// commit that belongs to another one.

// readSource answers the grammar's parser.c, fetching its submodule when the
// tree does not carry one.
func readSource(path string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err == nil || !os.IsNotExist(err) {
		return src, err
	}
	if err := ensureSource(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// ensureSource makes the file at path readable, cloning the submodule that
// holds it when it is not there yet.
func ensureSource(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, contents, err := findModules(filepath.Dir(abs))
	if err != nil {
		return err
	}
	dir, url, err := submoduleFor(root, parseModules(contents), abs)
	if err != nil {
		return err
	}
	return clone(url, dir)
}

// findModules walks up from dir for the .gitmodules that describes it, and
// answers the directory holding it alongside its contents.
func findModules(dir string) (root string, contents []byte, err error) {
	for {
		b, err := os.ReadFile(filepath.Join(dir, ".gitmodules"))
		if err == nil {
			return dir, b, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil, fmt.Errorf("no .gitmodules above %s", dir)
		}
		dir = parent
	}
}

// submoduleFor answers the directory and URL of the submodule holding target.
// The longest declared path wins, so a submodule nested inside another one is
// the answer for a file it contains.
func submoduleFor(root string, urls map[string]string, target string) (dir, url string, err error) {
	for rel, u := range urls {
		d := filepath.Join(root, filepath.FromSlash(rel))
		if target != d && !strings.HasPrefix(target, d+string(filepath.Separator)) {
			continue
		}
		if len(d) > len(dir) {
			dir, url = d, u
		}
	}
	if dir == "" {
		return "", "", fmt.Errorf("no submodule in %s holds %s",
			filepath.Join(root, ".gitmodules"), target)
	}
	return dir, url, nil
}

// clone puts the repository at url in dir, at the tip of its default branch.
func clone(url, dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty: refusing to clone %s over it", dir, url)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "ts-translate: cloning %s into %s\n", url, dir)
	cmd := exec.Command("git", "clone", "--depth", "1", url, dir)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// parseModules reads the path and url of every submodule .gitmodules declares.
// The file is git config: a section header, then indented key lines.
func parseModules(contents []byte) map[string]string {
	urls := map[string]string{}
	var path, url string
	commit := func() {
		if path != "" && url != "" {
			urls[path] = url
		}
		path, url = "", ""
	}
	s := bufio.NewScanner(bytes.NewReader(contents))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[") {
			commit()
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "path":
			path = strings.TrimSpace(value)
		case "url":
			url = strings.TrimSpace(value)
		}
	}
	commit()
	return urls
}
