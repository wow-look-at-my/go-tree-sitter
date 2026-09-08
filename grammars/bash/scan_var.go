package bash

import ts "github.com/wow-look-at-my/go-tree-sitter"

// scanVariableName covers the variable name, file descriptor and heredoc arrow
// tokens, which share an entry condition in the C scanner.
func (s *bashScanner) scanVariableName(lexer *ts.Lexer, valid []bool) (bool, bool, int) {
	if !(valid[variableName] || valid[fileDescriptor] || valid[heredocArrow]) ||
		valid[regexNoSlash] || inErrorRecovery(valid) {
		return false, false, entryTop
	}

	for {
		switch {
		case (lexer.Lookahead == ' ' || lexer.Lookahead == '\t' || lexer.Lookahead == '\r' ||
			(lexer.Lookahead == '\n' && !valid[newline])) && !valid[expansionWord]:
			lexer.Advance(true)
			continue
		case lexer.Lookahead == '\\':
			lexer.Advance(true)
			if lexer.EOF() {
				lexer.MarkEnd()
				lexer.ResultSymbol = variableName
				return true, true, entryTop
			}
			if lexer.Lookahead == '\r' {
				lexer.Advance(true)
			}
			if lexer.Lookahead == '\n' {
				lexer.Advance(true)
				continue
			}
			if lexer.Lookahead == '\\' && valid[expansionWord] {
				return false, false, entryExpansionWord
			}
			return true, false, entryTop
		}
		break
	}

	// Excluding '*', '@', '?', '-', '$', '0' and '_'.
	if !valid[expansionWord] &&
		(lexer.Lookahead == '*' || lexer.Lookahead == '@' || lexer.Lookahead == '?' ||
			lexer.Lookahead == '-' || lexer.Lookahead == '0' || lexer.Lookahead == '_') {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead == '=' || lexer.Lookahead == '[' || lexer.Lookahead == ':' ||
			lexer.Lookahead == '-' || lexer.Lookahead == '%' || lexer.Lookahead == '#' ||
			lexer.Lookahead == '/' {
			return true, false, entryTop
		}
		if valid[extglobPattern] && isSpace(lexer.Lookahead) {
			lexer.MarkEnd()
			lexer.ResultSymbol = extglobPattern
			return true, true, entryTop
		}
	}

	if valid[heredocArrow] && lexer.Lookahead == '<' {
		lexer.Advance(false)
		if lexer.Lookahead != '<' {
			return true, false, entryTop
		}
		lexer.Advance(false)
		switch {
		case lexer.Lookahead == '-':
			lexer.Advance(false)
			s.heredocs = append(s.heredocs, heredoc{allowsIndent: true})
			lexer.ResultSymbol = heredocArrowDash
		case lexer.Lookahead == '<' || lexer.Lookahead == '=':
			return true, false, entryTop
		default:
			s.heredocs = append(s.heredocs, heredoc{})
			lexer.ResultSymbol = heredocArrow
		}
		return true, true, entryTop
	}

	isNumber := true
	switch {
	case isDigit(lexer.Lookahead):
		lexer.Advance(false)
	case isAlpha(lexer.Lookahead) || lexer.Lookahead == '_':
		isNumber = false
		lexer.Advance(false)
	default:
		if lexer.Lookahead == '{' {
			return false, false, entryBraceStart
		}
		if valid[expansionWord] {
			return false, false, entryExpansionWord
		}
		if valid[extglobPattern] {
			return false, false, entryExtglob
		}
		return true, false, entryTop
	}

	for {
		if isDigit(lexer.Lookahead) {
			lexer.Advance(false)
			continue
		}
		if isAlpha(lexer.Lookahead) || lexer.Lookahead == '_' {
			isNumber = false
			lexer.Advance(false)
			continue
		}
		break
	}

	if isNumber && valid[fileDescriptor] &&
		(lexer.Lookahead == '>' || lexer.Lookahead == '<') {
		lexer.ResultSymbol = fileDescriptor
		return true, true, entryTop
	}

	if valid[variableName] {
		if done, result := finishVariableName(lexer, valid, isNumber); done {
			return true, result, entryTop
		}
	}
	return true, false, entryTop
}

func finishVariableName(lexer *ts.Lexer, valid []bool, isNumber bool) (bool, bool) {
	if lexer.Lookahead == '+' {
		lexer.MarkEnd()
		lexer.Advance(false)
		if lexer.Lookahead == '=' || lexer.Lookahead == ':' || valid[closingBrace] {
			lexer.ResultSymbol = variableName
			return true, true
		}
		return true, false
	}
	if lexer.Lookahead == '/' {
		return true, false
	}
	// A colon only counts outside a brace or a paren. The C scanner notes that
	// other word characters that are not variable names want the same care.
	if lexer.Lookahead == '=' || lexer.Lookahead == '[' ||
		(lexer.Lookahead == ':' && !valid[closingBrace] && !valid[openingParen]) ||
		lexer.Lookahead == '%' ||
		(lexer.Lookahead == '#' && !isNumber) || lexer.Lookahead == '@' ||
		(lexer.Lookahead == '-' && valid[closingBrace]) {
		lexer.MarkEnd()
		lexer.ResultSymbol = variableName
		return true, true
	}
	if lexer.Lookahead == '?' {
		lexer.MarkEnd()
		lexer.Advance(false)
		lexer.ResultSymbol = variableName
		return true, isAlpha(lexer.Lookahead)
	}
	return false, false
}
