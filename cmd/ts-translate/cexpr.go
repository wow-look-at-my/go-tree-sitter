package main

import (
	"fmt"
	"github.com/wow-look-at-my/go-containers/set"
)

var castTypes = set.Of[string]("uint8_t", "uint16_t", "uint32_t", "uint64_t",
	"int8_t", "int16_t", "int32_t", "int64_t",
	"unsigned", "int", "short", "long", "char",
	"TSSymbol", "TSStateId", "TSFieldId", "size_t",
	"bool", "void")

var castWidth = map[string]uint{
	"uint8_t": 8, "uint16_t": 16, "uint32_t": 32,
	"TSSymbol": 16, "TSStateId": 16, "TSFieldId": 16,
}

func (p *cParser) parseConstExpr() int64 {
	return p.parseTernary()
}

func (p *cParser) parseTernary() int64 {
	cond := p.parseBinaryExpr(0)
	if p.accept("?") {
		a := p.parseTernary()
		p.expect(":")
		b := p.parseTernary()
		if cond != 0 {
			return a
		}
		return b
	}
	return cond
}

var precedence = map[string]int{
	"||": 1, "&&": 2, "|": 3, "^": 4, "&": 5,
	"==": 6, "!=": 6,
	"<": 7, ">": 7, "<=": 7, ">=": 7,
	"<<": 8, ">>": 8,
	"+": 9, "-": 9,
	"*": 10, "/": 10, "%": 10,
}

func (p *cParser) parseBinaryExpr(minPrec int) int64 {
	left := p.parseUnaryExpr()
	for {
		t := p.cur()
		if t.kind != tokPunct {
			return left
		}
		prec, ok := precedence[t.text]
		if !ok || prec < minPrec {
			return left
		}
		op := t.text
		p.advance()
		right := p.parseBinaryExpr(prec + 1)
		left = applyBinary(op, left, right)
	}
}

func applyBinary(op string, a, b int64) int64 {
	switch op {
	case "||":
		return boolInt(a != 0 || b != 0)
	case "&&":
		return boolInt(a != 0 && b != 0)
	case "|":
		return a | b
	case "^":
		return a ^ b
	case "&":
		return a & b
	case "==":
		return boolInt(a == b)
	case "!=":
		return boolInt(a != b)
	case "<":
		return boolInt(a < b)
	case ">":
		return boolInt(a > b)
	case "<=":
		return boolInt(a <= b)
	case ">=":
		return boolInt(a >= b)
	case "<<":
		return a << uint(b)
	case ">>":
		return a >> uint(b)
	case "+":
		return a + b
	case "-":
		return a - b
	case "*":
		return a * b
	case "/":
		return a / b
	default:
		return a % b
	}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (p *cParser) parseUnaryExpr() int64 {
	switch {
	case p.accept("-"):
		return -p.parseUnaryExpr()
	case p.accept("+"):
		return p.parseUnaryExpr()
	case p.accept("!"):
		return boolInt(p.parseUnaryExpr() == 0)
	case p.accept("~"):
		return ^p.parseUnaryExpr()
	}
	return p.parsePrimary()
}

func (p *cParser) parsePrimary() int64 {
	t := p.cur()
	switch {
	case t.kind == tokNumber, t.kind == tokChar:
		p.advance()
		return t.val
	case p.at("("):
		save := p.pos
		p.advance()
		if p.cur().kind == tokIdent && castTypes.Contains(p.cur().text) {
			typeName := p.cur().text
			p.advance()
			for p.at("*") {
				p.advance()
			}
			if p.accept(")") {
				v := p.parseUnaryExpr()
				if w, ok := castWidth[typeName]; ok {
					return int64(uint64(v) & ((1 << w) - 1))
				}
				return v
			}
			p.pos = save
			p.advance()
		}
		v := p.parseTernary()
		p.expect(")")
		return v
	case t.kind == tokIdent:
		p.advance()
		if p.at("(") {
			p.advance()
			var args []int64
			for !p.at(")") {
				args = append(args, p.parseTernary())
				if !p.accept(",") {
					break
				}
			}
			p.expect(")")
			return p.applyMacro(t.text, args)
		}
		if v, ok := p.file.consts[t.text]; ok {
			return v
		}
		panic(fmt.Sprintf("line %d: unknown identifier %q", t.line, t.text))
	}
	panic(fmt.Sprintf("line %d: unexpected token %q", t.line, t.text))
}

func (p *cParser) applyMacro(name string, args []int64) int64 {
	switch name {
	case "ACTIONS", "STATE":
		return args[0]
	case "SMALL_STATE":
		return args[0] - p.file.consts["LARGE_STATE_COUNT"]
	}
	panic(fmt.Sprintf("unsupported macro %q", name))
}
