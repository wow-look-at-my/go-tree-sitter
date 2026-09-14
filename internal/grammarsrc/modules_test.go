package grammarsrc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modulesFile is a .gitmodules holding two grammars, one of which keeps
// several parsers in one repository.
const modulesFile = `[submodule "grammars/golang/testdata/tree-sitter-go"]
	path = grammars/golang/testdata/tree-sitter-go
	url = https://github.com/tree-sitter/tree-sitter-go.git
[submodule "grammars/typescript/testdata/tree-sitter-typescript"]
	path = grammars/typescript/testdata/tree-sitter-typescript
	url = https://github.com/tree-sitter/tree-sitter-typescript.git
`

func TestParseModulesReadsEveryDeclaration(test *testing.T) {
	assert.Equal(test, map[string]string{
		"grammars/golang/testdata/tree-sitter-go":             "https://github.com/tree-sitter/tree-sitter-go.git",
		"grammars/typescript/testdata/tree-sitter-typescript": "https://github.com/tree-sitter/tree-sitter-typescript.git",
	}, ParseModules([]byte(modulesFile)))
}

// A section with no url declares no submodule to fetch, and its path does not
// carry over into the next one.
func TestParseModulesSkipsAnIncompleteSection(test *testing.T) {
	const contents = `[submodule "half"]
	path = vendor/half
[submodule "whole"]
	path = vendor/whole
	url = https://github.com/owner/whole.git
`
	assert.Equal(test, map[string]string{
		"vendor/whole": "https://github.com/owner/whole.git",
	}, ParseModules([]byte(contents)))
}

func TestSubmoduleForTheHoldingDirectory(test *testing.T) {
	root := filepath.FromSlash("/repo")
	dir, url, err := SubmoduleFor(root, ParseModules([]byte(modulesFile)),
		filepath.Join(root, "grammars", "golang", "testdata", "tree-sitter-go", "src", "parser.c"))

	require.NoError(test, err)
	assert.Equal(test, filepath.Join(root, "grammars", "golang", "testdata", "tree-sitter-go"), dir)
	assert.Equal(test, "https://github.com/tree-sitter/tree-sitter-go.git", url)
}

// The tsx grammar's directive reaches into the typescript package, so the
// answer is decided by the path rather than by the caller's directory.
func TestSubmoduleForASiblingPackage(test *testing.T) {
	root := filepath.FromSlash("/repo")
	dir, _, err := SubmoduleFor(root, ParseModules([]byte(modulesFile)),
		filepath.Join(root, "grammars", "typescript", "testdata", "tree-sitter-typescript", "tsx", "src", "parser.c"))

	require.NoError(test, err)
	assert.Equal(test, filepath.Join(root, "grammars", "typescript", "testdata", "tree-sitter-typescript"), dir)
}

// A submodule nested inside another one holds the files it contains.
func TestSubmoduleForTheLongestDeclaredPath(test *testing.T) {
	root := filepath.FromSlash("/repo")
	urls := map[string]string{
		"vendor":       "https://github.com/owner/outer.git",
		"vendor/inner": "https://github.com/owner/inner.git",
	}

	dir, url, err := SubmoduleFor(root, urls, filepath.Join(root, "vendor", "inner", "src", "parser.c"))

	require.NoError(test, err)
	assert.Equal(test, filepath.Join(root, "vendor", "inner"), dir)
	assert.Equal(test, "https://github.com/owner/inner.git", url)
}

// A path no submodule holds is reported rather than fetched from a guess.
func TestSubmoduleForRefusesWhatItCannotPlace(test *testing.T) {
	root := filepath.FromSlash("/repo")
	for _, row := range []struct {
		name   string
		urls   map[string]string
		target string
	}{
		{
			name:   "no submodule declares that grammar",
			urls:   ParseModules([]byte(modulesFile)),
			target: filepath.Join(root, "grammars", "ruby", "testdata", "tree-sitter-ruby", "src", "parser.c"),
		},
		{
			name:   "a declared path that stops mid name",
			urls:   map[string]string{"vendor/lib": "https://github.com/owner/lib.git"},
			target: filepath.Join(root, "vendor", "library", "src", "parser.c"),
		},
	} {
		test.Run(row.name, func(test *testing.T) {
			_, _, err := SubmoduleFor(root, row.urls, row.target)
			require.Error(test, err)
			assert.Contains(test, err.Error(), "no submodule")
		})
	}
}

func TestFindModulesWalksUp(test *testing.T) {
	root := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(modulesFile), 0o644))
	deep := filepath.Join(root, "grammars", "golang")
	require.NoError(test, os.MkdirAll(deep, 0o755))

	found, contents, err := FindModules(deep)

	require.NoError(test, err)
	assert.Equal(test, root, found)
	assert.Equal(test, modulesFile, string(contents))
}

// A directory that does not exist yet is walked through: the submodule's own
// path is absent exactly when there is fetching to do.
func TestFindModulesThroughAnAbsentDirectory(test *testing.T) {
	root := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(modulesFile), 0o644))

	found, _, err := FindModules(filepath.Join(root, "grammars", "golang", "testdata", "tree-sitter-go", "src"))

	require.NoError(test, err)
	assert.Equal(test, root, found)
}

func TestFindModulesReportsAbsence(test *testing.T) {
	_, _, err := FindModules(test.TempDir())

	require.Error(test, err)
	assert.Contains(test, err.Error(), "no .gitmodules")
}

func TestRepoForReadsBothSpellingsGitWrites(test *testing.T) {
	for _, row := range []struct{ url, want string }{
		{"https://github.com/tree-sitter/tree-sitter-go.git", "tree-sitter/tree-sitter-go"},
		{"https://github.com/tree-sitter/tree-sitter-go", "tree-sitter/tree-sitter-go"},
		{"https://github.com/tree-sitter/tree-sitter-go/", "tree-sitter/tree-sitter-go"},
		{"git@github.com:tree-sitter/tree-sitter-go.git", "tree-sitter/tree-sitter-go"},
	} {
		got, err := RepoFor(row.url)
		require.NoError(test, err, row.url)
		assert.Equal(test, row.want, got, row.url)
	}
}

// A host codeload does not serve has no owner/name to ask it for, and a URL
// that names no repository is not turned into one.
func TestRepoForRefusesWhatCodeloadCannotServe(test *testing.T) {
	for _, url := range []string{
		"https://gitlab.com/owner/name.git",
		"https://github.com/owner",
		"https://github.com/owner/",
		"https://github.com/owner/name/extra",
		"../relative/path",
	} {
		_, err := RepoFor(url)
		assert.Error(test, err, url)
	}
}

// The whole point of reading .gitmodules: a consumer's copy carries that file
// and no submodule contents, and the directive that names no repository still
// finds the grammar. There is no commit to pin to, so the tip is what arrives.
func TestSourceFetchesTheRepositoryGitmodulesNames(test *testing.T) {
	withoutGit(test)
	hits := serve(test, "tree-sitter/tree-sitter-demo", "HEAD", tarball(test, grammarTree("HEAD")))
	root := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(
		"[submodule \"grammars/demo/testdata/tree-sitter-demo\"]\n"+
			"\tpath = grammars/demo/testdata/tree-sitter-demo\n"+
			"\turl = https://github.com/tree-sitter/tree-sitter-demo.git\n"), 0o644))
	parser := filepath.Join(root, "grammars", "demo", "testdata", "tree-sitter-demo", "src", "parser.c")

	src, err := Source(parser, "", "")

	require.NoError(test, err)
	assert.Equal(test, "/* parser */\n", string(src))
	assert.Equal(test, 1, *hits)
}

// A named repository is fetched at the commit it was pinned to, and the
// directory comes out of the parser path.
func TestSourceFetchesThePinnedRevision(test *testing.T) {
	withoutGit(test)
	const rev = "0123456789abcdef0123456789abcdef01234567"
	hits := serve(test, "tree-sitter/tree-sitter-demo", rev, tarball(test, grammarTree(rev)))
	parser := filepath.Join(test.TempDir(), "testdata", "tree-sitter-demo", "src", "parser.c")

	src, err := Source(parser, "tree-sitter/tree-sitter-demo", rev)

	require.NoError(test, err)
	assert.Equal(test, "/* parser */\n", string(src))
	assert.Equal(test, 1, *hits)
}

// A tree that already carries the grammar is read as it stands, and asks
// codeload for nothing.
func TestSourceKeepsAFileThatIsAlreadyThere(test *testing.T) {
	withoutGit(test)
	hits := serve(test, "tree-sitter/tree-sitter-demo", "HEAD", nil)
	dir := test.TempDir()
	parser := filepath.Join(dir, "parser.c")
	require.NoError(test, os.WriteFile(parser, []byte("int main;"), 0o644))

	src, err := Source(parser, "", "")

	require.NoError(test, err)
	assert.Equal(test, "int main;", string(src))
	assert.Zero(test, *hits)
}

// With no repository named and no .gitmodules to read, there is nowhere to
// fetch from, and the missing file is reported rather than guessed at.
func TestSourceReportsAGrammarItCannotPlace(test *testing.T) {
	withoutGit(test)
	root := test.TempDir()

	_, err := Source(filepath.Join(root, "testdata", "tree-sitter-demo", "src", "parser.c"), "", "")

	require.Error(test, err)
	assert.Contains(test, err.Error(), "no .gitmodules")
}

// Fetching over a populated directory would destroy whatever is there, so the
// clone refuses it even though the callers above never reach it with one.
func TestCloneRefusesAPopulatedDirectory(test *testing.T) {
	dir := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(dir, "keep"), nil, 0o644))

	err := clone("https://example.com/x.git", dir)

	require.Error(test, err)
	assert.Contains(test, err.Error(), "not empty")
}

// A host codeload does not serve is cloned with git instead of downloaded.
func TestFetchURLClonesAHostCodeloadDoesNotServe(test *testing.T) {
	withoutGit(test)
	hits := serve(test, "owner/name", "HEAD", nil)
	dir := filepath.Join(test.TempDir(), "name")

	err := fetchURL("https://gitlab.com/owner/name.git", dir)

	require.Error(test, err, "git is not on PATH, so the clone cannot succeed")
	assert.Zero(test, *hits, "a host codeload does not serve was asked of codeload")
}

// A tree that is there already is left alone whichever way it would be fetched.
func TestFetchURLIsANoOpOnceTheTreeIsThere(test *testing.T) {
	withoutGit(test)
	hits := serve(test, "tree-sitter/tree-sitter-demo", "HEAD", nil)
	dir := test.TempDir()
	require.NoError(test, os.WriteFile(filepath.Join(dir, "parser.c"), nil, 0o644))

	require.NoError(test, fetchURL("https://github.com/tree-sitter/tree-sitter-demo.git", dir))

	assert.Zero(test, *hits)
}
