package bash

import (
	"encoding/binary"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// Hand port of tree-sitter-bash's src/scanner.c. Heredocs are the reason it
// exists: their body runs until a delimiter the scanner read earlier, which no
// parse table can express.

// External token indices, in the order src/scanner.c declares them.
const (
	heredocStart = iota
	simpleHeredocBody
	heredocBodyBeginning
	heredocContent
	heredocEnd
	fileDescriptor
	emptyValue
	concat
	variableName
	testOperator
	regex
	regexNoSlash
	regexNoSpace
	expansionWord
	extglobPattern
	bareDollar
	braceStart
	immediateDoubleHash
	externalExpansionSymHash
	externalExpansionSymBang
	externalExpansionSymEqual
	closingBrace
	closingBracket
	heredocArrow
	heredocArrowDash
	newline
	openingParen
	esac
	errorRecovery
)

// serializationBufferSize is the runtime's cap on a serialized scanner state.
const serializationBufferSize = 1024

type heredoc struct {
	isRaw            bool
	started          bool
	allowsIndent     bool
	delimiter        []byte
	currentLeadingWd []byte
}

type bashScanner struct {
	lastGlobParenDepth  uint8
	extWasInDoubleQuote bool
	extSawOutsideQuote  bool
	heredocs            []heredoc
}

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Create makes the state a parse carries.
func (scanner) Create() any { return &bashScanner{} }

// Destroy releases the state. Go collects it, so nothing happens here.
func (scanner) Destroy(any) {}

// Scan reads an external token.
func (scanner) Scan(payload any, lexer *ts.Lexer, valid []bool) bool {
	return payload.(*bashScanner).scan(lexer, valid)
}

// Serialize writes the heredoc stack, so an incremental reparse resumes with
// the delimiters it needs to find a body's end.
func (scanner) Serialize(payload any, buffer []byte) uint32 {
	s := payload.(*bashScanner)
	size := 0
	buffer[size] = s.lastGlobParenDepth
	size++
	buffer[size] = boolByte(s.extWasInDoubleQuote)
	size++
	buffer[size] = boolByte(s.extSawOutsideQuote)
	size++
	buffer[size] = uint8(len(s.heredocs))
	size++

	for i := range s.heredocs {
		h := &s.heredocs[i]
		if size+3+4+len(h.delimiter) >= serializationBufferSize {
			return 0
		}
		buffer[size] = boolByte(h.isRaw)
		size++
		buffer[size] = boolByte(h.started)
		size++
		buffer[size] = boolByte(h.allowsIndent)
		size++
		binary.LittleEndian.PutUint32(buffer[size:], uint32(len(h.delimiter)))
		size += 4
		size += copy(buffer[size:], h.delimiter)
	}
	return uint32(size)
}

// Deserialize reads back what Serialize wrote.
func (scanner) Deserialize(payload any, buffer []byte) {
	s := payload.(*bashScanner)
	if len(buffer) == 0 {
		for i := range s.heredocs {
			s.heredocs[i].reset()
		}
		return
	}
	size := 0
	s.lastGlobParenDepth = buffer[size]
	size++
	s.extWasInDoubleQuote = buffer[size] != 0
	size++
	s.extSawOutsideQuote = buffer[size] != 0
	size++
	count := int(buffer[size])
	size++

	for i := 0; i < count; i++ {
		if i >= len(s.heredocs) {
			s.heredocs = append(s.heredocs, heredoc{})
		}
		h := &s.heredocs[i]
		h.isRaw = buffer[size] != 0
		size++
		h.started = buffer[size] != 0
		size++
		h.allowsIndent = buffer[size] != 0
		size++
		length := int(binary.LittleEndian.Uint32(buffer[size:]))
		size += 4
		h.delimiter = make([]byte, length)
		size += copy(h.delimiter, buffer[size:])
	}
}

func boolByte(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}

func (h *heredoc) reset() {
	h.isRaw = false
	h.started = false
	h.allowsIndent = false
	h.delimiter = h.delimiter[:0]
}

func (s *bashScanner) back() *heredoc { return &s.heredocs[len(s.heredocs)-1] }

// cString compares two byte runs the way strcmp does, stopping at the first
// zero byte in either.
func cString(a, b []byte) bool {
	for i := 0; ; i++ {
		var x, y byte
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return false
		}
		if x == 0 {
			return true
		}
	}
}

func inErrorRecovery(valid []bool) bool { return valid[errorRecovery] }

// advanceWord reads a POSIX word and returns it unquoted. It is the same
// approximation the C scanner makes: no substitution, and the default IFS.
func advanceWord(lexer *ts.Lexer, word *[]byte) bool {
	empty := true
	var quote int32
	if lexer.Lookahead == '\'' || lexer.Lookahead == '"' {
		quote = lexer.Lookahead
		lexer.Advance(false)
	}

	for lexer.Lookahead != 0 {
		if quote != 0 {
			if lexer.Lookahead == quote || lexer.Lookahead == '\r' || lexer.Lookahead == '\n' {
				break
			}
		} else if isSpace(lexer.Lookahead) {
			break
		}
		if lexer.Lookahead == '\\' {
			lexer.Advance(false)
			if lexer.Lookahead == 0 {
				return false
			}
		}
		empty = false
		*word = append(*word, byte(lexer.Lookahead))
		lexer.Advance(false)
	}
	*word = append(*word, 0)

	if quote != 0 && lexer.Lookahead == quote {
		lexer.Advance(false)
	}
	return !empty
}

func scanBareDollar(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) && lexer.Lookahead != '\n' && !lexer.EOF() {
		lexer.Advance(true)
	}
	if lexer.Lookahead == '$' {
		lexer.Advance(false)
		lexer.ResultSymbol = bareDollar
		lexer.MarkEnd()
		return isSpace(lexer.Lookahead) || lexer.EOF() || lexer.Lookahead == '"'
	}
	return false
}

func scanHeredocStart(h *heredoc, lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	lexer.ResultSymbol = heredocStart
	h.isRaw = lexer.Lookahead == '\'' || lexer.Lookahead == '"' || lexer.Lookahead == '\\'

	if !advanceWord(lexer, &h.delimiter) {
		h.delimiter = h.delimiter[:0]
		return false
	}
	return true
}

// scanHeredocEndIdentifier reads the start of a line and reports whether it is
// the delimiter the heredoc is waiting for.
func scanHeredocEndIdentifier(h *heredoc, lexer *ts.Lexer) bool {
	h.currentLeadingWd = h.currentLeadingWd[:0]
	at := 0
	if len(h.delimiter) > 0 {
		for lexer.Lookahead != 0 && lexer.Lookahead != '\n' &&
			int32(int8(h.delimiter[at])) == lexer.Lookahead &&
			len(h.currentLeadingWd) < len(h.delimiter) {
			h.currentLeadingWd = append(h.currentLeadingWd, byte(lexer.Lookahead))
			lexer.Advance(false)
			at++
		}
	}
	h.currentLeadingWd = append(h.currentLeadingWd, 0)
	if len(h.delimiter) == 0 {
		return false
	}
	return cString(h.currentLeadingWd, h.delimiter)
}

func (s *bashScanner) scanHeredocContent(lexer *ts.Lexer, middle, end int) bool {
	didAdvance := false
	h := s.back()

	for {
		switch lexer.Lookahead {
		case 0:
			if lexer.EOF() && didAdvance {
				h.reset()
				lexer.ResultSymbol = ts.Symbol(end)
				return true
			}
			return false

		case '\\':
			didAdvance = true
			lexer.Advance(false)
			lexer.Advance(false)

		case '$':
			if h.isRaw {
				didAdvance = true
				lexer.Advance(false)
				break
			}
			if didAdvance {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(middle)
				h.started = true
				lexer.Advance(false)
				if isAlpha(lexer.Lookahead) || lexer.Lookahead == '{' || lexer.Lookahead == '(' {
					return true
				}
				break
			}
			if middle == heredocBodyBeginning && lexer.GetColumn() == 0 {
				lexer.ResultSymbol = ts.Symbol(middle)
				h.started = true
				return true
			}
			return false

		case '\n':
			if !didAdvance {
				lexer.Advance(true)
			} else {
				lexer.Advance(false)
			}
			didAdvance = true
			if h.allowsIndent {
				for isSpace(lexer.Lookahead) {
					lexer.Advance(false)
				}
			}
			if h.started {
				lexer.ResultSymbol = ts.Symbol(middle)
			} else {
				lexer.ResultSymbol = ts.Symbol(end)
			}
			lexer.MarkEnd()
			if scanHeredocEndIdentifier(h, lexer) {
				if lexer.ResultSymbol == heredocEnd {
					s.heredocs = s.heredocs[:len(s.heredocs)-1]
				}
				return true
			}

		default:
			if lexer.GetColumn() == 0 {
				// Tracking the body's starting column statefully is the other
				// way to do this.
				for isSpace(lexer.Lookahead) {
					lexer.Advance(!didAdvance)
				}
				if end != simpleHeredocBody {
					lexer.ResultSymbol = ts.Symbol(middle)
					if scanHeredocEndIdentifier(h, lexer) {
						return true
					}
				}
				if end == simpleHeredocBody {
					lexer.ResultSymbol = ts.Symbol(end)
					lexer.MarkEnd()
					if scanHeredocEndIdentifier(h, lexer) {
						return true
					}
				}
			}
			didAdvance = true
			lexer.Advance(false)
		}
	}
}

// isSpace, isAlpha, isDigit and isAlnum match the wide character tests under
// the C locale, which the scanner runs under.
func isSpace(ch int32) bool {
	switch ch {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

func isDigit(ch int32) bool { return ch >= '0' && ch <= '9' }

func isAlpha(ch int32) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func isAlnum(ch int32) bool { return isAlpha(ch) || isDigit(ch) }
