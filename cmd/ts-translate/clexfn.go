package main

import "fmt"

func (p *cParser) parseLexFunc(name string) {
	p.advance()
	p.expect("(")
	for !p.at(")") {
		p.advance()
	}
	p.expect(")")
	p.expect("{")

	fn := &lexFunc{name: name}
	p.expect("START_LEXER")
	p.expect("(")
	p.expect(")")
	p.accept(";")

	for !p.at("switch") {
		p.parseLexStmt()
	}

	p.expect("switch")
	p.expect("(")
	p.expect("state")
	p.expect(")")
	p.expect("{")

	for !p.at("}") {
		fn.cases = append(fn.cases, p.parseLexCase())
	}
	p.expect("}")
	p.expect("}")

	p.file.lexFns[name] = fn
}

func (p *cParser) parseLexCase() lexCase {
	c := lexCase{}
	for {
		switch {
		case p.accept("case"):
			c.labels = append(c.labels, p.parseConstExpr())
			p.expect(":")
		case p.accept("default"):
			c.isDefault = true
			p.expect(":")
		default:
			goto body
		}
	}
body:
	for !p.at("case") && !p.at("default") && !p.at("}") {
		st := p.parseLexStmt()
		if st != nil {
			c.stmts = append(c.stmts, st)
		}
	}
	return c
}

func (p *cParser) parseLexStmt() lexStmt {
	switch {
	case p.at("{"):
		p.advance()
		var out []lexStmt
		for !p.at("}") {
			if st := p.parseLexStmt(); st != nil {
				out = append(out, st)
			}
		}
		p.expect("}")
		return stmtIf{cond: exprLit{v: 1}, then: out}

	case p.accept("if"):
		p.expect("(")
		cond := p.parseLexExpr(0)
		p.expect(")")
		var then []lexStmt
		if st := p.parseLexStmt(); st != nil {
			then = append(then, st)
		}
		if p.accept("else") {
			panic("an else branch is not supported in a generated lex function")
		}
		return stmtIf{cond: cond, then: then}

	case p.accept("ADVANCE"):
		p.expect("(")
		state := p.parseConstExpr()
		p.expect(")")
		p.accept(";")
		return stmtAdvance{state: state}

	case p.accept("SKIP"):
		p.expect("(")
		state := p.parseConstExpr()
		p.expect(")")
		p.accept(";")
		return stmtAdvance{state: state, skip: true}

	case p.accept("ADVANCE_MAP"):
		p.expect("(")
		var pairs [][2]int64
		for !p.at(")") {
			key := p.parseConstExpr()
			p.expect(",")
			target := p.parseConstExpr()
			pairs = append(pairs, [2]int64{key, target})
			if !p.accept(",") {
				break
			}
		}
		p.expect(")")
		p.accept(";")
		return stmtAdvanceMap{pairs: pairs}

	case p.accept("ACCEPT_TOKEN"):
		p.expect("(")
		symbol := p.parseConstExpr()
		p.expect(")")
		p.accept(";")
		return stmtAccept{symbol: symbol}

	case p.accept("END_STATE"):
		p.expect("(")
		p.expect(")")
		p.accept(";")
		return stmtEndState{}

	case p.accept("return"):
		value := "result"
		if p.cur().kind == tokIdent && p.toks[p.pos+1].text == ";" {
			value = p.cur().text
		}
		for !p.at(";") {
			p.advance()
		}
		p.expect(";")
		return stmtReturn{value: value}

	case p.at("eof"):
		p.advance()
		p.expect("=")
		for !p.at(";") {
			p.advance()
		}
		p.expect(";")
		return stmtEOFAssign{}
	}

	panic(fmt.Sprintf("line %d: unsupported statement at %q", p.cur().line, p.cur().text))
}

func (p *cParser) parseLexExpr(minPrec int) expr {
	left := p.parseLexUnary()
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
		right := p.parseLexExpr(prec + 1)
		left = exprBinary{op: op, l: left, r: right}
	}
}

func (p *cParser) parseLexUnary() expr {
	switch {
	case p.accept("!"):
		return exprUnary{op: "!", x: p.parseLexUnary()}
	case p.accept("-"):
		return exprUnary{op: "-", x: p.parseLexUnary()}
	case p.accept("+"):
		return p.parseLexUnary()
	}
	return p.parseLexPrimary()
}

func (p *cParser) parseLexPrimary() expr {
	t := p.cur()
	switch {
	case t.kind == tokNumber, t.kind == tokChar:
		p.advance()
		return exprLit{v: t.val}

	case p.at("("):
		save := p.pos
		p.advance()
		if p.cur().kind == tokIdent && castTypes.Contains(p.cur().text) {
			p.advance()
			for p.at("*") {
				p.advance()
			}
			if p.accept(")") {
				return p.parseLexUnary()
			}
			p.pos = save
			p.advance()
		}
		v := p.parseLexExpr(0)
		p.expect(")")
		return v

	case t.kind == tokIdent:
		p.advance()
		switch t.text {
		case "lookahead", "eof":
			return exprIdent{name: t.text}
		case "set_contains":
			p.expect("(")
			set := p.advance().text
			p.expect(",")
			n := p.parseConstExpr()
			p.expect(",")
			p.expect("lookahead")
			p.expect(")")
			return exprSetContains{set: set, n: n}
		}
		if v, ok := p.file.consts[t.text]; ok {
			return exprLit{v: v}
		}
		panic(fmt.Sprintf("line %d: unknown identifier %q in a lex condition", t.line, t.text))
	}
	panic(fmt.Sprintf("line %d: unexpected token %q in a lex condition", t.line, t.text))
}
