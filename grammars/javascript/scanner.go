package javascript

import (
	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// External token indices, in the order src/scanner.c declares them.
const (
	automaticSemicolon = iota
	templateChars
	ternaryQmark
	htmlComment
	logicalOr
	escapeSequence
	regexPattern
	jsxText
)

// The line terminators JavaScript recognises beyond the ASCII ones.
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

	if valid[automaticSemicolon] {
		scannedComment := false
		ok := scanAutomaticSemicolon(lexer, !valid[logicalOr], &scannedComment)
		if !ok && !scannedComment && valid[ternaryQmark] && lexer.Lookahead == '?' {
			return scanTernaryQmark(lexer)
		}
		return ok
	}

	if valid[ternaryQmark] {
		return scanTernaryQmark(lexer)
	}

	if valid[htmlComment] && !valid[logicalOr] && !valid[escapeSequence] && !valid[regexPattern] {
		return scanHTMLComment(lexer)
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

// The verdict on the whitespace and comments before a candidate semicolon.
const (
	// reject means the semicolon is illegal, because a syntax error occurred.
	reject = iota
	// noNewline means the legality is still unclear, so the caller continues.
	noNewline
	// accept means the semicolon is legal, assuming a comment was read.
	accept
)

// scanWhitespaceAndComments skips to the next real token. consume false reads
// only far enough to decide whether a comment makes a semicolon legal.
func scanWhitespaceAndComments(lexer *ts.Lexer, scannedComment *bool, consume bool) int {
	sawBlockNewline := false
	for {
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
		if lexer.Lookahead != '/' {
			return accept
		}
		lexer.Advance(true)

		switch lexer.Lookahead {
		case '/':
			lexer.Advance(true)
			for lexer.Lookahead != 0 && !isLineEnd(lexer.Lookahead) {
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
						*scannedComment = true
						if lexer.Lookahead != '/' && !consume {
							if sawBlockNewline {
								return accept
							}
							return noNewline
						}
						break
					}
					continue
				}
				if isLineEnd(lexer.Lookahead) {
					sawBlockNewline = true
				}
				lexer.Advance(true)
			}
		default:
			return reject
		}
	}
}

// scanAutomaticSemicolon decides where JavaScript inserts the semicolon the
// author left out.
func scanAutomaticSemicolon(lexer *ts.Lexer, commentCondition bool, scannedComment *bool) bool {
	lexer.ResultSymbol = automaticSemicolon
	lexer.MarkEnd()

	for {
		if lexer.Lookahead == 0 {
			return true
		}
		if lexer.Lookahead == '/' {
			switch scanWhitespaceAndComments(lexer, scannedComment, false) {
			case reject:
				return false
			case accept:
				if commentCondition && lexer.Lookahead != ',' && lexer.Lookahead != '=' {
					return true
				}
			}
		}
		if lexer.Lookahead == '}' || lexer.IsAtIncludedRangeStart() {
			return true
		}
		if isLineEnd(lexer.Lookahead) {
			break
		}
		if !isSpace(lexer.Lookahead) {
			return false
		}
		lexer.Advance(true)
	}

	lexer.Advance(true)
	if scanWhitespaceAndComments(lexer, scannedComment, true) == reject {
		return false
	}
	return semicolonFitsBefore(lexer)
}

// semicolonFitsBefore reports whether the token after the line break can start
// a new statement. An operator continues the statement above instead.
func semicolonFitsBefore(lexer *ts.Lexer) bool {
	switch lexer.Lookahead {
	case '`', ',', ':', ';', '*', '%', '>', '<', '=', '[', '(', '?', '^', '|', '&', '/':
		return false

	case '.':
		// A decimal literal starts a statement. A property access does not.
		lexer.Advance(true)
		return isDigit(lexer.Lookahead)

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
// optional chain and the `??` of a nullish coalesce.
func scanTernaryQmark(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	if lexer.Lookahead != '?' {
		return false
	}
	lexer.Advance(false)
	if lexer.Lookahead == '?' {
		return false
	}

	lexer.MarkEnd()
	lexer.ResultSymbol = ternaryQmark

	if lexer.Lookahead == '.' {
		// `a?` before a decimal is a conditional. `a?.b` is an optional chain.
		lexer.Advance(false)
		return isDigit(lexer.Lookahead)
	}
	return true
}

// scanHTMLComment reads the `<!--` and `-->` a browser needed to hide a script
// from itself, which JavaScript still accepts as a line comment.
func scanHTMLComment(lexer *ts.Lexer) bool {
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

	for lexer.Lookahead != 0 && !isLineEnd(lexer.Lookahead) {
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

// isLineEnd reports a character JavaScript treats as a line terminator.
func isLineEnd(ch int32) bool {
	return ch == '\n' || ch == lineSeparator || ch == paragraphSeparator
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
