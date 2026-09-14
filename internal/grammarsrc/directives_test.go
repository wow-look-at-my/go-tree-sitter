package grammarsrc

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moduleRoot is this module's top directory, as seen from this package.
const moduleRoot = "../.."

// directive is one grammar package's generate line, reduced to what says where
// its sources come from.
type directive struct {
	// pkg is the grammar package's directory, relative to the module root.
	pkg string
	// repo and rev name the upstream tree the directive fetches.
	repo, rev string
	// parser is the parser.c the directive reads, relative to pkg.
	parser string
}

// submodule answers the directory the directive fetches into, relative to the
// module root.
func (dir directive) submodule(test *testing.T) string {
	test.Helper()
	sub, err := DirFor(dir.parser, dir.repo)
	require.NoError(test, err)
	return filepath.ToSlash(filepath.Join(dir.pkg, sub))
}

// grammarDirectives reads every ts-translate directive under grammars/.
func grammarDirectives(test *testing.T) []directive {
	test.Helper()
	files, err := filepath.Glob(filepath.Join(moduleRoot, "grammars", "*", "*.go"))
	require.NoError(test, err)
	var found []directive
	for _, file := range files {
		found = append(found, fileDirectives(test, file)...)
	}
	require.NotEmpty(test, found, "no ts-translate directive under grammars/")
	sort.Slice(found, func(lhs, rhs int) bool { return found[lhs].pkg < found[rhs].pkg })
	return found
}

func fileDirectives(test *testing.T, file string) []directive {
	test.Helper()
	open, err := os.Open(file)
	require.NoError(test, err)
	defer open.Close()

	rel, err := filepath.Rel(moduleRoot, filepath.Dir(file))
	require.NoError(test, err)
	var found []directive
	scan := bufio.NewScanner(open)
	for scan.Scan() {
		line, isDirective := strings.CutPrefix(scan.Text(), "//go:generate ")
		if !isDirective || !strings.Contains(line, "/cmd/ts-translate") {
			continue
		}
		words := strings.Fields(line)
		found = append(found, directive{
			pkg:    filepath.ToSlash(rel),
			repo:   flagValue(words, "-repo"),
			rev:    flagValue(words, "-rev"),
			parser: words[len(words)-1],
		})
	}
	require.NoError(test, scan.Err())
	return found
}

func flagValue(words []string, name string) string {
	for idx, word := range words[:len(words)-1] {
		if word == name {
			return words[idx+1]
		}
	}
	return ""
}

// The directive fetches the commit it names, and a checkout's submodule holds
// the commit its gitlink records. Those have to be one commit, or a checkout
// and a consumer build two different grammars from one version of this module.
func TestEachDirectiveFetchesTheCommitItsSubmoduleRecords(test *testing.T) {
	urls := submoduleURLs(test)
	for _, dir := range grammarDirectives(test) {
		test.Run(dir.pkg, func(test *testing.T) {
			require.NotEmpty(test, dir.repo, "the directive names no -repo")
			require.NotEmpty(test, dir.rev, "the directive names no -rev")
			path := dir.submodule(test)

			out, err := exec.Command("git", "-C", moduleRoot, "ls-files", "--stage", "--", path).Output()
			require.NoError(test, err, "git ls-files %s", path)
			fields := strings.Fields(string(out))
			require.Len(test, fields, 4, "%s is not one gitlink: %q", path, out)
			assert.Equal(test, "160000", fields[0], "%s is not a submodule", path)
			assert.Equal(test, fields[1], dir.rev, "the directive fetches a commit the submodule does not record")

			assert.Equal(test, "https://github.com/"+dir.repo+".git", urls[path],
				"the directive fetches a repository the submodule does not name")
		})
	}
}

// submoduleURLs reads .gitmodules into a path to url map.
func submoduleURLs(test *testing.T) map[string]string {
	test.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, ".gitmodules"))
	require.NoError(test, err)
	return ParseModules(body)
}

// A directive that names no repository reads the one .gitmodules records, so
// the two have to agree on every grammar or the pinned build and the unpinned
// one read different repositories.
func TestEveryDeclaredURLNamesTheRepositoryItsDirectiveFetches(test *testing.T) {
	urls := submoduleURLs(test)
	require.NotEmpty(test, urls)
	for _, dir := range grammarDirectives(test) {
		test.Run(dir.pkg, func(test *testing.T) {
			repo, err := RepoFor(urls[dir.submodule(test)])
			require.NoError(test, err)
			assert.Equal(test, dir.repo, repo)
		})
	}
}
