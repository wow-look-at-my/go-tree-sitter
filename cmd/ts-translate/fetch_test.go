package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const modules = `[submodule "grammars/golang/testdata/tree-sitter-go"]
	path = grammars/golang/testdata/tree-sitter-go
	url = https://github.com/tree-sitter/tree-sitter-go.git
[submodule "grammars/typescript/testdata/tree-sitter-typescript"]
	path = grammars/typescript/testdata/tree-sitter-typescript
	url = https://github.com/tree-sitter/tree-sitter-typescript.git
`

func TestParseModules(t *testing.T) {
	urls := parseModules([]byte(modules))
	assert.Equal(t, map[string]string{
		"grammars/golang/testdata/tree-sitter-go":             "https://github.com/tree-sitter/tree-sitter-go.git",
		"grammars/typescript/testdata/tree-sitter-typescript": "https://github.com/tree-sitter/tree-sitter-typescript.git",
	}, urls)
}

func TestSubmoduleForHoldingDirectory(t *testing.T) {
	root := "/repo"
	dir, url, err := submoduleFor(root, parseModules([]byte(modules)),
		filepath.Join(root, "grammars/golang/testdata/tree-sitter-go/src/parser.c"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "grammars/golang/testdata/tree-sitter-go"), dir)
	assert.Equal(t, "https://github.com/tree-sitter/tree-sitter-go.git", url)
}

// The tsx grammar's directive reaches into the typescript package, so the
// answer is decided by the path rather than by the caller's directory.
func TestSubmoduleForSiblingPackage(t *testing.T) {
	root := "/repo"
	dir, _, err := submoduleFor(root, parseModules([]byte(modules)),
		filepath.Join(root, "grammars/typescript/testdata/tree-sitter-typescript/tsx/src/parser.c"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "grammars/typescript/testdata/tree-sitter-typescript"), dir)
}

// A path no submodule holds is reported rather than cloned from a guess.
func TestSubmoduleForUnknownPath(t *testing.T) {
	_, _, err := submoduleFor("/repo", parseModules([]byte(modules)), "/repo/grammars/ruby/testdata/x/parser.c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no submodule")
}

// A prefix that stops mid-name is not a holding directory.
func TestSubmoduleForNameIsNotAPrefix(t *testing.T) {
	urls := map[string]string{"vendor/lib": "https://example.com/lib.git"}
	_, _, err := submoduleFor("/repo", urls, "/repo/vendor/library/src/parser.c")
	require.Error(t, err)
}

func TestFindModulesWalksUp(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(modules), 0o644))
	deep := filepath.Join(root, "grammars", "golang")
	require.NoError(t, os.MkdirAll(deep, 0o755))

	found, contents, err := findModules(deep)
	require.NoError(t, err)
	assert.Equal(t, root, found)
	assert.Equal(t, modules, string(contents))
}

// A directory that does not exist yet is walked through: the submodule's own
// path is absent exactly when this has work to do.
func TestFindModulesThroughAbsentDirectory(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(modules), 0o644))

	found, _, err := findModules(filepath.Join(root, "grammars/golang/testdata/tree-sitter-go/src"))
	require.NoError(t, err)
	assert.Equal(t, root, found)
}

func TestFindModulesReportsAbsence(t *testing.T) {
	_, _, err := findModules(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no .gitmodules")
}

// A tree that already carries the grammar is read as it stands.
func TestReadSourceKeepsAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "parser.c")
	require.NoError(t, os.WriteFile(path, []byte("int main;"), 0o644))

	src, err := readSource(path)
	require.NoError(t, err)
	assert.Equal(t, "int main;", string(src))
}

// Cloning over a populated directory would destroy whatever is there.
func TestCloneRefusesAPopulatedDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep"), nil, 0o644))

	err := clone("https://example.com/x.git", dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not empty")
}
