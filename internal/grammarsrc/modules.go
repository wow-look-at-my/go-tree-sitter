package grammarsrc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FetchFromModules puts the sources that hold parser where a translator can
// read them, taking the repository from the .gitmodules above parser.
//
// This is the answer for a caller that names no repository of its own: a
// module zip carries .gitmodules and none of the submodule contents, so the
// file that records where each grammar comes from is present exactly when the
// grammars themselves are not. The fetch takes the tip of the default branch,
// since .gitmodules records a URL and no commit.
func FetchFromModules(parser string) error {
	abs, err := filepath.Abs(parser)
	if err != nil {
		return err
	}
	root, contents, err := FindModules(filepath.Dir(abs))
	if err != nil {
		return err
	}
	dir, url, err := SubmoduleFor(root, ParseModules(contents), abs)
	if err != nil {
		return err
	}
	return fetchURL(url, dir)
}

// FindModules walks up from dir for the .gitmodules that describes it, and
// answers the directory holding it alongside its contents.
//
// A directory that does not exist yet is walked through: a submodule's own
// path is absent exactly when there is fetching to do.
func FindModules(dir string) (root string, contents []byte, err error) {
	for {
		body, err := os.ReadFile(filepath.Join(dir, ".gitmodules"))
		if err == nil {
			return dir, body, nil
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

// ParseModules reads the path and url of every submodule .gitmodules declares.
// The file is git config: a section header, then indented key lines.
func ParseModules(contents []byte) map[string]string {
	urls := map[string]string{}
	var path, url string
	record := func() {
		if path != "" && url != "" {
			urls[path] = url
		}
		path, url = "", ""
	}
	lines := bufio.NewScanner(bytes.NewReader(contents))
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if strings.HasPrefix(line, "[") {
			record()
			continue
		}
		key, value, isPair := strings.Cut(line, "=")
		if !isPair {
			continue
		}
		switch strings.TrimSpace(key) {
		case "path":
			path = strings.TrimSpace(value)
		case "url":
			url = strings.TrimSpace(value)
		}
	}
	record()
	return urls
}

// SubmoduleFor answers the directory and URL of the submodule holding target.
// The longest declared path wins, so a submodule nested inside another one is
// the answer for a file it contains.
func SubmoduleFor(root string, urls map[string]string, target string) (dir, url string, err error) {
	for rel, declared := range urls {
		held := filepath.Join(root, filepath.FromSlash(rel))
		if target != held && !strings.HasPrefix(target, held+string(filepath.Separator)) {
			continue
		}
		if len(held) > len(dir) {
			dir, url = held, declared
		}
	}
	if dir == "" {
		return "", "", fmt.Errorf("no submodule in %s holds %s",
			filepath.Join(root, ".gitmodules"), target)
	}
	return dir, url, nil
}

// RepoFor answers the owner/name a git URL addresses, for the hosts codeload
// serves. Both spellings git writes into .gitmodules are read: the https URL
// and the scp-like ssh one.
func RepoFor(url string) (string, error) {
	rest, isHTTPS := strings.CutPrefix(url, "https://github.com/")
	if !isHTTPS {
		rest, isHTTPS = strings.CutPrefix(url, "git@github.com:")
	}
	if !isHTTPS {
		return "", fmt.Errorf("%q is not a github.com repository", url)
	}
	rest = strings.TrimSuffix(strings.TrimSuffix(rest, "/"), ".git")
	owner, name, isPair := strings.Cut(rest, "/")
	if !isPair || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", fmt.Errorf("%q names no owner/name", url)
	}
	return owner + "/" + name, nil
}

// fetchURL makes dir hold the repository at url, at the tip of its default
// branch. It is a no-op once the sources are there, on the same terms as Fetch.
//
// A host codeload does not serve is cloned with git, which is the only way to
// read it. That clone needs git on PATH, where the download does not.
func fetchURL(url, dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil
	}
	if inGitWorkTree(dir) {
		cmd := exec.Command("git", "submodule", "update", "--init", "--depth", "1", "--", dir)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
	}
	repo, err := RepoFor(url)
	if err != nil {
		return clone(url, dir)
	}
	return download(repo, "HEAD", dir)
}

// clone puts the repository at url in dir, at the tip of its default branch.
func clone(url, dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s is not empty: refusing to clone %s over it", dir, url)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "grammarsrc: cloning %s into %s\n", url, dir)
	cmd := exec.Command("git", "clone", "--depth", "1", url, dir)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}
