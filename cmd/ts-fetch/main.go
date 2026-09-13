// Command ts-fetch puts a grammar's upstream sources where ts-translate can
// read them.
//
// The sources arrive as a git submodule, and a module zip carries the gitlink
// and none of the files. So a consumer that resolves this module from the
// proxy has an empty testdata directory, and the generate step that builds the
// parse tables has nothing to read. This fetches the same tree over HTTP when
// there is no git work tree to init.
package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	log := flag.String("repo", "", "upstream repository, as owner/name")
	rev := flag.String("rev", "", "commit to fetch")
	dir := flag.String("dir", "", "directory the submodule occupies")
	flag.Parse()
	if *log == "" || *rev == "" || *dir == "" {
		fmt.Fprintln(os.Stderr, "ts-fetch: -repo, -rev and -dir are all required")
		os.Exit(2)
	}
	if err := fetch(*log, *rev, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "ts-fetch: %v\n", err)
		os.Exit(1)
	}
}

// fetch makes dir hold the grammar at rev. It is a no-op once the sources are
// there, so a build that already has them costs nothing.
func fetch(repo, rev, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "src", "parser.c")); err == nil {
		return nil
	}
	if inGitWorkTree(dir) {
		cmd := exec.Command("git", "submodule", "update", "--init", "--depth", "1", "--", dir)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err == nil {
			return nil
		}
		// A shallow clone or a checkout with no submodule registered falls
		// through to the download rather than failing the generate.
	}
	return download(repo, rev, dir)
}

// inGitWorkTree reports whether dir sits inside a checkout git can act on.
func inGitWorkTree(dir string) bool {
	start := dir
	if _, err := os.Stat(start); err != nil {
		start = filepath.Dir(start)
	}
	cmd := exec.Command("git", "-C", start, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// download unpacks the repository's tree at rev into dir.
func download(repo, rev, dir string) error {
	url := fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", repo, rev)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: %s", url, resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}
	defer gz.Close()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// codeload wraps everything in one <name>-<rev>/ directory.
		_, rel, ok := strings.Cut(h.Name, "/")
		if !ok || rel == "" {
			continue
		}
		if err := extract(tr, h, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
}

// extract writes one archive entry, refusing a path that escapes dir.
func extract(tr *tar.Reader, h *tar.Header, dest string) error {
	if strings.Contains(h.Name, "..") {
		return fmt.Errorf("refusing entry %q", h.Name)
	}
	switch h.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dest, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, tr)
		return err
	}
	return nil
}
