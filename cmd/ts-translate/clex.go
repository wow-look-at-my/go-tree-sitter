package main

import (
	"fmt"
	"strings"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokNumber
	tokChar
	tokString
	tokPunct
	tokDirective
)

type token struct {
	kind tokenKind
	text string
	val  int64
	line int
}

func (t token) String() string {
	return fmt.Sprintf("%d:%q", t.line, t.text)
}

type cScanner struct {
	src  []byte
	pos  int
	line int
}

func newScanner(src []byte) *cScanner {
	return &cScanner{src: src, line: 1}
}

func (s *cScanner) peekByte(offset int) byte {
	if s.pos+offset >= len(s.src) {
		return 0
	}
	return s.src[s.pos+offset]
}

func (s *cScanner) skipSpace() bool {
	atLineStart := s.pos == 0
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch {
		case c == '\n':
			s.line++
			s.pos++
			atLineStart = true
		case c == ' ' || c == '\t' || c == '\r':
			s.pos++
		case c == '/' && s.peekByte(1) == '/':
			for s.pos < len(s.src) && s.src[s.pos] != '\n' {
				s.pos++
			}
		case c == '/' && s.peekByte(1) == '*':
			s.pos += 2
			for s.pos < len(s.src) && !(s.src[s.pos] == '*' && s.peekByte(1) == '/') {
				if s.src[s.pos] == '\n' {
					s.line++
				}
				s.pos++
			}
			s.pos += 2
		default:
			return atLineStart
		}
	}
	return atLineStart
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// next returns the following token in the stream.
func (s *cScanner) next() token {
	atLineStart := s.skipSpace()
	if s.pos >= len(s.src) {
		return token{kind: tokEOF, line: s.line}
	}
	start := s.pos
	line := s.line
	c := s.src[s.pos]

	if c == '#' && atLineStart {
		for s.pos < len(s.src) && s.src[s.pos] != '\n' {
			if s.src[s.pos] == '\\' && s.peekByte(1) == '\n' {
				s.pos++
				s.line++
			}
			s.pos++
		}
		return token{kind: tokDirective, text: string(s.src[start:s.pos]), line: line}
	}

	if isIdentStart(c) {
		for s.pos < len(s.src) && isIdentPart(s.src[s.pos]) {
			s.pos++
		}
		return token{kind: tokIdent, text: string(s.src[start:s.pos]), line: line}
	}

	if isDigit(c) {
		for s.pos < len(s.src) && (isIdentPart(s.src[s.pos]) || s.src[s.pos] == '.') {
			s.pos++
		}
		text := string(s.src[start:s.pos])
		v, err := parseCNumber(text)
		if err != nil {
			panic(fmt.Sprintf("line %d: %v", line, err))
		}
		return token{kind: tokNumber, text: text, val: v, line: line}
	}

	if c == '\'' {
		s.pos++
		v, ok := s.scanCharBody('\'')
		if !ok {
			panic(fmt.Sprintf("line %d: unterminated character literal", line))
		}
		s.pos++
		return token{kind: tokChar, text: string(s.src[start:s.pos]), val: v, line: line}
	}

	if c == '"' {
		s.pos++
		var sb strings.Builder
		for s.pos < len(s.src) && s.src[s.pos] != '"' {
			v, ok := s.scanCharBody('"')
			if !ok {
				break
			}
			sb.WriteRune(rune(v))
		}
		s.pos++
		return token{kind: tokString, text: sb.String(), line: line}
	}

	for _, op := range threeCharOps {
		if strings.HasPrefix(string(s.src[s.pos:min(s.pos+3, len(s.src))]), op) {
			s.pos += 3
			return token{kind: tokPunct, text: op, line: line}
		}
	}
	for _, op := range twoCharOps {
		if strings.HasPrefix(string(s.src[s.pos:min(s.pos+2, len(s.src))]), op) {
			s.pos += 2
			return token{kind: tokPunct, text: op, line: line}
		}
	}
	s.pos++
	return token{kind: tokPunct, text: string(c), line: line}
}

var threeCharOps = []string{"<<=", ">>=", "..."}

var twoCharOps = []string{
	"==", "!=", "<=", ">=", "&&", "||", "<<", ">>",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "->", "++", "--",
}

// scanCharBody reads one character of a literal, resolving escapes.
func (s *cScanner) scanCharBody(quote byte) (int64, bool) {
	if s.pos >= len(s.src) {
		return 0, false
	}
	c := s.src[s.pos]
	if c == quote {
		return 0, false
	}
	if c != '\\' {
		s.pos++
		if c < 0x80 {
			return int64(c), true
		}
		r, size := decodeRune(s.src[s.pos-1:])
		s.pos += size - 1
		return int64(r), true
	}
	s.pos++
	e := s.src[s.pos]
	s.pos++
	switch e {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case '0':
		return 0, true
	case 'a':
		return 7, true
	case 'b':
		return 8, true
	case 'f':
		return 12, true
	case 'v':
		return 11, true
	case '\\':
		return '\\', true
	case '\'':
		return '\'', true
	case '"':
		return '"', true
	case '?':
		return '?', true
	case 'x', 'u', 'U':
		var v int64
		for s.pos < len(s.src) && isHex(s.src[s.pos]) {
			v = v*16 + int64(hexValue(s.src[s.pos]))
			s.pos++
		}
		return v, true
	}
	return int64(e), true
}

func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

func parseCNumber(text string) (int64, error) {
	t := strings.TrimRight(text, "uUlL")
	var v int64
	switch {
	case strings.HasPrefix(t, "0x"), strings.HasPrefix(t, "0X"):
		for _, c := range []byte(t[2:]) {
			if !isHex(c) {
				return 0, fmt.Errorf("bad hex literal %q", text)
			}
			v = v*16 + int64(hexValue(c))
		}
	case len(t) > 1 && t[0] == '0':
		for _, c := range []byte(t[1:]) {
			if c < '0' || c > '7' {
				return 0, fmt.Errorf("bad octal literal %q", text)
			}
			v = v*8 + int64(c-'0')
		}
	default:
		for _, c := range []byte(t) {
			if !isDigit(c) {
				return 0, fmt.Errorf("bad decimal literal %q", text)
			}
			v = v*10 + int64(c-'0')
		}
	}
	return v, nil
}

func decodeRune(b []byte) (int32, int) {
	if len(b) == 0 {
		return 0, 0
	}
	c := b[0]
	switch {
	case c < 0x80:
		return int32(c), 1
	case c&0xE0 == 0xC0 && len(b) >= 2:
		return int32(c&0x1F)<<6 | int32(b[1]&0x3F), 2
	case c&0xF0 == 0xE0 && len(b) >= 3:
		return int32(c&0x0F)<<12 | int32(b[1]&0x3F)<<6 | int32(b[2]&0x3F), 3
	case c&0xF8 == 0xF0 && len(b) >= 4:
		return int32(c&0x07)<<18 | int32(b[1]&0x3F)<<12 | int32(b[2]&0x3F)<<6 | int32(b[3]&0x3F), 4
	}
	return int32(c), 1
}

// tokenize turns a C source file into a token slice.
func tokenize(src []byte) []token {
	s := newScanner(src)
	var out []token
	for {
		t := s.next()
		out = append(out, t)
		if t.kind == tokEOF {
			return out
		}
	}
}
