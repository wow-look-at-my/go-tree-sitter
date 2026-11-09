// inputs.go holds Section.

package querygoldens

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/wow-look-at-my/go-tree-sitter/internal/corpus"
)

// Section is one source file of an input. Name is the path the CLI prints,
// relative to the directory the recorder runs it in.
type Section struct {
	Name   string
	Source []byte
}

// Input is the set of sources one golden file covers. Golden is the golden's
// path under <stem>/<mode>/, without the .golden suffix.
type Input struct {
	Golden   string
	Sections []Section
}

// Query is one .scm file a grammar runs.
type Query struct {
	Stem string
	Path string
}

// Queries lists the .scm files of a grammar, sorted by stem. dir is the
// grammar package directory. A missing directory is an error.
func Queries(g Grammar, dir string) ([]Query, error) {
	var queries []Query
	for _, qdir := range g.Queries {
		entries, err := os.ReadDir(filepath.Join(dir, qdir))
		if err != nil {
			return nil, fmt.Errorf("query directory (is the submodule checked out?): %w", err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".scm") {
				continue
			}
			stem := strings.TrimSuffix(name, ".scm")
			if slices.ContainsFunc(queries, func(q Query) bool { return q.Stem == stem }) {
				return nil, fmt.Errorf("two queries share the stem %q", stem)
			}
			queries = append(queries, Query{Stem: stem, Path: filepath.Join(qdir, name)})
		}
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no .scm files under %v", g.Queries)
	}
	sort.Slice(queries, func(i, j int) bool { return queries[i].Stem < queries[j].Stem })
	return queries, nil
}

// Inputs reads every source a grammar's queries run against. dir is the
// grammar package directory. A configured directory that is missing or empty
// is an error, so a checkout without submodules fails.
func Inputs(g Grammar, dir string) ([]Input, error) {
	var inputs []Input
	if g.Samples != "" {
		samples, err := readFiles(filepath.Join(dir, g.Samples), "samples", func(name string, _ int64) bool {
			return strings.HasSuffix(name, ".txt")
		})
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, samples...)
	}
	if g.Examples != "" {
		examples, err := readFiles(filepath.Join(dir, g.Examples), "examples", func(_ string, size int64) bool {
			return size <= MaxExampleBytes
		})
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, examples...)
	}
	cases, err := corpusInputs(g, filepath.Join(dir, g.Corpus))
	if err != nil {
		return nil, err
	}
	return append(inputs, cases...), nil
}

// readFiles reads the files of a directory, one input each.
func readFiles(dir, prefix string, keep func(name string, size int64) bool) ([]Input, error) {
	var inputs []Input
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !keep(rel, info.Size()) {
			return nil
		}
		source, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name := path.Join(prefix, rel)
		inputs = append(inputs, Input{Golden: name, Sections: []Section{{Name: name, Source: source}}})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no source files under %s", dir)
	}
	return inputs, nil
}

// corpusInputs returns one input per corpus file. A section is a case that the
// corpus run of this grammar parses. Its index counts every case in the file.
func corpusInputs(g Grammar, dir string) ([]Input, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("corpus directory (is the submodule checked out?): %w", err)
	}
	cases, err := corpus.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var inputs []Input
	index := 0
	for _, c := range cases {
		golden := "corpus/" + c.File
		if len(inputs) == 0 || inputs[len(inputs)-1].Golden != golden {
			inputs = append(inputs, Input{Golden: golden})
			index = 0
		}
		name := fmt.Sprintf("%s/%03d", golden, index)
		index++
		if c.Expect == corpus.ExpectSkip || !c.Platform || (c.Language != "" && c.Language != g.Name) {
			continue
		}
		last := &inputs[len(inputs)-1]
		last.Sections = append(last.Sections, Section{Name: name, Source: c.Input})
	}
	kept := inputs[:0]
	for _, in := range inputs {
		if len(in.Sections) > 0 {
			kept = append(kept, in)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("no corpus cases under %s", dir)
	}
	return kept, nil
}
