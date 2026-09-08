package bash

import ts "github.com/wow-look-at-my/go-tree-sitter"

type globState struct {
	done           bool
	sawNonAlphaDot bool
	parenDepth     uint32
	bracketDepth   uint32
	braceDepth     uint32
}

func (s *bashScanner) scanExtglob(lexer *ts.Lexer, valid []bool) (bool, bool) {
	if !valid[extglobPattern] || inErrorRecovery(valid) {
		return false, false
	}
	// Skip whitespace, then look for the characters a pattern can open with.
	for isSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
	opens := lexer.Lookahead == '?' || lexer.Lookahead == '*' || lexer.Lookahead == '+' ||
		lexer.Lookahead == '@' || lexer.Lookahead == '!' || lexer.Lookahead == '-' ||
		lexer.Lookahead == ')' || lexer.Lookahead == '\\' || lexer.Lookahead == '.' ||
		lexer.Lookahead == '[' || isAlpha(lexer.Lookahead)
	if !opens {
		s.lastGlobParenDepth = 0
		return true, false
	}

	if lexer.Lookahead == '\\' {
		lexer.Advance(false)
		if (isSpace(lexer.Lookahead) || lexer.Lookahead == '"') &&
			lexer.Lookahead != '\r' && lexer.Lookahead != '\n' {
			lexer.Advance(false)
		} else {
			return true, false
		}
	}

	if lexer.Lookahead == ')' && s.lastGlobParenDepth == 0 {
		lexer.MarkEnd()
		lexer.Advance(false)
		if isSpace(lexer.Lookahead) {
			return true, false
		}
	}

	lexer.MarkEnd()
	wasNonAlpha := !isAlpha(lexer.Lookahead)
	if lexer.Lookahead != '[' {
		if lexer.Lookahead == 'e' {
			// A case arm cannot open with esac.
			lexer.MarkEnd()
			lexer.Advance(false)
			if lexer.Lookahead == 's' {
				lexer.Advance(false)
				if lexer.Lookahead == 'a' {
					lexer.Advance(false)
					if lexer.Lookahead == 'c' {
						lexer.Advance(false)
						if isSpace(lexer.Lookahead) {
							return true, false
						}
					}
				}
			}
		} else {
			lexer.Advance(false)
		}
	}

	// A dash followed by word characters is an ordinary word.
	if lexer.Lookahead == '-' {
		lexer.MarkEnd()
		lexer.Advance(false)
		for isAlnum(lexer.Lookahead) {
			lexer.Advance(false)
		}
		if lexer.Lookahead == ')' || lexer.Lookahead == '\\' || lexer.Lookahead == '.' {
			return true, false
		}
		lexer.MarkEnd()
	}

	// A case arm such as -) or *).
	if lexer.Lookahead == ')' && s.lastGlobParenDepth == 0 {
		lexer.MarkEnd()
		lexer.Advance(false)
		if isSpace(lexer.Lookahead) {
			lexer.ResultSymbol = extglobPattern
			return true, wasNonAlpha
		}
	}

	if isSpace(lexer.Lookahead) {
		lexer.MarkEnd()
		lexer.ResultSymbol = extglobPattern
		s.lastGlobParenDepth = 0
		return true, true
	}

	if lexer.Lookahead == '$' {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead == '{' || lexer.Lookahead == '(' {
			lexer.ResultSymbol = extglobPattern
			return true, true
		}
	}

	if lexer.Lookahead == '|' {
		lexer.MarkEnd()
		lexer.Advance(false)
		lexer.ResultSymbol = extglobPattern
		return true, true
	}

	if !isAlnum(lexer.Lookahead) && lexer.Lookahead != '(' && lexer.Lookahead != '"' &&
		lexer.Lookahead != '[' && lexer.Lookahead != '?' && lexer.Lookahead != '/' &&
		lexer.Lookahead != '\\' && lexer.Lookahead != '_' && lexer.Lookahead != '*' {
		return true, false
	}
	return true, s.extglobBody(lexer, wasNonAlpha)
}

func (s *bashScanner) extglobBody(lexer *ts.Lexer, wasNonAlpha bool) bool {
	st := globState{sawNonAlphaDot: wasNonAlpha, parenDepth: uint32(s.lastGlobParenDepth)}
	for !st.done {
		switch lexer.Lookahead {
		case 0:
			return false
		case '(':
			st.parenDepth++
		case '[':
			st.bracketDepth++
		case '{':
			st.braceDepth++
		case ')':
			if st.parenDepth == 0 {
				st.done = true
			}
			st.parenDepth--
		case ']':
			if st.bracketDepth == 0 {
				st.done = true
			}
			st.bracketDepth--
		case '}':
			if st.braceDepth == 0 {
				st.done = true
			}
			st.braceDepth--
		}

		if lexer.Lookahead == '|' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if st.parenDepth == 0 && st.bracketDepth == 0 && st.braceDepth == 0 {
				lexer.ResultSymbol = extglobPattern
				return true
			}
		}

		if st.done {
			continue
		}
		wasSpace := isSpace(lexer.Lookahead)
		if lexer.Lookahead == '$' {
			lexer.MarkEnd()
			if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
				st.sawNonAlphaDot = true
			}
			lexer.Advance(false)
			if lexer.Lookahead == '(' || lexer.Lookahead == '{' {
				lexer.ResultSymbol = extglobPattern
				s.lastGlobParenDepth = uint8(st.parenDepth)
				return st.sawNonAlphaDot
			}
		}
		if wasSpace {
			lexer.MarkEnd()
			lexer.ResultSymbol = extglobPattern
			s.lastGlobParenDepth = 0
			return st.sawNonAlphaDot
		}
		if lexer.Lookahead == '"' {
			lexer.MarkEnd()
			lexer.ResultSymbol = extglobPattern
			s.lastGlobParenDepth = 0
			return st.sawNonAlphaDot
		}
		if lexer.Lookahead == '\\' {
			if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
				st.sawNonAlphaDot = true
			}
			lexer.Advance(false)
			if isSpace(lexer.Lookahead) || lexer.Lookahead == '"' {
				lexer.Advance(false)
			}
		} else {
			if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
				st.sawNonAlphaDot = true
			}
			lexer.Advance(false)
		}
		if !wasSpace {
			lexer.MarkEnd()
		}
	}

	lexer.ResultSymbol = extglobPattern
	s.lastGlobParenDepth = 0
	return st.sawNonAlphaDot
}
