package main

import (
	"fmt"

	ts "github.com/wow-look-at-my/go-tree-sitter"
)

// A deeper condition than the runtime's operand stack stops the build here,
// rather than overrun that stack at parse time.
const maxStackDepth = 16

// progBuilder lays a lex function out as a single instruction stream: the
// prologue, a dispatch on the lexer state, then the case bodies.
type progBuilder struct {
	e     *emitter
	code  []ts.LexInstr
	maps  [][]int32
	depth int
	max   int
}

// buildLexProgram turns a parsed lex function into the data the runtime walks.
func (e *emitter) buildLexProgram(fn *lexFunc) *ts.LexProgram {
	if fn == nil {
		return nil
	}
	b := &progBuilder{e: e}
	for _, st := range fn.prologue {
		b.stmt(st)
	}
	b.emit(ts.OpDispatch, 0)

	entry := map[int64]int32{}
	deflt := int32(-1)
	highest := int64(-1)
	for _, c := range fn.cases {
		pc := int32(len(b.code))
		if c.isDefault {
			deflt = pc
		}
		for _, label := range c.labels {
			entry[label] = pc
			if label > highest {
				highest = label
			}
		}
		for _, st := range c.stmts {
			b.stmt(st)
		}
		if !b.endsRun() {
			b.emit(ts.OpEnd, 0)
		}
	}

	states := make([]int32, highest+1)
	for i := range states {
		states[i] = deflt
	}
	for label, pc := range entry {
		states[label] = pc
	}
	if b.max > maxStackDepth {
		panic(fmt.Sprintf("a lex condition nests %d deep, and the runtime stack holds %d",
			b.max, maxStackDepth))
	}
	return &ts.LexProgram{Code: b.code, States: states, Maps: b.maps}
}

// endsRun reports whether the stream already finishes with an answer.
func (b *progBuilder) endsRun() bool {
	if len(b.code) == 0 {
		return false
	}
	switch b.code[len(b.code)-1].Op {
	case ts.OpEnd, ts.OpReturn:
		return true
	}
	return false
}

func (b *progBuilder) emit(op ts.LexOp, arg int32) int {
	b.code = append(b.code, ts.LexInstr{Op: op, Arg: arg})
	return len(b.code) - 1
}

func (b *progBuilder) stmt(st lexStmt) {
	switch v := st.(type) {
	case stmtIf:
		// A bare block reaches here as an always-true condition, and needs no test.
		if lit, ok := v.cond.(exprLit); ok && lit.v == 1 {
			for _, inner := range v.then {
				b.stmt(inner)
			}
			return
		}
		b.depth = 0
		b.cond(v.cond)
		jump := b.emit(ts.OpJumpIfFalse, 0)
		for _, inner := range v.then {
			b.stmt(inner)
		}
		b.code[jump].Arg = int32(len(b.code))

	case stmtAdvance:
		op := ts.OpAdvance
		if v.skip {
			op = ts.OpSkip
		}
		b.emit(op, int32(v.state))

	case stmtAdvanceMap:
		pairs := make([]int32, 0, len(v.pairs)*2)
		for _, pair := range v.pairs {
			pairs = append(pairs, int32(pair[0]), int32(pair[1]))
		}
		b.maps = append(b.maps, pairs)
		b.emit(ts.OpAdvanceMap, int32(len(b.maps)-1))

	case stmtAccept:
		b.emit(ts.OpAccept, int32(v.symbol))

	case stmtEndState:
		b.emit(ts.OpEnd, 0)

	case stmtReturn:
		switch v.value {
		case "result":
			b.emit(ts.OpEnd, 0)
		case "false":
			b.emit(ts.OpReturn, 0)
		case "true":
			b.emit(ts.OpReturn, 1)
		default:
			panic(fmt.Sprintf("a lex function returns %q, which is not a value this translator knows", v.value))
		}

	case stmtEOFAssign:
		b.emit(ts.OpReadEOF, 0)

	default:
		panic(fmt.Sprintf("unsupported statement %T in a lex function", st))
	}
}

// cond lays a condition out in postfix, which is what the runtime evaluates.
func (b *progBuilder) cond(x expr) {
	switch v := x.(type) {
	case exprLit:
		b.push(ts.OpPushLit, int32(v.v))
	case exprIdent:
		switch v.name {
		case "lookahead":
			b.push(ts.OpPushLookahead, 0)
		case "eof":
			b.push(ts.OpPushEOF, 0)
		default:
			panic(fmt.Sprintf("unknown identifier %q in a lex condition", v.name))
		}
	case exprSetContains:
		b.push(ts.OpPushSet, int32(b.e.setIndex[v.set]))
	case exprUnary:
		b.cond(v.x)
		switch v.op {
		case "!":
			b.emit(ts.OpNot, 0)
		case "-":
			b.emit(ts.OpNeg, 0)
		default:
			panic(fmt.Sprintf("unknown unary operator %q in a lex condition", v.op))
		}
	case exprBinary:
		b.cond(v.l)
		b.cond(v.r)
		op, ok := binaryOps[v.op]
		if !ok {
			panic(fmt.Sprintf("unknown binary operator %q in a lex condition", v.op))
		}
		b.emit(ts.OpBinary, op)
		b.depth--
	default:
		panic(fmt.Sprintf("unsupported expression %T in a lex condition", x))
	}
}

// push emits an instruction that grows the operand stack, and tracks how deep
// the stack gets.
func (b *progBuilder) push(op ts.LexOp, arg int32) {
	b.emit(op, arg)
	b.depth++
	if b.depth > b.max {
		b.max = b.depth
	}
}

var binaryOps = map[string]int32{
	"||": ts.BinOr, "&&": ts.BinAnd, "|": ts.BinBitOr, "^": ts.BinXor, "&": ts.BinBitAnd,
	"==": ts.BinEq, "!=": ts.BinNe, "<": ts.BinLt, ">": ts.BinGt, "<=": ts.BinLe, ">=": ts.BinGe,
	"<<": ts.BinShl, ">>": ts.BinShr,
	"+": ts.BinAdd, "-": ts.BinSub, "*": ts.BinMul, "/": ts.BinDiv, "%": ts.BinMod,
}
