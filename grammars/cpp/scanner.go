package cpp

import (
	"encoding/binary"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// This file is a hand port of tree-sitter-cpp's src/scanner.c. It recognizes
// the two tokens the parse table cannot: the delimiter and the content of a
// raw string literal, R"delim(content)delim".

// The C++ standard caps a raw string delimiter at this many characters.
const maxDelimiterLength = 16

// The external token indices, in the order src/scanner.c declares them.
const (
	rawStringDelimiter = iota
	rawStringContent
)

// wcharSize is what the C scanner writes per delimiter character. It stores a
// wchar_t, which is 4 bytes on every platform tree-sitter builds this scanner
// for, so a serialized state is byte-identical to the C one.
const wcharSize = 4

type cppScanner struct {
	delimiter []int32
}

func (s *cppScanner) reset() { s.delimiter = s.delimiter[:0] }

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Create makes the state one parse carries.
func (scanner) Create() any { return &cppScanner{} }

// Destroy releases the state. Go collects it, so nothing happens here.
func (scanner) Destroy(any) {}

// Scan reads one external token.
func (scanner) Scan(payload any, lexer *ts.Lexer, validSymbols []bool) bool {
	s := payload.(*cppScanner)

	if validSymbols[rawStringDelimiter] && validSymbols[rawStringContent] {
		// Both valid at once means the parser is recovering from an error.
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
		// The closing delimiter must match the opening one exactly. Stopping
		// at a quote instead would break R"""hello""", which is valid.
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
			// An empty delimiter gets no token. The grammar then falls back to
			// its delimiter-less rule.
			return len(s.delimiter) > 0
		}
		s.delimiter = append(s.delimiter, lexer.Lookahead)
		lexer.Advance(false)
	}
}

// scanContent reads the content of R"delim(content)delim".
func (s *cppScanner) scanContent(lexer *ts.Lexer) bool {
	// How far the closing delimiter has matched since the last close paren. A
	// delimiter cannot contain a close paren, so one counter is enough.
	delimiterIndex := -1
	for {
		// End of input terminates the content. That leaves an incomplete raw
		// string literal, which models the code well.
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
			// The content stops before the closing )delim" run. The scanner
			// still reads through that run, outside the token.
			lexer.MarkEnd()
			delimiterIndex = 0
		}

		lexer.Advance(false)
	}
}

// Serialize writes the delimiter so an incremental reparse can resume inside a
// raw string. Each character takes four bytes, little endian, which is what the
// C scanner writes.
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

// isSpace matches the C library's iswspace under the C locale, which is what
// the scanner runs under. Only these characters count.
func isSpace(ch int32) bool {
	switch ch {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}
