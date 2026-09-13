package grammarsrc

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// modulePath is this module's import path.
const modulePath = "github.com/wow-look-at-my/go-tree-sitter"

// snippets is a line of source for each grammar the consumer parses. A grammar
// with no line here fails the test, so a new grammar cannot go unchecked.
var snippets = map[string]string{
	"bash":       "echo hi\n",
	"clang":      "int main(void) { return 0; }\n",
	"cpp":        "int main() { return 0; }\n",
	"golang":     "package main\n",
	"javascript": "let answer = 42;\n",
	"rust":       "fn main() {}\n",
	"tsx":        "let node = <div />;\n",
	"typescript": "let answer: number = 42;\n",
}

// A module another module requires arrives as the files git tracks, with each
// submodule an empty gitlink, no .git, and nothing generated. The build that
// fetched it runs each grammar package's directive in a writable copy of that
// tree and compiles the result. This does the same with no git on PATH, then
// builds a consumer against the copy and parses with every grammar.
func TestAConsumerCopyGeneratesAndParses(test *testing.T) {
	goCmd, err := exec.LookPath("go")
	require.NoError(test, err)
	work := test.TempDir()
	copyRoot := filepath.Join(work, "module")
	copyModule(test, copyRoot)
	before := treeListing(test, copyRoot)

	env := consumerEnv(goCmd)
	directives := grammarDirectives(test)
	allowed := map[string]bool{}
	for _, dir := range directives {
		run(test, copyRoot, env, goCmd, "generate", "./"+dir.pkg)
		allowed[dir.submodule(test)] = true
		allowed[dir.pkg] = true
	}

	for path := range treeListing(test, copyRoot) {
		if before[path] {
			continue
		}
		assert.True(test, underAny(path, allowed), "generate wrote %s, outside its package and its sources", path)
	}
	for _, dir := range directives {
		assert.FileExists(test, filepath.Join(copyRoot, dir.pkg, "parser.go"))
		assert.FileExists(test, filepath.Join(copyRoot, dir.pkg, "tables.zst"))
	}
	beside, err := os.ReadDir(work)
	require.NoError(test, err)
	require.Len(test, beside, 1, "generate wrote beside the module")

	app := filepath.Join(work, "consumer")
	writeConsumer(test, app, copyRoot, directives)
	run(test, app, env, goCmd, "mod", "tidy")
	out := run(test, app, env, goCmd, "run", ".")
	for _, dir := range directives {
		name := filepath.Base(dir.pkg)
		assert.Contains(test, out, name+": parsed without error\n")
	}
}

// consumerEnv is the environment a dependency's directive runs in: no git, the
// toolchain it was handed, and no generating of dependencies in turn.
func consumerEnv(goCmd string) []string {
	var env []string
	for _, pair := range os.Environ() {
		if strings.HasPrefix(pair, "PATH=") || strings.HasPrefix(pair, "GOFLAGS=") {
			continue
		}
		env = append(env, pair)
	}
	return append(env,
		"PATH="+filepath.Dir(goCmd),
		"GOFLAGS=-mod=mod",
		"GOTOOLCHAIN=local",
		"GOGENERATEDEPS=off",
		"CGO_ENABLED=0",
	)
}

func run(test *testing.T, dir string, env []string, argv ...string) string {
	test.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	test.Logf("%s (in %s):\n%s", strings.Join(argv, " "), dir, out.String())
	require.NoError(test, err, "%s", strings.Join(argv, " "))
	return out.String()
}

// copyModule copies the files git tracks into dest. A gitlink is a directory
// the module zip leaves out, so none is copied.
func copyModule(test *testing.T, dest string) {
	test.Helper()
	out, err := exec.Command("git", "-C", moduleRoot, "ls-files", "--stage", "-z").Output()
	require.NoError(test, err)
	copied := 0
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		meta, path, isRecord := strings.Cut(record, "\t")
		require.True(test, isRecord, "git ls-files printed %q", record)
		if strings.HasPrefix(meta, "160000 ") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(path)))
		require.NoError(test, err)
		target := filepath.Join(dest, filepath.FromSlash(path))
		require.NoError(test, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(test, os.WriteFile(target, body, 0o644))
		copied++
	}
	require.NotZero(test, copied)
}

// treeListing answers every path under root, relative and slash-spelled.
func treeListing(test *testing.T, root string) map[string]bool {
	test.Helper()
	paths := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = true
		return nil
	})
	require.NoError(test, err)
	return paths
}

// underAny reports whether path is one of dirs, lies under one, or is a parent
// directory that had to be made to reach one.
func underAny(path string, dirs map[string]bool) bool {
	for dir := range dirs {
		if path == dir || strings.HasPrefix(path, dir+"/") || strings.HasPrefix(dir, path+"/") {
			return true
		}
	}
	return false
}

// writeConsumer writes a module that requires this one from moduleDir and
// parses a snippet with each grammar.
func writeConsumer(test *testing.T, app, moduleDir string, directives []directive) {
	test.Helper()
	require.NoError(test, os.MkdirAll(app, 0o755))
	goMod := fmt.Sprintf("module example.com/consumer\n\ngo 1.26\n\nrequire %s v0.0.0\n\nreplace %s => %s\n",
		modulePath, modulePath, filepath.ToSlash(moduleDir))
	require.NoError(test, os.WriteFile(filepath.Join(app, "go.mod"), []byte(goMod), 0o644))
	sums, err := os.ReadFile(filepath.Join(moduleDir, "go.sum"))
	require.NoError(test, err)
	require.NoError(test, os.WriteFile(filepath.Join(app, "go.sum"), sums, 0o644))

	var names []string
	for _, dir := range directives {
		names = append(names, filepath.Base(dir.pkg))
	}
	sort.Strings(names)
	var src strings.Builder
	src.WriteString("package main\n\nimport (\n\t\"fmt\"\n\n\tts \"" + modulePath + "\"\n")
	for _, name := range names {
		fmt.Fprintf(&src, "\t%q\n", modulePath+"/grammars/"+name)
	}
	src.WriteString(")\n\nfunc parse(name string, language *ts.Language, text string) {\n")
	src.WriteString("\tparser := ts.NewParser()\n")
	src.WriteString("\tif !parser.SetLanguage(language) {\n\t\tpanic(name + \": the parser rejected the language\")\n\t}\n")
	src.WriteString("\troot := parser.ParseString(nil, []byte(text)).RootNode()\n")
	src.WriteString("\tif root.HasError() {\n\t\tpanic(name + \": \" + root.String())\n\t}\n")
	src.WriteString("\tfmt.Printf(\"%s: parsed without error\\n\", name)\n}\n\nfunc main() {\n")
	for _, name := range names {
		snippet, known := snippets[name]
		require.True(test, known, "grammars/%s has no snippet to parse", name)
		fmt.Fprintf(&src, "\tparse(%q, %s.Language(), %q)\n", name, name, snippet)
	}
	src.WriteString("}\n")
	require.NoError(test, os.WriteFile(filepath.Join(app, "main.go"), []byte(src.String()), 0o644))
}
