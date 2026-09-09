package treesitter

// A grammar's lexer is a state machine, and it travels as data in the same blob
// as the parse tables. Translating it into Go source instead would put a
// megabyte of generated code in the tree for every grammar, and the tree is for
// source somebody wrote.

// LexOp is one instruction of a lexer program. The condition ops push onto an
// operand stack and the rest act on the lexer.
type LexOp uint8

const (
	// OpPushLit pushes Arg.
	OpPushLit LexOp = iota
	// OpPushLookahead pushes the code point at the current position.
	OpPushLookahead
	// OpPushEOF pushes the end-of-input flag this iteration read.
	OpPushEOF
	// OpPushSet pushes whether character set Arg holds the lookahead.
	OpPushSet
	// OpNot and OpNeg replace the top of the stack.
	OpNot
	OpNeg
	// OpBinary combines the top two values with binary operator Arg.
	OpBinary
	// OpJumpIfFalse pops, and jumps to Arg when the value is zero.
	OpJumpIfFalse
	// OpDispatch jumps to the body of the case for the current state.
	OpDispatch
	// OpAdvance and OpSkip move to state Arg. Skip drops what it consumed.
	OpAdvance
	OpSkip
	// OpAdvanceMap reads map Arg, and advances on a lookahead the map names.
	OpAdvanceMap
	// OpAccept marks the token Arg, ending here.
	OpAccept
	// OpEnd answers with what the run accepted. OpReturn answers Arg.
	OpEnd
	OpReturn
	// OpReadEOF reads the end-of-input flag.
	OpReadEOF
)

// The binary operators a lex condition uses, as OpBinary's Arg.
const (
	BinOr int32 = iota
	BinAnd
	BinBitOr
	BinXor
	BinBitAnd
	BinEq
	BinNe
	BinLt
	BinGt
	BinLe
	BinGe
	BinShl
	BinShr
	BinAdd
	BinSub
	BinMul
	BinDiv
	BinMod
)

// LexInstr is one instruction and its single argument.
type LexInstr struct {
	Op  LexOp
	Arg int32
}

// LexProgram is a lexer state machine. Code opens with the prologue and an
// OpDispatch, and the case bodies follow it. States gives the entry point of
// each lexer state, and an entry of -1 answers with what the run accepted.
type LexProgram struct {
	Code   []LexInstr
	States []int32
	Maps   [][]int32
}

// Run walks the program for one token. It answers whether the run accepted one,
// which is what a grammar's LexFn answers.
func (p *LexProgram) Run(sets [][]CharacterRange, lexer *Lexer, state StateID) bool {
	result := false
	var stack [16]int64
restart:
	for {
		sp := 0
		eof := false
		lookahead := lexer.Lookahead
		pc := 0
		for {
			ins := p.Code[pc]
			pc++
			switch ins.Op {
			case OpPushLit:
				stack[sp] = int64(ins.Arg)
				sp++
			case OpPushLookahead:
				stack[sp] = int64(lookahead)
				sp++
			case OpPushEOF:
				stack[sp] = boolInt(eof)
				sp++
			case OpPushSet:
				stack[sp] = boolInt(SetContains(sets[ins.Arg], lookahead))
				sp++
			case OpNot:
				stack[sp-1] = boolInt(stack[sp-1] == 0)
			case OpNeg:
				stack[sp-1] = -stack[sp-1]
			case OpBinary:
				sp--
				stack[sp-1] = applyBinary(ins.Arg, stack[sp-1], stack[sp])
			case OpJumpIfFalse:
				sp--
				if stack[sp] == 0 {
					pc = int(ins.Arg)
				}
			case OpDispatch:
				target := int32(-1)
				if int(state) < len(p.States) {
					target = p.States[state]
				}
				if target < 0 {
					return result
				}
				pc = int(target)
			case OpAdvance:
				state = StateID(ins.Arg)
				lexer.Advance(false)
				continue restart
			case OpSkip:
				state = StateID(ins.Arg)
				lexer.Advance(true)
				continue restart
			case OpAdvanceMap:
				pairs := p.Maps[ins.Arg]
				for i := 0; i < len(pairs); i += 2 {
					if pairs[i] == lookahead {
						state = StateID(pairs[i+1])
						lexer.Advance(false)
						continue restart
					}
				}
			case OpAccept:
				result = true
				lexer.ResultSymbol = Symbol(ins.Arg)
				lexer.MarkEnd()
			case OpEnd:
				return result
			case OpReturn:
				return ins.Arg != 0
			case OpReadEOF:
				eof = lexer.EOF()
			}
		}
	}
}

func applyBinary(op int32, l, r int64) int64 {
	switch op {
	case BinOr:
		return boolInt(l != 0 || r != 0)
	case BinAnd:
		return boolInt(l != 0 && r != 0)
	case BinBitOr:
		return l | r
	case BinXor:
		return l ^ r
	case BinBitAnd:
		return l & r
	case BinEq:
		return boolInt(l == r)
	case BinNe:
		return boolInt(l != r)
	case BinLt:
		return boolInt(l < r)
	case BinGt:
		return boolInt(l > r)
	case BinLe:
		return boolInt(l <= r)
	case BinGe:
		return boolInt(l >= r)
	case BinShl:
		return l << uint(r)
	case BinShr:
		return l >> uint(r)
	case BinAdd:
		return l + r
	case BinSub:
		return l - r
	case BinMul:
		return l * r
	case BinDiv:
		return l / r
	case BinMod:
		return l % r
	}
	return 0
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
