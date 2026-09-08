// Package corpus reads the test corpus format that every tree-sitter grammar
// repository ships under test/corpus. The rules here are a port of the upstream
// CLI's own reader in crates/cli/src/test.rs, so a case that upstream accepts is
// read the same way.
package corpus

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Expectation says what a case demands of the parse.
type Expectation int

// The expectations a case header can declare.
const (
	// ExpectPass compares the rendered tree against the expected tree.
	ExpectPass Expectation = iota
	// ExpectSkip marks a case upstream itself does not run.
	ExpectSkip
	// ExpectError demands only that the tree contains an error.
	ExpectError
)

// Case is a corpus entry: a source text and the tree it must produce.
type Case struct {
	// Name is the header text.
	Name string
	// File is the corpus file the case came from.
	File string
	// Input is the source text, with its trailing newline removed.
	Input []byte
	// Output is the expected tree, normalized onto a single line.
	Output string
	// HasFields is true when the expected tree names field names.
	HasFields bool
	// Expect says whether the case must pass, must error, or is skipped.
	Expect Expectation
	// Platform is false when a header restricts the case to another system.
	Platform bool
}

// ReadDir reads every .txt file under a directory as a corpus file. The walk
// recurses, because a grammar can group its corpus into subdirectories and
// upstream runs those too.
func ReadDir(dir string) ([]Case, error) {
	var names []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".txt") {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	var cases []Case
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		label, err := filepath.Rel(dir, name)
		if err != nil {
			label = filepath.Base(name)
		}
		cases = append(cases, Parse(filepath.ToSlash(label), string(data))...)
	}
	return cases, nil
}

// Parse reads a corpus file.
func Parse(file, content string) []Case {
	lines := splitInclusive(content)
	first, found := firstSuffix(lines)

	var cases []Case
	var pending *header
	lineNum := 0
	for lineNum < len(lines) {
		next, bodyStart := parseHeader(lines, first, found, lineNum)
		if next == nil {
			lineNum++
			continue
		}
		opening := lineNum
		lineNum = bodyStart
		if pending != nil {
			if c, ok := buildCase(lines[pending.bodyStart:opening], first, found, pending, file); ok {
				cases = append(cases, c)
			}
		}
		pending = next
	}
	if pending != nil {
		if c, ok := buildCase(lines[pending.bodyStart:], first, found, pending, file); ok {
			cases = append(cases, c)
		}
	}
	return cases
}

// splitInclusive splits on newlines and keeps each terminator, matching Rust's
// split_inclusive.
func splitInclusive(content string) []string {
	var lines []string
	for len(content) > 0 {
		i := strings.IndexByte(content, '\n')
		if i < 0 {
			lines = append(lines, content)
			break
		}
		lines = append(lines, content[:i+1])
		content = content[i+1:]
	}
	return lines
}

func firstSuffix(lines []string) (suffix string, found bool) {
	for _, line := range lines {
		if _, s, ok := parseDelimiterLine(line, '='); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// parseDelimiterLine reports whether a line opens with a long enough run of c,
// and returns the text after that run.
func parseDelimiterLine(line string, c byte) (delimLen int, suffix string, ok bool) {
	for delimLen < len(line) && line[delimLen] == c {
		delimLen++
	}
	if delimLen < 3 {
		return 0, "", false
	}
	return delimLen, strings.TrimRight(line[delimLen:], "\r\n"), true
}

func suffixMatches(first string, firstFound bool, suffix string) bool {
	if !firstFound {
		return suffix == ""
	}
	return suffix != "" && first == suffix
}

type header struct {
	name      string
	expect    Expectation
	platform  bool
	bodyStart int
}

// parseHeader reads a header block that starts at lines[start], and returns the
// index where the body begins.
func parseHeader(lines []string, first string, firstFound bool, start int) (*header, int) {
	_, suffix, ok := parseDelimiterLine(lines[start], '=')
	if !ok || !suffixMatches(first, firstFound, suffix) {
		return nil, 0
	}

	var name strings.Builder
	seenMarker, seenSkip, seenError := false, false, false
	platformSet, platformOK := false, false

	lineNum := start + 1
	for lineNum < len(lines) {
		if _, closing, ok := parseDelimiterLine(lines[lineNum], '='); ok &&
			suffixMatches(first, firstFound, closing) {
			break
		}
		trimmed := strings.TrimSpace(lines[lineNum])
		if trimmed == "" && !seenMarker {
			return nil, 0
		}
		head, _, _ := strings.Cut(trimmed, "(")
		switch head {
		case ":skip":
			seenMarker, seenSkip = true, true
		case ":error":
			seenMarker, seenError = true, true
		case ":fail-fast", ":cst":
			seenMarker = true
		case ":language":
			if _, ok := attributeArgument(trimmed, "language"); ok {
				seenMarker = true
			}
		case ":platform":
			if inner, ok := attributeArgument(trimmed, "platform"); ok {
				seenMarker = true
				platformSet = true
				platformOK = platformOK || strings.TrimSpace(inner) == "linux"
			}
		default:
			if !seenMarker {
				name.WriteString(lines[lineNum])
			}
		}
		lineNum++
	}
	if lineNum >= len(lines) {
		return nil, 0
	}

	expect := ExpectPass
	switch {
	case seenSkip:
		expect = ExpectSkip
	case seenError:
		expect = ExpectError
	}
	platform := true
	if platformSet {
		platform = platformOK
	}
	return &header{
		name:      strings.TrimRight(name.String(), " \t\r\n"),
		expect:    expect,
		platform:  platform,
		bodyStart: lineNum + 1,
	}, lineNum + 1
}

func attributeArgument(trimmed, key string) (string, bool) {
	rest, ok := strings.CutPrefix(trimmed, ":"+key+"(")
	if !ok {
		return "", false
	}
	inner, ok := strings.CutSuffix(rest, ")")
	return inner, ok
}

// buildCase splits a body on its longest --- divider.
func buildCase(body []string, first string, firstFound bool, h *header, file string) (Case, bool) {
	dividerLine, bestTotal, found := 0, 0, false
	for j, line := range body {
		delimLen, suffix, ok := parseDelimiterLine(line, '-')
		if !ok || !suffixMatches(first, firstFound, suffix) {
			continue
		}
		// On a tie the later candidate wins: an earlier run of the same length
		// is a literal inside the source.
		if total := delimLen + len(suffix); total >= bestTotal {
			dividerLine, bestTotal, found = j, total, true
		}
	}
	if !found {
		return Case{}, false
	}

	input := strings.Join(body[:dividerLine], "")
	input = strings.TrimSuffix(input, "\n")
	input = strings.TrimSuffix(input, "\r")

	output, hasFields := NormalizeSexp(strings.Join(body[dividerLine+1:], ""))
	return Case{
		Name:      h.name,
		File:      file,
		Input:     []byte(input),
		Output:    output,
		HasFields: hasFields,
		Expect:    h.expect,
		Platform:  h.platform,
	}, true
}

// NormalizeSexp collapses an expected tree onto a single line and reports
// whether it names any field.
func NormalizeSexp(raw string) (string, bool) {
	var result []byte
	prevWasSpace := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), ";") {
			continue
		}
		for _, ch := range line {
			if ch == ' ' || ch == '\t' || ch == '\v' || ch == '\f' {
				if !prevWasSpace && len(result) > 0 {
					result = append(result, ' ')
					prevWasSpace = true
				}
				continue
			}
			if ch == ')' && prevWasSpace {
				result = result[:len(result)-1]
			}
			result = append(result, string(ch)...)
			prevWasSpace = false
		}
		if len(result) > 0 && !prevWasSpace {
			result = append(result, ' ')
			prevWasSpace = true
		}
	}
	out := strings.TrimRight(string(result), " \t\n")
	return out, strings.Contains(out, ": (")
}

// StripFields removes the field names from a rendered tree, for comparison
// against an expected tree that names none.
func StripFields(sexp string) string {
	var result strings.Builder
	remaining := sexp
	for {
		pos := strings.Index(remaining, ": (")
		if pos < 0 {
			break
		}
		if spacePos := strings.LastIndex(remaining[:pos], " "); spacePos >= 0 {
			word := remaining[spacePos+1 : pos]
			if word != "" && isFieldName(word) {
				result.WriteString(remaining[:spacePos+1])
				result.WriteByte('(')
				remaining = remaining[pos+3:]
				continue
			}
		}
		result.WriteString(remaining[:pos+3])
		remaining = remaining[pos+3:]
	}
	result.WriteString(remaining)
	return result.String()
}

func isFieldName(word string) bool {
	for i := 0; i < len(word); i++ {
		b := word[i]
		alnum := b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
		if !alnum && b != '_' {
			return false
		}
	}
	return true
}
