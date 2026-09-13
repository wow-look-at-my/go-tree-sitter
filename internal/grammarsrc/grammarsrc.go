// Package grammarsrc puts a grammar's upstream sources where a translator can
// read them.
//
// The sources arrive as a git submodule, and a module zip carries the gitlink
// and none of the files. So a consumer that resolves this module from the
// proxy has an empty testdata directory, and the generate step that builds the
// parse tables has nothing to read. This fetches the same tree over HTTP when
// there is no git work tree to init.
package grammarsrc

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Fetch makes dir hold the grammar at rev. It is a no-op once the sources are
// there, so a build that already has them costs nothing.
//
// The sources are there when dir holds anything at all. A submodule directory
// is empty until it is initialized, a module zip carries none of it, and a
// download lands whole or not at all. A repository that keeps several grammars
// has no parser.c at its top, so asking for one would fetch it again for every
// grammar it holds.
func Fetch(repo, rev, dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
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

// DirFor answers the directory the submodule occupies, given a path inside it
// and the repository it came from. A caller that names its parser.c already
// names the directory, so it does not have to say it twice and the two cannot
// disagree.
//
// The submodule's own directory is the ancestor named after the repository:
// testdata/tree-sitter-bash/src/parser.c sits under testdata/tree-sitter-bash,
// and a grammar that keeps several parsers in one repository is no different,
// so testdata/tree-sitter-typescript/tsx/src/parser.c answers the same
// testdata/tree-sitter-typescript.
func DirFor(parser, repo string) (string, error) {
	_, name, split := strings.Cut(repo, "/")
	if !split || name == "" {
		return "", fmt.Errorf("repository %q is not owner/name", repo)
	}
	dir := filepath.Dir(filepath.Clean(parser))
	for {
		if filepath.Base(dir) == name {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %q directory above %s", name, parser)
		}
		dir = parent
	}
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

// codeload serves a repository's tree at any commit as a tarball.
var codeload = "https://codeload.github.com"

// download unpacks the repository's tree at rev into dir.
//
// The tree is unpacked beside dir and moved into place whole, so an interrupted
// download leaves dir as it found it rather than holding a parser.c that the
// next run would take for a complete one.
func download(repo, rev, dir string) error {
	url := fmt.Sprintf("%s/%s/tar.gz/%s", codeload, repo, rev)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: %s", url, resp.Status)
	}

	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := unpack(resp.Body, staging); err != nil {
		return fmt.Errorf("unpacking %s: %w", url, err)
	}
	if err := os.Chmod(staging, 0o755); err != nil {
		return err
	}
	// A submodule that was never initialized is an empty directory, which is
	// what the move replaces. Anything else there is not ours to remove.
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("making room for the sources: %w", err)
	}
	return os.Rename(staging, dir)
}

// unpack writes a gzipped tarball into dir, dropping the one top directory
// codeload wraps everything in.
func unpack(body io.Reader, dir string) error {
	unzip, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	defer unzip.Close()

	archive := tar.NewReader(unzip)
	for {
		hdr, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, split := strings.Cut(hdr.Name, "/")
		if !split || rel == "" {
			continue
		}
		if err := extract(archive, hdr, filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
}

// extract writes one archive entry, refusing a path that escapes dir.
func extract(archive *tar.Reader, hdr *tar.Header, dest string) error {
	if strings.Contains(hdr.Name, "..") {
		return fmt.Errorf("refusing entry %q", hdr.Name)
	}
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(dest, 0o755)
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(file, archive); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}
	// A link or a device would come out of a checkout as something this does
	// not write, so the tree would differ from the submodule's without a word.
	return fmt.Errorf("refusing entry %q of type %q", hdr.Name, hdr.Typeflag)
}
