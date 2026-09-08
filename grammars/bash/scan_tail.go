package bash

import ts "github.com/wow-look-at-my/go-tree-sitter"

type regexState struct {
	done                     bool
	advancedOnce             bool
	foundNonAlnumDollarUnder bool
	lastWasEscape            bool
	inSingleQuote            bool
	parenDepth               uint32
	bracketDepth             uint32
	braceDepth               uint32
}

func (s *bashScanner) scanRegex(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !(valid[regex] || valid[regexNoSlash] || valid[regexNoSpace]) || inErrorRecovery(valid) {
		return false, false
	}
	if valid[regex] || valid[regexNoSpace] {
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
	}
	enter := (lexer.Lookahead != '"' && lexer.Lookahead != '\'') ||
		((lexer.Lookahead == '$' || lexer.Lookahead == '\'') && valid[regexNoSlash]) ||
		(lexer.Lookahead == '\'' && valid[regexNoSpace])
	if !enter {
		return false, false
	}

	if lexer.Lookahead == '$' && valid[regexNoSlash] {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead == '(' {
			return true, false
		}
	}
	lexer.MarkEnd()

	var st regexState
	for !st.done {
		if st.inSingleQuote && lexer.Lookahead == '\'' {
			st.inSingleQuote = false
			lexer.Advance(false)
			lexer.MarkEnd()
		}
		switch lexer.Lookahead {
		case '\\':
			st.lastWasEscape = true
		case 0:
			return true, false
		case '(':
			st.parenDepth++
			st.lastWasEscape = false
		case '[':
			st.bracketDepth++
			st.lastWasEscape = false
		case '{':
			if !st.lastWasEscape {
				st.braceDepth++
			}
			st.lastWasEscape = false
		case ')':
			if st.parenDepth == 0 {
				st.done = true
			}
			st.parenDepth--
			st.lastWasEscape = false
		case ']':
			if st.bracketDepth == 0 {
				st.done = true
			}
			st.bracketDepth--
			st.lastWasEscape = false
		case '}':
			if st.braceDepth == 0 {
				st.done = true
			}
			st.braceDepth--
			st.lastWasEscape = false
		case '\'':
			// Enter or leave a single quoted string.
			st.inSingleQuote = !st.inSingleQuote
			lexer.Advance(false)
			st.advancedOnce = true
			st.lastWasEscape = false
			continue
		default:
			st.lastWasEscape = false
		}

		if st.done {
			continue
		}
		switch {
		case valid[regex]:
			wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
			lexer.Advance(false)
			st.advancedOnce = true
			if !wasSpace || st.parenDepth > 0 {
				lexer.MarkEnd()
			}
		case valid[regexNoSlash]:
			if done, result := regexNoSlashStep(lexer, &st); done {
				return true, result
			}
		case valid[regexNoSpace]:
			if done, result := regexNoSpaceStep(lexer, &st); done {
				return true, result
			}
		}
	}

	switch {
	case valid[regexNoSlash]:
		lexer.ResultSymbol = regexNoSlash
	case valid[regexNoSpace]:
		lexer.ResultSymbol = regexNoSpace
	default:
		lexer.ResultSymbol = regex
	}
	if valid[regex] && !st.advancedOnce {
		return true, false
	}
	return true, true
}

func regexNoSlashStep(lexer *ts.Lexer, st *regexState) (bool, bool) {
	if lexer.Lookahead == '/' {
		lexer.MarkEnd()
		lexer.ResultSymbol = regexNoSlash
		return true, st.advancedOnce
	}
	if lexer.Lookahead == '\\' {
		lexer.Advance(false)
		st.advancedOnce = true
		if !lexer.EOF() && lexer.Lookahead != '[' && lexer.Lookahead != '/' {
			lexer.Advance(false)
			lexer.MarkEnd()
		}
		return false, false
	}
	wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
	lexer.Advance(false)
	st.advancedOnce = true
	if !wasSpace {
		lexer.MarkEnd()
	}
	return false, false
}

func regexNoSpaceStep(lexer *ts.Lexer, st *regexState) (bool, bool) {
	switch {
	case lexer.Lookahead == '\\':
		st.foundNonAlnumDollarUnder = true
		lexer.Advance(false)
		if !lexer.EOF() {
			lexer.Advance(false)
		}
	case lexer.Lookahead == '$':
		lexer.MarkEnd()
		lexer.Advance(false)
		// A command substitution is not a regex.
		if lexer.Lookahead == '(' {
			return true, false
		}
		// A trailing dollar always means a regex, as in 99999999$.
		if isSpace(lexer.Lookahead) {
			lexer.ResultSymbol = regexNoSpace
			lexer.MarkEnd()
			return true, true
		}
	default:
		wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
		if wasSpace && st.parenDepth == 0 {
			lexer.MarkEnd()
			lexer.ResultSymbol = regexNoSpace
			return true, st.foundNonAlnumDollarUnder
		}
		if !isAlnum(lexer.Lookahead) && lexer.Lookahead != '$' &&
			lexer.Lookahead != '-' && lexer.Lookahead != '_' {
			st.foundNonAlnumDollarUnder = true
		}
		lexer.Advance(false)
	}
	return false, false
}

func scanExpansionWord(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[expansionWord] {
		return false, false
	}
	advancedOnce := false
	advancedOnceSpace := false
	for {
		if lexer.Lookahead == '"' {
			return true, false
		}
		if lexer.Lookahead == '$' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead == '{' || lexer.Lookahead == '(' ||
				lexer.Lookahead == '\'' || isAlnum(lexer.Lookahead) {
				lexer.ResultSymbol = expansionWord
				return true, advancedOnce
			}
			advancedOnce = true
		}
		if lexer.Lookahead == '}' {
			lexer.MarkEnd()
			lexer.ResultSymbol = expansionWord
			return true, advancedOnce || advancedOnceSpace
		}
		if lexer.Lookahead == '(' && !(advancedOnce || advancedOnceSpace) {
			done, result := expansionParens(lexer, &advancedOnce, &advancedOnceSpace)
			if done {
				return true, result
			}
		}
		if lexer.Lookahead == '\'' {
			return true, false
		}
		if lexer.EOF() {
			return true, false
		}
		advancedOnce = advancedOnce || !isSpace(lexer.Lookahead)
		advancedOnceSpace = advancedOnceSpace || isSpace(lexer.Lookahead)
		lexer.Advance(false)
	}
}

// expansionParens reads a parenthesized run inside an expansion word. A dollar
// that opens an expansion inside it is taken as a concatenation rather than an
// error.
func expansionParens(lexer *ts.Lexer, advancedOnce, advancedOnceSpace *bool) (bool, bool) {
	lexer.MarkEnd()
	lexer.Advance(false)
	for lexer.Lookahead != ')' && !lexer.EOF() {
		if lexer.Lookahead == '$' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead == '{' || lexer.Lookahead == '(' ||
				lexer.Lookahead == '\'' || isAlnum(lexer.Lookahead) {
				lexer.ResultSymbol = expansionWord
				return true, *advancedOnce
			}
			*advancedOnce = true
			continue
		}
		*advancedOnce = *advancedOnce || !isSpace(lexer.Lookahead)
		*advancedOnceSpace = *advancedOnceSpace || isSpace(lexer.Lookahead)
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	if lexer.Lookahead != ')' {
		return true, false
	}
	*advancedOnce = true
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead == '}' {
		return true, false
	}
	return false, false
}

func scanBraceStart(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[braceStart] || inErrorRecovery(valid) {
		return false, false
	}
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	if lexer.Lookahead != '{' {
		return true, false
	}
	lexer.Advance(false)
	lexer.MarkEnd()

	for isDigit(lexer.Lookahead) {
		lexer.Advance(false)
	}
	if lexer.Lookahead != '.' {
		return true, false
	}
	lexer.Advance(false)
	if lexer.Lookahead != '.' {
		return true, false
	}
	lexer.Advance(false)
	for isDigit(lexer.Lookahead) {
		lexer.Advance(false)
	}
	if lexer.Lookahead != '}' {
		return true, false
	}
	lexer.ResultSymbol = braceStart
	return true, true
}
