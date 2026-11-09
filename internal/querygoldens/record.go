// record.go holds Recorder.

package querygoldens

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Recorder runs the reference CLI and writes the goldens of one grammar.
type Recorder struct {
	// CLI is the tree-sitter executable.
	CLI string
	// Root is the repository root.
	Root string
	// Log receives one line per golden written.
	Log io.Writer
}

// Record replaces every query golden of a grammar. It writes the inputs to a
// temporary directory and runs the CLI there, so the name lines are the
// stable section names.
func (r Recorder) Record(g Grammar) error {
	dir := filepath.Join(r.Root, g.Package)
	queries, err := Queries(g, dir)
	if err != nil {
		return err
	}
	inputs, err := Inputs(g, dir)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "ts-goldens-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	var names []string
	for _, in := range inputs {
		for _, s := range in.Sections {
			p := filepath.Join(work, filepath.FromSlash(s.Name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(p, s.Source, 0o644); err != nil {
				return err
			}
			names = append(names, s.Name)
		}
	}

	goldens := filepath.Join(dir, GoldensDir)
	if err := os.RemoveAll(goldens); err != nil {
		return err
	}
	grammar, err := filepath.Abs(filepath.Join(dir, g.CLIGrammar))
	if err != nil {
		return err
	}
	for _, q := range queries {
		qpath, err := filepath.Abs(filepath.Join(dir, q.Path))
		if err != nil {
			return err
		}
		if err := r.recordQuery(work, grammar, qpath, filepath.Join(goldens, q.Stem), names, inputs); err != nil {
			return fmt.Errorf("%s: %w", q.Path, err)
		}
	}
	return nil
}

func (r Recorder) recordQuery(work, grammar, query, out string, names []string, inputs []Input) error {
	for _, mode := range Modes {
		args := []string{"query", "-p", grammar}
		if mode == Captures {
			args = append(args, "-c")
		}
		args = append(append(args, query), names...)
		cmd := exec.Command(r.CLI, args...)
		cmd.Dir = work
		cmd.Env = append(os.Environ(), "RUST_BACKTRACE=0", "RUST_LIB_BACKTRACE=0")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		runErr := cmd.Run()
		if runErr != nil {
			if stdout.Len() > 0 || mode != Matches {
				return fmt.Errorf("%s: %v\n%s", mode, runErr, stderr.String())
			}
			text, err := CLIError(stderr.String())
			if err != nil {
				return err
			}
			return r.write(filepath.Join(out, "compile.golden"), []byte(text))
		}
		if mode == Matches {
			if err := r.write(filepath.Join(out, "compile.golden"), []byte(Accepted)); err != nil {
				return err
			}
		}
		parts, err := Split(stdout.Bytes(), names)
		if err != nil {
			return fmt.Errorf("%s: %w", mode, err)
		}
		for _, in := range inputs {
			var golden []byte
			for range in.Sections {
				golden = append(golden, parts[0]...)
				parts = parts[1:]
			}
			if err := r.write(filepath.Join(out, string(mode), in.Golden+".golden"), golden); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r Recorder) write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if r.Log != nil {
		fmt.Fprintf(r.Log, "wrote %s (%d bytes)\n", path, len(data))
	}
	return nil
}

// Split cuts CLI stdout into one part per name. Each part starts with its name
// line. A result line is indented, so a name line cannot be confused with one.
func Split(stdout []byte, names []string) ([][]byte, error) {
	text := string(stdout)
	parts := make([][]byte, 0, len(names))
	for i, name := range names {
		if !strings.HasPrefix(text, name+"\n") {
			return nil, fmt.Errorf("output does not continue with the name line %q", name)
		}
		end := len(text)
		if i+1 < len(names) {
			next := strings.Index(text, "\n"+names[i+1]+"\n")
			if next < 0 {
				return nil, fmt.Errorf("output has no name line %q", names[i+1])
			}
			end = next + 1
		}
		parts = append(parts, []byte(text[:end]))
		text = text[end:]
	}
	if text != "" {
		return nil, fmt.Errorf("output continues past the last name: %q", text)
	}
	return parts, nil
}
