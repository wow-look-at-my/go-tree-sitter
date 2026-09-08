package rust

import (
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// External token indices, in the order src/scanner.c declares them.
const (
	stringContent = iota
	stringClose
	rawStringLiteralStart
	rawStringLiteralContent
	rawStringLiteralEnd
	floatLiteral
	blockOuterDocMarker
	blockInnerDocMarker
	blockCommentContent
	lineDocContent
	errorSentinel
)

type rustScanner struct {
	openingHashCount uint8
}

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Create makes the state a parse carries.
func (scanner) Create() any { return &rustScanner{} }

// Destroy releases the state. Go collects it, so nothing happens here.
func (scanner) Destroy(any) {}

// Serialize writes the open hash count of the raw string being read.
func (scanner) Serialize(payload any, buffer []byte) uint32 {
	if len(buffer) == 0 {
		return 0
	}
	buffer[0] = payload.(*rustScanner).openingHashCount
	return 1
}

// Deserialize reads back what Serialize wrote.
func (scanner) Deserialize(payload any, buffer []byte) {
	s := payload.(*rustScanner)
	s.openingHashCount = 0
	if len(buffer) == 1 {
		s.openingHashCount = buffer[0]
	}
}

// Scan reads an external token.
func (scanner) Scan(payload any, lexer *ts.Lexer, validSymbols []bool) bool {
	// The error sentinel is valid only while the parser recovers from an error.
	// The scanner cannot help there, so it fails.
	if validSymbols[errorSentinel] {
		return false
	}

	s := payload.(*rustScanner)

	if validSymbols[blockCommentContent] || validSymbols[blockInnerDocMarker] ||
		validSymbols[blockOuterDocMarker] {
		return processBlockComment(lexer, validSymbols)
	}

	if validSymbols[stringContent] && !validSymbols[floatLiteral] {
		if processString(lexer) {
			return true
		}
		// A false here means the next character is a quote or a backslash, so
		// there is no content. Fall through and let stringClose take the quote.
	}

	if validSymbols[stringClose] && lexer.Lookahead == '"' {
		lexer.Advance(false)
		lexer.ResultSymbol = stringClose
		lexer.MarkEnd()
		return true
	}

	if validSymbols[lineDocContent] {
		return processLineDocContent(lexer)
	}

	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}

	if validSymbols[rawStringLiteralStart] &&
		(lexer.Lookahead == 'r' || lexer.Lookahead == 'b' || lexer.Lookahead == 'c') {
		return s.scanRawStringStart(lexer)
	}
	if validSymbols[rawStringLiteralContent] {
		return s.scanRawStringContent(lexer)
	}
	if validSymbols[rawStringLiteralEnd] && lexer.Lookahead == '"' {
		return s.scanRawStringEnd(lexer)
	}
	if validSymbols[floatLiteral] && isDigit(lexer.Lookahead) {
		return processFloatLiteral(lexer)
	}
	return false
}

func processString(lexer *ts.Lexer) bool {
	hasContent := false
	for {
		if lexer.Lookahead == '"' || lexer.Lookahead == '\\' {
			break
		}
		if lexer.EOF() {
			return false
		}
		hasContent = true
		lexer.Advance(false)
	}
	lexer.ResultSymbol = stringContent
	lexer.MarkEnd()
	return hasContent
}

func (s *rustScanner) scanRawStringStart(lexer *ts.Lexer) bool {
	if lexer.Lookahead == 'b' || lexer.Lookahead == 'c' {
		lexer.Advance(false)
	}
	if lexer.Lookahead != 'r' {
		return false
	}
	lexer.Advance(false)

	var hashes uint8
	for lexer.Lookahead == '#' {
		lexer.Advance(false)
		hashes++
	}
	if lexer.Lookahead != '"' {
		return false
	}
	lexer.Advance(false)
	s.openingHashCount = hashes
	lexer.ResultSymbol = rawStringLiteralStart
	return true
}

func (s *rustScanner) scanRawStringContent(lexer *ts.Lexer) bool {
	for {
		if lexer.EOF() {
			return false
		}
		if lexer.Lookahead != '"' {
			lexer.Advance(false)
			continue
		}
		lexer.MarkEnd()
		lexer.Advance(false)
		var hashes uint8
		for lexer.Lookahead == '#' && hashes < s.openingHashCount {
			lexer.Advance(false)
			hashes++
		}
		if hashes == s.openingHashCount {
			lexer.ResultSymbol = rawStringLiteralContent
			return true
		}
	}
}

func (s *rustScanner) scanRawStringEnd(lexer *ts.Lexer) bool {
	lexer.Advance(false)
	for i := uint8(0); i < s.openingHashCount; i++ {
		lexer.Advance(false)
	}
	lexer.ResultSymbol = rawStringLiteralEnd
	return true
}

func processFloatLiteral(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = floatLiteral

	lexer.Advance(false)
	for isNumChar(lexer.Lookahead) {
		lexer.Advance(false)
	}

	hasFraction, hasExponent := false, false

	if lexer.Lookahead == '.' {
		hasFraction = true
		lexer.Advance(false)
		// A letter after the dot makes this a method call, not a float.
		if isAlpha(lexer.Lookahead) {
			return false
		}
		if lexer.Lookahead == '.' {
			return false
		}
		for isNumChar(lexer.Lookahead) {
			lexer.Advance(false)
		}
	}

	lexer.MarkEnd()

	if lexer.Lookahead == 'e' || lexer.Lookahead == 'E' {
		hasExponent = true
		lexer.Advance(false)
		if lexer.Lookahead == '+' || lexer.Lookahead == '-' {
			lexer.Advance(false)
		}
		if !isNumChar(lexer.Lookahead) {
			return true
		}
		lexer.Advance(false)
		for isNumChar(lexer.Lookahead) {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
	}

	if !hasExponent && !hasFraction {
		return false
	}
	if lexer.Lookahead != 'u' && lexer.Lookahead != 'i' && lexer.Lookahead != 'f' {
		return true
	}
	lexer.Advance(false)
	if !isDigit(lexer.Lookahead) {
		return true
	}
	for isDigit(lexer.Lookahead) {
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	return true
}

func processLineDocContent(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = lineDocContent
	for {
		if lexer.EOF() {
			return true
		}
		if lexer.Lookahead == '\n' {
			// Markdown injection needs the newline inside the doc content.
			lexer.Advance(false)
			return true
		}
		lexer.Advance(false)
	}
}

// blockCommentState names where the reader stands inside a block comment.
type blockCommentState int

const (
	leftForwardSlash blockCommentState = iota
	leftAsterisk
	continuing
)

type blockCommentProcessing struct {
	state        blockCommentState
	nestingDepth uint32
}

func (p *blockCommentProcessing) leftForwardSlash(current int32) {
	if current == '*' {
		p.nestingDepth++
	}
	p.state = continuing
}

func (p *blockCommentProcessing) leftAsterisk(current int32, lexer *ts.Lexer) {
	if current == '*' {
		lexer.MarkEnd()
		p.state = leftAsterisk
		return
	}
	if current == '/' {
		p.nestingDepth--
	}
	p.state = continuing
}

func (p *blockCommentProcessing) continuing(current int32) {
	switch current {
	case '/':
		p.state = leftForwardSlash
	case '*':
		p.state = leftAsterisk
	}
}

func processBlockComment(lexer *ts.Lexer, validSymbols []bool) bool {
	// The C scanner keeps this in a char, so a wide lookahead truncates.
	first := narrow(lexer.Lookahead)

	// Only the opening character is kept, so the scanner may advance a single
	// time. It therefore advances on every branch, to leave a known state.
	switch {
	case validSymbols[blockInnerDocMarker] && first == '!':
		lexer.ResultSymbol = blockInnerDocMarker
		lexer.Advance(false)
		return true
	case validSymbols[blockOuterDocMarker] && first == '*':
		lexer.Advance(false)
		lexer.MarkEnd()
		// A slash next means the block comment is empty.
		if lexer.Lookahead == '/' {
			return false
		}
		// An asterisk next means a longer run, which is not this marker.
		if lexer.Lookahead != '*' {
			lexer.ResultSymbol = blockOuterDocMarker
			return true
		}
	default:
		lexer.Advance(false)
	}

	if !validSymbols[blockCommentContent] {
		return false
	}

	processing := blockCommentProcessing{state: continuing, nestingDepth: 1}
	switch first {
	case '*':
		processing.state = leftAsterisk
		// A doc block comment can be empty, as in the text /*!*/. It has no
		// content, so the scanner gives up.
		if lexer.Lookahead == '/' {
			return false
		}
	case '/':
		processing.state = leftForwardSlash
	default:
		processing.state = continuing
	}

	// An unterminated block comment reads as content rather than an error. That
	// is wrong for parsing and right for highlighting, which must colour a file
	// before its comment is closed.
	for !lexer.EOF() && processing.nestingDepth != 0 {
		first = narrow(lexer.Lookahead)
		switch processing.state {
		case leftForwardSlash:
			processing.leftForwardSlash(first)
		case leftAsterisk:
			processing.leftAsterisk(first, lexer)
		case continuing:
			lexer.MarkEnd()
			processing.continuing(first)
		}
		lexer.Advance(false)
		if first == '/' && processing.nestingDepth != 0 {
			lexer.MarkEnd()
		}
	}
	lexer.ResultSymbol = blockCommentContent
	return true
}

// narrow reproduces the C cast of a lookahead to a char.
func narrow(ch int32) int32 { return int32(int8(ch)) }

// isSpace, isDigit and isAlpha match the wide character tests under the C
// locale, which the scanner runs under.
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

func isNumChar(ch int32) bool { return ch == '_' || isDigit(ch) }
