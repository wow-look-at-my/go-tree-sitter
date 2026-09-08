package main

import (
	"fmt"
	"strings"
)

func goExpr(e expr) string {
	switch v := e.(type) {
	case exprLit:
		return fmt.Sprintf("%d", v.v)
	case exprIdent:
		return v.name
	case exprSetContains:
		return fmt.Sprintf("ts.SetContains(%s, lookahead)", goIdent(v.set))
	case exprUnary:
		return "(" + v.op + goExpr(v.x) + ")"
	case exprBinary:
		return "(" + goExpr(v.l) + " " + v.op + " " + goExpr(v.r) + ")"
	}
	panic("unsupported expression")
}

func (e *emitter) emitLexFunc(goName string, fn *lexFunc) {
	e.printf("func %s(lexer *ts.Lexer, state ts.StateID) bool {\n", goName)
	e.printf("\tresult := false\n")
	e.printf("\tskip := false\n")
	e.printf("\teof := false\n")
	e.printf("\tvar lookahead int32\n")
	e.printf("\t_, _ = eof, lookahead\n")
	e.printf("\tgoto start\n")
	e.printf("nextState:\n")
	e.printf("\tlexer.Advance(skip)\n")
	e.printf("start:\n")
	e.printf("\tskip = false\n")
	e.printf("\tlookahead = lexer.Lookahead\n")
	for _, st := range fn.prologue {
		e.emitLexStmt(st, 1)
	}
	e.printf("\tswitch state {\n")
	for _, c := range fn.cases {
		if c.isDefault {
			e.printf("\tdefault:\n")
		} else {
			labels := make([]string, len(c.labels))
			for i, l := range c.labels {
				labels[i] = fmt.Sprintf("%d", l)
			}
			e.printf("\tcase %s:\n", strings.Join(labels, ", "))
		}
		for _, st := range c.stmts {
			e.emitLexStmt(st, 2)
		}
		if !endsRun(c.stmts) {
			e.printf("\t\treturn result\n")
		}
	}
	e.printf("\t}\n")
	e.printf("}\n\n")
}

// endsRun reports whether a case body already finishes with a return.
func endsRun(stmts []lexStmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch stmts[len(stmts)-1].(type) {
	case stmtEndState, stmtReturn:
		return true
	}
	return false
}

func (e *emitter) emitLexStmt(st lexStmt, depth int) {
	pad := strings.Repeat("\t", depth)
	switch v := st.(type) {
	case stmtIf:
		if lit, ok := v.cond.(exprLit); ok && lit.v == 1 {
			for _, inner := range v.then {
				e.emitLexStmt(inner, depth)
			}
			return
		}
		e.printf("%sif %s {\n", pad, goExpr(v.cond))
		for _, inner := range v.then {
			e.emitLexStmt(inner, depth+1)
		}
		e.printf("%s}\n", pad)
	case stmtAdvance:
		if v.skip {
			e.printf("%sskip = true\n", pad)
		}
		e.printf("%sstate = %d\n", pad, v.state)
		e.printf("%sgoto nextState\n", pad)
	case stmtAdvanceMap:
		e.printf("%sswitch lookahead {\n", pad)
		for _, pair := range v.pairs {
			e.printf("%scase %d:\n", pad, pair[0])
			e.printf("%s\tstate = %d\n", pad, pair[1])
			e.printf("%s\tgoto nextState\n", pad)
		}
		e.printf("%s}\n", pad)
	case stmtAccept:
		e.printf("%sresult = true\n", pad)
		e.printf("%slexer.ResultSymbol = %d\n", pad, v.symbol)
		e.printf("%slexer.MarkEnd()\n", pad)
	case stmtEndState:
		e.printf("%sreturn result\n", pad)
	case stmtReturn:
		e.printf("%sreturn %s\n", pad, v.value)
	case stmtEOFAssign:
		e.printf("%seof = lexer.EOF()\n", pad)
	default:
		panic("unsupported statement")
	}
}
