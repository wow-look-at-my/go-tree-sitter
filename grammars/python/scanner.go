package python

import (
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Hand port of tree-sitter-python's src/scanner.c. Indentation is the reason it
// exists: a block opens and closes on column width, which no parse table can
// express. The scanner keeps a stack of the widths that are open, and emits
// INDENT and DEDENT against it. String delimiters ride the same scanner,
// because an f-string body ends on the quote that opened it.

// External token indices, in the order src/scanner.c declares them.
const (
	newline = iota
	indent
	dedent
	stringStart
	stringContent
	escapeInterpolation
	stringEnd
	comment
	closeParen
	closeBracket
	closeBrace
	except
)

// comment has no branch of its own. The constant keeps the token order right.
var _ = comment

// Delimiter flags, as src/scanner.c declares them. They pack into one byte,
// because the serialized form writes one byte per open delimiter.
const (
	flagSingleQuote = 1 << iota
	flagDoubleQuote
	flagBackQuote
	flagRaw
	flagFormat
	flagTriple
	flagBytes
)

// delimiter is one open string, and the flags that say how it ends.
type delimiter uint8

func (d delimiter) isFormat() bool { return d&flagFormat != 0 }
func (d delimiter) isRaw() bool    { return d&flagRaw != 0 }
func (d delimiter) isTriple() bool { return d&flagTriple != 0 }
func (d delimiter) isBytes() bool  { return d&flagBytes != 0 }

// endCharacter is the quote that closes this string, and 0 when the flags name
// no quote at all.
func (d delimiter) endCharacter() int32 {
	switch {
	case d&flagSingleQuote != 0:
		return '\''
	case d&flagDoubleQuote != 0:
		return '"'
	case d&flagBackQuote != 0:
		return '`'
	}
	return 0
}

// setEndCharacter records the quote that opened the string.
func (d *delimiter) setEndCharacter(ch int32) {
	switch ch {
	case '\'':
		*d |= flagSingleQuote
	case '"':
		*d |= flagDoubleQuote
	case '`':
		*d |= flagBackQuote
	}
}

type pythonScanner struct {
	// indents is the stack of open column widths. It always carries a 0 at the
	// bottom, so a dedent to column 0 has something to pop back to.
	indents []uint16
	// delimiters is the stack of strings currently open. An f-string holds an
	// expression that holds another string, so it is a stack rather than one
	// value.
	delimiters []delimiter
	// insideInterpolatedString suppresses a dedent inside an f-string, where a
	// line break is layout rather than block structure.
	insideInterpolatedString bool
}

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Create makes the state a parse carries. It seeds the indent stack the way the
// C scanner does, by deserializing an empty state.
func (scanner) Create() any {
	s := &pythonScanner{}
	s.reset()
	return s
}

// Destroy releases the state. Go collects it, so nothing happens here.
func (scanner) Destroy(any) {}

// Scan reads an external token.
func (scanner) Scan(payload any, lexer *ts.Lexer, valid []bool) bool {
	return payload.(*pythonScanner).scan(lexer, valid)
}

// Serialize writes the open strings and the indent stack, so an incremental
// reparse resumes inside the block and the string it left off in.
func (scanner) Serialize(payload any, buffer []byte) uint32 {
	s := payload.(*pythonScanner)
	size := 0
	buffer[size] = boolByte(s.insideInterpolatedString)
	size++

	count := len(s.delimiters)
	if count > 0xFF {
		count = 0xFF
	}
	buffer[size] = byte(count)
	size++
	for i := 0; i < count; i++ {
		buffer[size+i] = byte(s.delimiters[i])
	}
	size += count

	// The bottom of the indent stack is always 0, and Deserialize puts it back.
	// Writing it would cost two bytes per reparse and say nothing.
	for i := 1; i < len(s.indents) && size+1 < len(buffer); i++ {
		buffer[size] = byte(s.indents[i] & 0xFF)
		buffer[size+1] = byte(s.indents[i] >> 8)
		size += 2
	}
	return uint32(size)
}

// Deserialize reads back what Serialize wrote.
func (scanner) Deserialize(payload any, buffer []byte) {
	s := payload.(*pythonScanner)
	s.reset()
	if len(buffer) == 0 {
		return
	}

	size := 0
	s.insideInterpolatedString = buffer[size] != 0
	size++

	count := int(buffer[size])
	size++
	for i := 0; i < count && size < len(buffer); i++ {
		s.delimiters = append(s.delimiters, delimiter(buffer[size]))
		size++
	}

	for ; size+1 < len(buffer); size += 2 {
		s.indents = append(s.indents, uint16(buffer[size])|uint16(buffer[size+1])<<8)
	}
}

// reset returns the scanner to the state a fresh parse starts in.
func (s *pythonScanner) reset() {
	s.delimiters = s.delimiters[:0]
	s.indents = append(s.indents[:0], 0)
	s.insideInterpolatedString = false
}

func (s *pythonScanner) scan(lexer *ts.Lexer, valid []bool) bool {
	// The parser offers both of these together only while it recovers from an
	// error, where the scanner cannot tell a string body from block structure.
	errorRecoveryMode := valid[stringContent] && valid[indent]
	withinBrackets := valid[closeBrace] || valid[closeParen] || valid[closeBracket]

	advancedOnce := false
	if valid[escapeInterpolation] && len(s.delimiters) > 0 &&
		(lexer.Lookahead == '{' || lexer.Lookahead == '}') && !errorRecoveryMode {
		if s.back().isFormat() {
			lexer.MarkEnd()
			isLeftBrace := lexer.Lookahead == '{'
			lexer.Advance(false)
			advancedOnce = true
			if (lexer.Lookahead == '{' && isLeftBrace) || (lexer.Lookahead == '}' && !isLeftBrace) {
				lexer.Advance(false)
				lexer.MarkEnd()
				lexer.ResultSymbol = escapeInterpolation
				return true
			}
			return false
		}
	}

	if valid[stringContent] && len(s.delimiters) > 0 && !errorRecoveryMode {
		if done, ok := s.scanStringContent(lexer, advancedOnce); done {
			return ok
		}
	}

	lexer.MarkEnd()

	bail, foundEndOfLine, indentLength, firstCommentIndentLength := s.scanIndentation(lexer, valid)
	if bail {
		return false
	}

	if foundEndOfLine {
		if len(s.indents) > 0 {
			current := s.indents[len(s.indents)-1]

			if valid[indent] && indentLength > current {
				s.indents = append(s.indents, indentLength)
				lexer.ResultSymbol = indent
				return true
			}

			nextIsStringStart := lexer.Lookahead == '"' || lexer.Lookahead == '\'' || lexer.Lookahead == '`'

			if (valid[dedent] ||
				(!valid[newline] && !(valid[stringStart] && nextIsStringStart) && !withinBrackets)) &&
				indentLength < current && !s.insideInterpolatedString &&
				// A comment indented with the block belongs to the block, so the
				// dedent waits until the comments are consumed.
				firstCommentIndentLength < int32(current) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.ResultSymbol = dedent
				return true
			}
		}

		if valid[newline] && !errorRecoveryMode {
			lexer.ResultSymbol = newline
			return true
		}
	}

	if firstCommentIndentLength == -1 && valid[stringStart] {
		return s.scanStringStart(lexer)
	}
	return false
}

// scanStringContent reads the body of the string on top of the stack. done is
// false when the body ran to the end of the input without a verdict, and the
// caller then falls through to the indentation scan.
func (s *pythonScanner) scanStringContent(lexer *ts.Lexer, advancedOnce bool) (done, ok bool) {
	d := s.back()
	endChar := d.endCharacter()
	hasContent := advancedOnce
	for lexer.Lookahead != 0 {
		if (advancedOnce || lexer.Lookahead == '{' || lexer.Lookahead == '}') && d.isFormat() {
			lexer.MarkEnd()
			lexer.ResultSymbol = stringContent
			return true, hasContent
		}
		switch {
		case lexer.Lookahead == '\\':
			if d.isRaw() {
				// A raw string keeps the backslash, so the scanner steps over it
				// and over whatever it escapes, and keeps reading content.
				lexer.Advance(false)
				if lexer.Lookahead == endChar || lexer.Lookahead == '\\' {
					lexer.Advance(false)
				}
				if lexer.Lookahead == '\r' {
					lexer.Advance(false)
					if lexer.Lookahead == '\n' {
						lexer.Advance(false)
					}
				} else if lexer.Lookahead == '\n' {
					lexer.Advance(false)
				}
				continue
			}
			if d.isBytes() {
				lexer.MarkEnd()
				lexer.Advance(false)
				// A bytes string has no \N{...}, \uXXXX or \UXXXXXXXX escape, so
				// those stay content rather than becoming an escape sequence.
				if lexer.Lookahead == 'N' || lexer.Lookahead == 'u' || lexer.Lookahead == 'U' {
					lexer.Advance(false)
				} else {
					lexer.ResultSymbol = stringContent
					return true, hasContent
				}
			} else {
				lexer.MarkEnd()
				lexer.ResultSymbol = stringContent
				return true, hasContent
			}
		case lexer.Lookahead == endChar:
			return true, s.scanQuote(lexer, d, endChar, hasContent)
		case lexer.Lookahead == '\n' && hasContent && !d.isTriple():
			return true, false
		}
		lexer.Advance(false)
		hasContent = true
	}
	return false, false
}

// scanQuote handles the closing quote of a string, which either ends the string
// or is content because the string needs three of them.
func (s *pythonScanner) scanQuote(lexer *ts.Lexer, d delimiter, endChar int32, hasContent bool) bool {
	if d.isTriple() {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead != endChar {
			lexer.MarkEnd()
			lexer.ResultSymbol = stringContent
			return true
		}
		lexer.Advance(false)
		if lexer.Lookahead != endChar {
			lexer.MarkEnd()
			lexer.ResultSymbol = stringContent
			return true
		}
		if hasContent {
			lexer.ResultSymbol = stringContent
		} else {
			lexer.Advance(false)
			lexer.MarkEnd()
			s.pop()
			lexer.ResultSymbol = stringEnd
			s.insideInterpolatedString = false
		}
		return true
	}

	if hasContent {
		lexer.ResultSymbol = stringContent
	} else {
		lexer.Advance(false)
		s.pop()
		lexer.ResultSymbol = stringEnd
		s.insideInterpolatedString = false
	}
	lexer.MarkEnd()
	return true
}

// scanIndentation consumes the whitespace, comments and line continuations
// before the next real token, and measures the column the token starts at.
//
// bail says the scanner has no token to report at all, and the caller must stop
// rather than read the other three answers.
//
// firstCommentIndentLength is -1 when no comment was consumed. It is the column
// of the first one otherwise, which is what holds a dedent back until the
// comments that belong to the block are read.
func (s *pythonScanner) scanIndentation(lexer *ts.Lexer, valid []bool) (bail, foundEndOfLine bool, indentLength uint16, firstCommentIndentLength int32) {
	firstCommentIndentLength = -1
	for {
		switch {
		case lexer.Lookahead == '\n':
			foundEndOfLine = true
			indentLength = 0
			lexer.Advance(true)
		case lexer.Lookahead == ' ':
			indentLength++
			lexer.Advance(true)
		case lexer.Lookahead == '\r' || lexer.Lookahead == '\f':
			indentLength = 0
			lexer.Advance(true)
		case lexer.Lookahead == '\t':
			indentLength += 8
			lexer.Advance(true)
		case lexer.Lookahead == '#' &&
			(valid[indent] || valid[dedent] || valid[newline] || valid[except]):
			// Without an end of line first, this comment trails an expression,
			// as in "foo = bar # comment". That line opens no block, so the
			// scanner reports nothing rather than an indent or a dedent.
			if !foundEndOfLine {
				return true, false, indentLength, firstCommentIndentLength
			}
			if firstCommentIndentLength == -1 {
				firstCommentIndentLength = int32(indentLength)
			}
			for lexer.Lookahead != 0 && lexer.Lookahead != '\n' {
				lexer.Advance(true)
			}
			lexer.Advance(true)
			indentLength = 0
		case lexer.Lookahead == '\\':
			lexer.Advance(true)
			if lexer.Lookahead == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead == '\n' || lexer.EOF() {
				lexer.Advance(true)
			} else {
				return true, false, indentLength, firstCommentIndentLength
			}
		case lexer.EOF():
			// The end of the file closes every open block, so it counts as a
			// line ending at column zero.
			return false, true, 0, firstCommentIndentLength
		default:
			return false, foundEndOfLine, indentLength, firstCommentIndentLength
		}
	}
}

// scanStringStart reads a string's prefix letters and opening quote, and pushes
// the delimiter the body will be read against.
func (s *pythonScanner) scanStringStart(lexer *ts.Lexer) bool {
	var d delimiter

	for lexer.Lookahead != 0 {
		switch lexer.Lookahead {
		case 'f', 'F', 't', 'T':
			d |= flagFormat
		case 'r', 'R':
			d |= flagRaw
		case 'b', 'B':
			d |= flagBytes
		case 'u', 'U':
			// A u prefix changes nothing about how the body reads.
		default:
			return s.openQuote(lexer, d)
		}
		lexer.Advance(false)
	}
	return s.openQuote(lexer, d)
}

// openQuote takes the quote that follows a string's prefix letters.
func (s *pythonScanner) openQuote(lexer *ts.Lexer, d delimiter) bool {
	switch lexer.Lookahead {
	case '`':
		d.setEndCharacter('`')
		lexer.Advance(false)
		lexer.MarkEnd()
	case '\'', '"':
		quote := lexer.Lookahead
		d.setEndCharacter(quote)
		lexer.Advance(false)
		lexer.MarkEnd()
		if lexer.Lookahead == quote {
			lexer.Advance(false)
			if lexer.Lookahead == quote {
				lexer.Advance(false)
				lexer.MarkEnd()
				d |= flagTriple
			}
		}
	}

	if d.endCharacter() != 0 {
		s.delimiters = append(s.delimiters, d)
		lexer.ResultSymbol = stringStart
		s.insideInterpolatedString = d.isFormat()
		return true
	}
	// Prefix letters with no quote after them are an identifier, not a string.
	return false
}

// back is the delimiter on top of the stack. The callers check the stack is not
// empty first.
func (s *pythonScanner) back() delimiter { return s.delimiters[len(s.delimiters)-1] }

// pop drops the delimiter on top of the stack.
func (s *pythonScanner) pop() { s.delimiters = s.delimiters[:len(s.delimiters)-1] }

// boolByte writes a bool the way the C scanner does.
func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}
