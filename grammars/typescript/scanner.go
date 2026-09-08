package typescript

import (
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// External token indices, in the order common/scanner.h declares them. The
// leading entries match JavaScript, and TypeScript appends its own.
const (
	automaticSemicolon = iota
	templateChars
	ternaryQmark
	htmlComment
	logicalOr
	escapeSequence
	regexPattern
	jsxText
	functionSignatureAutomaticSemicolon
	// The grammar also declares an error-recovery token.
	_
)

// The line terminators the closing-comment scan skips past. iswspace matches
// neither, so the loop that reads them names them.
const (
	lineSeparator      = 0x2028
	paragraphSeparator = 0x2029
)

// scanner is the value the generated table installs on the language.
type scanner struct{}

// Scanner returns the hand written external scanner.
func Scanner() ts.ExternalScanner { return scanner{} }

func (scanner) Create() any             { return nil }
func (scanner) Destroy(any)             {}
func (scanner) Deserialize(any, []byte) {}

// Serialize writes nothing. A stateless scanner resumes anywhere.
func (scanner) Serialize(any, []byte) uint32 { return 0 }

// Scan reads an external token. The order of these cases is the grammar's, and
// changing it changes which token wins where several are valid.
func (scanner) Scan(_ any, lexer *ts.Lexer, valid []bool) bool {
	if valid[templateChars] {
		// Both valid together means the parser is recovering from an error.
		if valid[automaticSemicolon] {
			return false
		}
		return scanTemplateChars(lexer)
	}

	if valid[jsxText] && scanJSXText(lexer) {
		return true
	}

	if valid[automaticSemicolon] || valid[functionSignatureAutomaticSemicolon] {
		scannedComment := false
		ok := scanAutomaticSemicolon(lexer, valid, &scannedComment)
		if !ok && !scannedComment && valid[ternaryQmark] && lexer.Lookahead == '?' {
			return scanTernaryQmark(lexer)
		}
		return ok
	}

	if valid[ternaryQmark] {
		return scanTernaryQmark(lexer)
	}

	if valid[htmlComment] && !valid[logicalOr] && !valid[escapeSequence] && !valid[regexPattern] {
		return scanClosingComment(lexer)
	}

	return false
}

// scanTemplateChars reads the literal run inside a template string, stopping
// before the backtick, the escape or the interpolation that ends it.
func scanTemplateChars(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = templateChars
	hasContent := false
	for ; ; hasContent = true {
		lexer.MarkEnd()
		switch lexer.Lookahead {
		case '`', '\\':
			return hasContent
		case 0:
			return false
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead == '{' {
				return hasContent
			}
		default:
			lexer.Advance(false)
		}
	}
}

// scanWhitespaceAndComments skips to the next real token, and reports false on
// a slash that starts neither comment.
func scanWhitespaceAndComments(lexer *ts.Lexer, scannedComment *bool) bool {
	for {
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
		if lexer.Lookahead != '/' {
			return true
		}
		lexer.Advance(true)

		switch lexer.Lookahead {
		case '/':
			lexer.Advance(true)
			for lexer.Lookahead != 0 && lexer.Lookahead != '\n' {
				lexer.Advance(true)
			}
			*scannedComment = true
		case '*':
			lexer.Advance(true)
			for lexer.Lookahead != 0 {
				if lexer.Lookahead == '*' {
					lexer.Advance(true)
					if lexer.Lookahead == '/' {
						lexer.Advance(true)
						break
					}
					continue
				}
				lexer.Advance(true)
			}
		default:
			return false
		}
	}
}

// scanAutomaticSemicolon decides where TypeScript inserts the semicolon the
// author left out.
func scanAutomaticSemicolon(lexer *ts.Lexer, valid []bool, scannedComment *bool) bool {
	lexer.ResultSymbol = automaticSemicolon
	lexer.MarkEnd()

	for {
		if lexer.Lookahead == 0 {
			return true
		}
		if lexer.Lookahead == '}' {
			// An inserted semicolon hides an object pattern in a typed position, as in
			// `type F = ({a}: {a: number}) => number`.
			for {
				lexer.Advance(true)
				if !isSpace(lexer.Lookahead) {
					break
				}
			}
			if lexer.Lookahead == ':' {
				// A valid `||` means a ternary rather than a type annotation.
				return valid[logicalOr]
			}
			return true
		}
		if !isSpace(lexer.Lookahead) {
			return false
		}
		if lexer.Lookahead == '\n' {
			break
		}
		lexer.Advance(true)
	}

	lexer.Advance(true)
	if !scanWhitespaceAndComments(lexer, scannedComment) {
		return false
	}
	return semicolonFitsBefore(lexer, valid)
}

// semicolonFitsBefore reports whether the token after the line break can start
// a new statement. An operator continues the statement above instead.
func semicolonFitsBefore(lexer *ts.Lexer, valid []bool) bool {
	switch lexer.Lookahead {
	case '`', ',', '.', ';', '*', '%', '>', '<', '=', '?', '^', '|', '&', '/', ':':
		return false

	case '{':
		// A signature's body follows it rather than starting a statement.
		return !valid[functionSignatureAutomaticSemicolon]

	case '(', '[':
		// A call or an index in an expression.
		return !valid[logicalOr]

	case '+':
		// `++` starts a statement. Binary `+` does not.
		lexer.Advance(true)
		return lexer.Lookahead == '+'
	case '-':
		lexer.Advance(true)
		return lexer.Lookahead == '-'

	case '!':
		// Unary `!` starts a statement. `!=` does not.
		lexer.Advance(true)
		return lexer.Lookahead != '='

	case 'i':
		// An identifier starts a statement. `in` and `instanceof` do not.
		lexer.Advance(true)
		if lexer.Lookahead != 'n' {
			return true
		}
		lexer.Advance(true)
		if !isAlpha(lexer.Lookahead) {
			return false
		}
		for _, want := range "stanceof" {
			if lexer.Lookahead != want {
				return true
			}
			lexer.Advance(true)
		}
		return isAlpha(lexer.Lookahead)
	}
	return true
}

// scanTernaryQmark separates the `?` of a conditional from the `?.` of an
// optional chain, the `??` of a nullish coalesce, and the `?` of an optional
// parameter or property.
func scanTernaryQmark(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	if lexer.Lookahead != '?' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead == '?' || lexer.Lookahead == '.' {
		return false
	}

	lexer.MarkEnd()
	lexer.ResultSymbol = ternaryQmark

	// An optional parameter writes `?:`, and the whitespace between them is
	// legal, so the type has to be read past before the shape is known.
	for isSpace(lexer.Lookahead) {
		lexer.Advance(false)
	}
	switch lexer.Lookahead {
	case ':', ')', ',':
		return false
	case '.':
		// `a?` before a decimal is a conditional. `a?.b` was rejected above.
		lexer.Advance(false)
		return isDigit(lexer.Lookahead)
	}
	return true
}

// scanClosingComment reads the `<!--` and `-->` a browser needed to hide a
// script from itself, which the language still accepts as a line comment.
func scanClosingComment(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) || isLineEnd(lexer.Lookahead) {
		lexer.Advance(true)
	}

	var opener string
	switch lexer.Lookahead {
	case '<':
		opener = "<!--"
	case '-':
		opener = "-->"
	default:
		return false
	}
	for _, want := range opener {
		if lexer.Lookahead != want {
			return false
		}
		lexer.Advance(false)
	}

	for lexer.Lookahead != 0 && lexer.Lookahead != '\n' && !isLineEnd(lexer.Lookahead) {
		lexer.Advance(false)
	}

	lexer.ResultSymbol = htmlComment
	lexer.MarkEnd()
	return true
}

// scanJSXText reads the literal text between JSX tags.
//
// Whitespace that only pads a line break is not text, so an element indented
// across several lines does not gain a child nobody wrote.
func scanJSXText(lexer *ts.Lexer) bool {
	sawText := false
	atNewline := false

	for lexer.Lookahead != 0 && !isJSXDelimiter(lexer.Lookahead) {
		if lexer.Lookahead == '\n' {
			atNewline = true
		} else {
			atNewline = atNewline && isSpace(lexer.Lookahead)
			if !atNewline {
				sawText = true
			}
		}
		lexer.Advance(false)
	}

	lexer.ResultSymbol = jsxText
	return sawText
}

// isJSXDelimiter reports a character that ends a run of JSX text.
func isJSXDelimiter(ch int32) bool {
	switch ch {
	case '<', '>', '{', '}', '&':
		return true
	}
	return false
}

// isLineEnd reports the line terminators outside the ASCII set.
func isLineEnd(ch int32) bool {
	return ch == lineSeparator || ch == paragraphSeparator
}

// isSpace matches iswspace under the C locale, which the scanner runs under.
func isSpace(ch int32) bool {
	switch ch {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// isDigit matches iswdigit under the C locale.
func isDigit(ch int32) bool { return ch >= '0' && ch <= '9' }

// isAlpha matches iswalpha under the C locale.
func isAlpha(ch int32) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}
