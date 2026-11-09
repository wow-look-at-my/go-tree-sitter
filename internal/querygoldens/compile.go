// compile.go holds Accepted.

package querygoldens

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Accepted is the compile golden of a query the CLI compiled.
const Accepted = "accepted\n"

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// CLIError extracts the QueryError text from the stderr of a failed
// `tree-sitter query`: the "Caused by:" block, without its indent.
func CLIError(stderr string) (string, error) {
	stderr = ansi.ReplaceAllString(stderr, "")
	if !strings.Contains(stderr, "Query compilation failed") {
		return "", fmt.Errorf("stderr does not report a query compile failure:\n%s", stderr)
	}
	_, cause, ok := strings.Cut(stderr, "\nCaused by:\n")
	if !ok {
		return "", fmt.Errorf("stderr has no cause:\n%s", stderr)
	}
	cause, _, _ = strings.Cut(cause, "\nStack backtrace:")
	lines := strings.Split(strings.TrimRight(cause, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "    ")
	}
	return strings.Join(lines, "\n") + "\n", nil
}

var errorHead = regexp.MustCompile(`^Query error at (\d+):(\d+)\. (Invalid node type |Invalid field name |Invalid capture name |Invalid predicate: |Impossible pattern:\n|Invalid syntax:\n)`)

var kindPhrases = map[string]ts.QueryErrorKind{
	"Invalid node type ":    ts.QueryErrorNodeType,
	"Invalid field name ":   ts.QueryErrorField,
	"Invalid capture name ": ts.QueryErrorCapture,
	"Invalid predicate: ":   ts.QueryErrorPredicate,
	"Impossible pattern:\n": ts.QueryErrorStructure,
	"Invalid syntax:\n":     ts.QueryErrorSyntax,
}

// Expected is what a compile golden demands of the Go compile.
type Expected struct {
	Accepted bool
	Kind     ts.QueryErrorKind
	Row      int
	Column   int
	// Offset is the byte offset the CLI's row and column name.
	Offset int
	// Text is QueryError's Display, as the CLI printed it.
	Text string
}

// ParseCompileGolden reads a compile golden for a query source.
func ParseCompileGolden(golden, source string) (Expected, error) {
	if golden == Accepted {
		return Expected{Accepted: true}, nil
	}
	m := errorHead.FindStringSubmatch(golden)
	if m == nil {
		return Expected{}, fmt.Errorf("compile golden is neither %q nor a query error: %q", Accepted, golden)
	}
	row, _ := strconv.Atoi(m[1])
	column, _ := strconv.Atoi(m[2])
	e := Expected{Kind: kindPhrases[m[3]], Row: row - 1, Column: column - 1, Text: strings.TrimSuffix(golden, "\n")}
	e.Offset = offsetOf(source, e.Row, e.Column)
	return e, nil
}

// offsetOf inverts the row and column walk of Query::new_raw, which steps over
// str::lines(). A line drops its "\n" and a trailing "\r", and counts as its
// length plus one.
func offsetOf(source string, row, column int) int {
	start := 0
	rest := source
	for r := 0; r < row && rest != ""; r++ {
		line, after, found := strings.Cut(rest, "\n")
		if !found {
			after = ""
		}
		start += len(strings.TrimSuffix(line, "\r")) + 1
		rest = after
	}
	return start + column
}

// CheckCompile compares a Go compile result with a compile golden. It returns
// nil when they agree.
func CheckCompile(expected Expected, err error) error {
	if expected.Accepted {
		if err != nil {
			return fmt.Errorf("the CLI compiled the query, Go refused it: %v", err)
		}
		return nil
	}
	if err == nil {
		return fmt.Errorf("the CLI refused the query, Go compiled it\n want: %s", expected.Text)
	}
	var qerr *ts.QueryError
	if !errors.As(err, &qerr) {
		return fmt.Errorf("the error is not a *QueryError: %v", err)
	}
	var problems []string
	if qerr.Kind != expected.Kind {
		problems = append(problems, fmt.Sprintf("kind %v, want %v", qerr.Kind, expected.Kind))
	}
	if qerr.Row != expected.Row || qerr.Column != expected.Column {
		problems = append(problems, fmt.Sprintf("row:column %d:%d, want %d:%d",
			qerr.Row, qerr.Column, expected.Row, expected.Column))
	}
	if qerr.Offset != expected.Offset {
		problems = append(problems, fmt.Sprintf("offset %d, want %d", qerr.Offset, expected.Offset))
	}
	if qerr.Error() != expected.Text {
		problems = append(problems, fmt.Sprintf("text %q, want %q", qerr.Error(), expected.Text))
	}
	if len(problems) > 0 {
		return fmt.Errorf("the compile error differs from the CLI: %s", strings.Join(problems, "; "))
	}
	return nil
}
