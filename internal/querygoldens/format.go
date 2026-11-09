// format.go holds Mode.

package querygoldens

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Mode is how the CLI walks the results: by match, or by capture with -c.
type Mode string

// Both output modes of `tree-sitter query`.
const (
	Matches  Mode = "matches"
	Captures Mode = "captures"
)

// Modes lists both modes in the order the recorder runs them.
var Modes = []Mode{Matches, Captures}

// Render writes what `tree-sitter query` prints to stdout for each section,
// in crates/cli/src/query.rs format. It also reports whether any section
// exceeded the match limit, which the CLI only logs to stderr.
func Render(language *ts.Language, query *ts.Query, sections []Section, mode Mode) ([]byte, bool, error) {
	var out bytes.Buffer
	exceeded := false
	for _, section := range sections {
		fmt.Fprintf(&out, "%s\n", section.Name)
		parser := ts.NewParser()
		if !parser.SetLanguage(language) {
			return nil, false, fmt.Errorf("the parser refused the language")
		}
		tree := parser.ParseString(nil, section.Source)
		if tree == nil {
			return nil, false, fmt.Errorf("%s: the parser returned no tree", section.Name)
		}
		cursor := ts.NewQueryCursor()
		source := section.Source
		if mode == Captures {
			for match, index := range cursor.Captures(query, tree.RootNode(), source) {
				capture := match.Captures[index]
				fmt.Fprintf(&out, "    pattern: %2d, capture: %d - %s, start: %s, end: %s, text: `%s`\n",
					match.PatternIndex, capture.Index, query.CaptureNameForID(capture.Index),
					point(capture.Node.StartPoint()), point(capture.Node.EndPoint()), text(capture.Node, source))
			}
		} else {
			for match := range cursor.Matches(query, tree.RootNode(), source) {
				fmt.Fprintf(&out, "  pattern: %d\n", match.PatternIndex)
				for _, capture := range match.Captures {
					start, end := capture.Node.StartPoint(), capture.Node.EndPoint()
					name := query.CaptureNameForID(capture.Index)
					if start.Row == end.Row {
						fmt.Fprintf(&out, "    capture: %d - %s, start: %s, end: %s, text: `%s`\n",
							capture.Index, name, point(start), point(end), text(capture.Node, source))
					} else {
						fmt.Fprintf(&out, "    capture: %s, start: %s, end: %s\n", name, point(start), point(end))
					}
				}
			}
		}
		exceeded = exceeded || cursor.DidExceedMatchLimit()
	}
	return out.Bytes(), exceeded, nil
}

// point formats a point as the Rust binding's Display does.
func point(p ts.Point) string {
	return fmt.Sprintf("(%d, %d)", p.Row, p.Column)
}

// text is utf8_text(...).unwrap_or(""): invalid UTF-8 prints as nothing.
func text(node ts.Node, source []byte) []byte {
	b := source[node.StartByte():node.EndByte()]
	if !utf8.Valid(b) {
		return nil
	}
	return b
}
