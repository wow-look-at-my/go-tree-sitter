package cpp

import (
	"encoding/binary"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// maxDelimiterLength is the C++ standard's cap on a raw string delimiter.
const maxDelimiterLength = 16

// External token indices, in the order src/scanner.c declares them.
const (
	rawStringDelimiter = iota
	rawStringContent
)

// wcharSize is the width of the wchar_t the C scanner serializes.
const wcharSize = 4

type cppScanner struct {
	delimiter []int32
}

func (s *cppScanner) reset() { s.delimiter = s.delimiter[:0] }

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Create makes the state a parse carries.
func (scanner) Create() any { return &cppScanner{} }

// Destroy releases the state. Go collects it, so nothing happens here.
func (scanner) Destroy(any) {}

// Scan reads an external token.
func (scanner) Scan(payload any, lexer *ts.Lexer, validSymbols []bool) bool {
	s := payload.(*cppScanner)

	// Both tokens valid together means the parser is recovering from an error.
	if validSymbols[rawStringDelimiter] && validSymbols[rawStringContent] {
		return false
	}

	// Leading whitespace is not skipped. Raw string syntax is space-sensitive.
	if validSymbols[rawStringDelimiter] {
		lexer.ResultSymbol = rawStringDelimiter
		return s.scanDelimiter(lexer)
	}
	if validSymbols[rawStringContent] {
		lexer.ResultSymbol = rawStringContent
		return s.scanContent(lexer)
	}
	return false
}

// scanDelimiter reads the delimiter of R"delim(content)delim".
func (s *cppScanner) scanDelimiter(lexer *ts.Lexer) bool {
	if len(s.delimiter) > 0 {
		// The closing delimiter must match the opening delimiter exactly.
		// Stopping at a quote would break R"""hello""", which is valid.
		for _, want := range s.delimiter {
			if lexer.Lookahead != want {
				return false
			}
			lexer.Advance(false)
		}
		s.reset()
		return true
	}

	// The opening delimiter runs up to the open paren. A d-char is any basic
	// character other than a paren, a backslash, and a space.
	for {
		if len(s.delimiter) >= maxDelimiterLength || lexer.EOF() ||
			lexer.Lookahead == '\\' || isSpace(lexer.Lookahead) {
			return false
		}
		if lexer.Lookahead == '(' {
			// An empty delimiter gets no token, so the grammar falls back.
			return len(s.delimiter) > 0
		}
		s.delimiter = append(s.delimiter, lexer.Lookahead)
		lexer.Advance(false)
	}
}

// scanContent reads the content of R"delim(content)delim".
func (s *cppScanner) scanContent(lexer *ts.Lexer) bool {
	// How far the closing delimiter has matched since the last close paren. A
	// delimiter cannot contain a close paren, so this counter is enough. It
	// holds a negative value while no match is in progress.
	delimiterIndex := -1
	for {
		// End of input terminates the content, leaving an incomplete literal.
		if lexer.EOF() {
			lexer.MarkEnd()
			return true
		}

		if delimiterIndex >= 0 {
			switch {
			case delimiterIndex == len(s.delimiter):
				if lexer.Lookahead == '"' {
					return true
				}
				delimiterIndex = -1
			case lexer.Lookahead == s.delimiter[delimiterIndex]:
				delimiterIndex++
			default:
				delimiterIndex = -1
			}
		}

		if delimiterIndex == -1 && lexer.Lookahead == ')' {
			// The content stops before the closing run, which the scanner
			// still reads through, outside the token.
			lexer.MarkEnd()
			delimiterIndex = 0
		}

		lexer.Advance(false)
	}
}

// Serialize writes the delimiter, little endian, so an incremental reparse can
// resume inside a raw string.
func (scanner) Serialize(payload any, buffer []byte) uint32 {
	s := payload.(*cppScanner)
	size := len(s.delimiter) * wcharSize
	if size > len(buffer) {
		return 0
	}
	for i, ch := range s.delimiter {
		binary.LittleEndian.PutUint32(buffer[i*wcharSize:], uint32(ch))
	}
	return uint32(size)
}

// Deserialize reads back what Serialize wrote.
func (scanner) Deserialize(payload any, buffer []byte) {
	s := payload.(*cppScanner)
	s.reset()
	for i := 0; i+wcharSize <= len(buffer); i += wcharSize {
		s.delimiter = append(s.delimiter, int32(binary.LittleEndian.Uint32(buffer[i:])))
	}
}

// isSpace matches iswspace under the C locale, which the scanner runs under.
func isSpace(ch int32) bool {
	switch ch {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}
