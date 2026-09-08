package bash

import ts "github.com/wow-look-at-my/go-tree-sitter"

// The C scanner jumps forward with goto into labelled sections. Go cannot jump
// over declarations, so each section is a function, and an entry point says
// where to resume. A section whose guard fails falls through to the next,
// exactly as the C does.
const (
	entryTop = iota
	entryRegex
	entryExtglob
	entryExpansionWord
	entryBraceStart
)

func (s *bashScanner) scan(lexer *ts.Lexer, valid []bool) bool {
	if done, result, entry := s.scanHead(lexer, valid); done {
		return result
	} else {
		return s.scanFrom(entry, lexer, valid)
	}
}

// scanFrom runs the labelled sections from an entry point onward.
func (s *bashScanner) scanFrom(entry int, lexer *ts.Lexer, valid []bool) bool {
	if entry <= entryRegex {
		if handled, result := s.scanRegex(lexer, valid); handled {
			return result
		}
	}
	if entry <= entryExtglob {
		if handled, result := s.scanExtglob(lexer, valid); handled {
			return result
		}
	}
	if entry <= entryExpansionWord {
		if handled, result := scanExpansionWord(lexer, valid); handled {
			return result
		}
	}
	if entry <= entryBraceStart {
		if handled, result := scanBraceStart(lexer, valid); handled {
			return result
		}
	}
	return false
}

// scanHead runs everything above the labels. It reports whether it finished,
// its result, and where to resume when it did not.
func (s *bashScanner) scanHead(lexer *ts.Lexer, valid []bool) (bool, bool, int) {
	if done, result := s.scanConcat(lexer, valid); done {
		return true, result, entryTop
	}
	if done, result := scanDoubleHash(lexer, valid); done {
		return true, result, entryTop
	}
	if done, result := scanExpansionSym(lexer, valid); done {
		return true, result, entryTop
	}
	if valid[emptyValue] {
		if isSpace(lexer.Lookahead) || lexer.EOF() ||
			lexer.Lookahead == ';' || lexer.Lookahead == '&' {
			lexer.ResultSymbol = emptyValue
			return true, true, entryTop
		}
	}
	if done, result := s.scanHeredocs(lexer, valid); done {
		return true, result, entryTop
	}
	if done, result, entry := s.scanTestOperator(lexer, valid); done {
		return true, result, entryTop
	} else if entry != entryTop {
		return false, false, entry
	}
	// The variable name block comes first, and it returns or jumps whenever its
	// guard holds. A bare dollar is therefore only reachable when that guard
	// fails, which is what keeps $$ a special variable rather than a bare
	// dollar that swallows the second character.
	if done, result, entry := s.scanVariableName(lexer, valid); done {
		return true, result, entryTop
	} else if entry != entryTop {
		return false, false, entry
	}
	if valid[bareDollar] && !inErrorRecovery(valid) && scanBareDollar(lexer) {
		return true, true, entryTop
	}
	return false, false, entryTop
}

func (s *bashScanner) scanConcat(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[concat] || inErrorRecovery(valid) {
		return false, false
	}
	stop := lexer.Lookahead == 0 || isSpace(lexer.Lookahead) || lexer.Lookahead == '>' ||
		lexer.Lookahead == '<' || lexer.Lookahead == ')' || lexer.Lookahead == '(' ||
		lexer.Lookahead == ';' || lexer.Lookahead == '&' || lexer.Lookahead == '|' ||
		(lexer.Lookahead == '}' && valid[closingBrace]) ||
		(lexer.Lookahead == ']' && valid[closingBracket])
	if !stop {
		lexer.ResultSymbol = concat
		// For a`b` the answer is a concat. A backtick run followed by
		// whitespace is what settles it.
		if lexer.Lookahead == '`' {
			lexer.MarkEnd()
			lexer.Advance(false)
			for lexer.Lookahead != '`' && !lexer.EOF() {
				lexer.Advance(false)
			}
			if lexer.EOF() {
				return true, false
			}
			if lexer.Lookahead == '`' {
				lexer.Advance(false)
			}
			return true, isSpace(lexer.Lookahead) || lexer.EOF()
		}
		// A string with an expansion that holds an escaped quote or backslash
		// needs this to come back as a concat.
		if lexer.Lookahead == '\\' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead == '"' || lexer.Lookahead == '\'' || lexer.Lookahead == '\\' {
				return true, true
			}
			if lexer.EOF() {
				return true, false
			}
		} else {
			return true, true
		}
	}
	if isSpace(lexer.Lookahead) && valid[closingBrace] && !valid[expansionWord] {
		lexer.ResultSymbol = concat
		return true, true
	}
	return false, false
}

func scanDoubleHash(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[immediateDoubleHash] || inErrorRecovery(valid) {
		return false, false
	}
	// Read both hashes, and reject a closing brace after them.
	if lexer.Lookahead == '#' {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead == '#' {
			lexer.Advance(false)
			if lexer.Lookahead != '}' {
				lexer.ResultSymbol = immediateDoubleHash
				lexer.MarkEnd()
				return true, true
			}
		}
	}
	return false, false
}

func scanExpansionSym(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[externalExpansionSymHash] || inErrorRecovery(valid) {
		return false, false
	}
	if lexer.Lookahead != '#' && lexer.Lookahead != '=' && lexer.Lookahead != '!' {
		return false, false
	}
	switch lexer.Lookahead {
	case '#':
		lexer.ResultSymbol = externalExpansionSymHash
	case '!':
		lexer.ResultSymbol = externalExpansionSymBang
	default:
		lexer.ResultSymbol = externalExpansionSymEqual
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	for lexer.Lookahead == '#' || lexer.Lookahead == '=' || lexer.Lookahead == '!' {
		lexer.Advance(false)
	}
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	return true, lexer.Lookahead == '}'
}

func (s *bashScanner) scanHeredocs(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if (valid[heredocBodyBeginning] || valid[simpleHeredocBody]) && len(s.heredocs) > 0 &&
		!s.back().started && !inErrorRecovery(valid) {
		return true, s.scanHeredocContent(lexer, heredocBodyBeginning, simpleHeredocBody)
	}
	if valid[heredocEnd] && len(s.heredocs) > 0 {
		if scanHeredocEndIdentifier(s.back(), lexer) {
			s.heredocs = s.heredocs[:len(s.heredocs)-1]
			lexer.ResultSymbol = heredocEnd
			return true, true
		}
	}
	if valid[heredocContent] && len(s.heredocs) > 0 && s.back().started &&
		!inErrorRecovery(valid) {
		return true, s.scanHeredocContent(lexer, heredocContent, heredocEnd)
	}
	if valid[heredocStart] && !inErrorRecovery(valid) && len(s.heredocs) > 0 {
		return true, scanHeredocStart(s.back(), lexer)
	}
	return false, false
}

func (s *bashScanner) scanTestOperator(lexer *ts.Lexer, valid []bool) (bool, bool, int) {
	if !valid[testOperator] || valid[expansionWord] {
		return false, false, entryTop
	}
	for isSpace(lexer.Lookahead) && lexer.Lookahead != '\n' {
		lexer.Advance(true)
	}

	if lexer.Lookahead == '\\' {
		if valid[extglobPattern] {
			return false, false, entryExtglob
		}
		if valid[regexNoSpace] {
			return false, false, entryRegex
		}
		lexer.Advance(true)
		if lexer.EOF() {
			return true, false, entryTop
		}
		switch {
		case lexer.Lookahead == '\r':
			lexer.Advance(true)
			if lexer.Lookahead == '\n' {
				lexer.Advance(true)
			}
		case lexer.Lookahead == '\n':
			lexer.Advance(true)
		default:
			return true, false, entryTop
		}
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
	}

	if lexer.Lookahead == '\n' && !valid[newline] {
		lexer.Advance(true)
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
	}

	if lexer.Lookahead == '-' {
		lexer.Advance(false)
		advancedOnce := false
		for isAlpha(lexer.Lookahead) {
			advancedOnce = true
			lexer.Advance(false)
		}
		if isSpace(lexer.Lookahead) && advancedOnce {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead == '}' && valid[closingBrace] {
				if valid[expansionWord] {
					lexer.MarkEnd()
					lexer.ResultSymbol = expansionWord
					return true, true, entryTop
				}
				return true, false, entryTop
			}
			lexer.ResultSymbol = testOperator
			return true, true, entryTop
		}
		if isSpace(lexer.Lookahead) && valid[extglobPattern] {
			lexer.ResultSymbol = extglobPattern
			return true, true, entryTop
		}
	}

	if valid[bareDollar] && !inErrorRecovery(valid) && scanBareDollar(lexer) {
		return true, true, entryTop
	}
	return false, false, entryTop
}
