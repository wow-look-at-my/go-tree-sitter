package main

import (
	"fmt"
	"strings"
)

type initValue struct {
	isScalar bool
	num      int64
	isStr    bool
	str      string
	isNull   bool
	isAction bool
	action   actionValue
	elems    []initElem
}

type initElem struct {
	index int
	field string
	value *initValue
}

type actionValue struct {
	kind              string
	state             int64
	symbol            int64
	childCount        int64
	dynamicPrecedence int64
	productionID      int64
	count             int64
	reusable          bool
}

type arrayDecl struct {
	name  string
	ctype string
	dims  []int64
	value *initValue
}

type expr interface{}

type exprBinary struct {
	op   string
	l, r expr
}

type exprUnary struct {
	op string
	x  expr
}

type exprLit struct{ v int64 }

type exprIdent struct{ name string }

type exprSetContains struct {
	set string
	n   int64
}

type lexStmt interface{}

type stmtIf struct {
	cond expr
	then []lexStmt
}

type stmtAdvance struct {
	state int64
	skip  bool
}

type stmtAdvanceMap struct{ pairs [][2]int64 }

type stmtAccept struct{ symbol int64 }

type stmtEndState struct{}

type stmtReturn struct{ value string }

type stmtEOFAssign struct{}

type lexCase struct {
	labels    []int64
	isDefault bool
	stmts     []lexStmt
}

type lexFunc struct {
	name  string
	cases []lexCase
}

type langValue struct {
	isNum bool
	num   int64
	isStr bool
	str   string
	ref   string
	elems map[string]int64
}

type cFile struct {
	consts       map[string]int64
	arrays       map[string]*arrayDecl
	lexFns       map[string]*lexFunc
	lang         map[string]langValue
	languageName string
	hasScanner   bool
}

type cParser struct {
	toks []token
	pos  int
	file *cFile
}

func (p *cParser) cur() token { return p.toks[p.pos] }
func (p *cParser) at(s string) bool {
	t := p.cur()
	return (t.kind == tokPunct || t.kind == tokIdent) && t.text == s
}

func (p *cParser) advance() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *cParser) expect(s string) token {
	if !p.at(s) {
		panic(fmt.Sprintf("line %d: expected %q, found %q", p.cur().line, s, p.cur().text))
	}
	return p.advance()
}

func (p *cParser) accept(s string) bool {
	if p.at(s) {
		p.advance()
		return true
	}
	return false
}

// parseFile reads a generated parser source into a structured form.
func parseFile(toks []token) *cFile {
	p := &cParser{
		toks: toks,
		file: &cFile{
			consts: map[string]int64{},
			arrays: map[string]*arrayDecl{},
			lexFns: map[string]*lexFunc{},
			lang:   map[string]langValue{},
		},
	}
	p.run()
	return p.file
}

func (p *cParser) run() {
	for p.cur().kind != tokEOF {
		t := p.cur()
		switch {
		case t.kind == tokDirective:
			p.parseDirective(t.text)
			p.advance()
		case t.kind == tokIdent && t.text == "enum":
			p.parseEnum()
		case t.kind == tokIdent && t.text == "extern":
			p.advance()
			if p.cur().kind == tokString {
				p.advance()
			}
			p.accept("{")
		case t.kind == tokPunct && (t.text == "}" || t.text == ";"):
			p.advance()
		case t.kind == tokIdent && t.text == "static":
			p.parseStatic()
		case t.kind == tokIdent && (t.text == "TS_PUBLIC" || t.text == "const"):
			p.parseLanguageFunc()
		default:
			p.skipDeclaration()
		}
	}
}

func (p *cParser) parseDirective(text string) {
	fields := strings.Fields(text)
	if len(fields) >= 3 && fields[0] == "#define" {
		if v, err := parseCNumber(fields[2]); err == nil {
			p.file.consts[fields[1]] = v
		}
	}
}

func (p *cParser) parseEnum() {
	p.expect("enum")
	if p.cur().kind == tokIdent {
		p.advance()
	}
	p.expect("{")
	next := int64(0)
	for !p.at("}") {
		name := p.advance().text
		if p.accept("=") {
			next = p.parseConstExpr()
		}
		p.file.consts[name] = next
		next++
		if !p.accept(",") {
			break
		}
	}
	p.expect("}")
	p.accept(";")
}

// skipDeclaration walks past a declaration this translator does not model.
func (p *cParser) skipDeclaration() {
	depth := 0
	for p.cur().kind != tokEOF {
		t := p.advance()
		if t.kind == tokPunct {
			switch t.text {
			case "{", "(":
				depth++
			case "}", ")":
				depth--
				if depth == 0 && t.text == "}" {
					return
				}
			case ";":
				if depth == 0 {
					return
				}
			}
		}
	}
}

func (p *cParser) parseStatic() {
	save := p.pos
	p.expect("static")
	if p.at("inline") {
		p.pos = save
		p.skipDeclaration()
		return
	}
	if p.at("bool") {
		p.advance()
		name := p.cur().text
		if name == "ts_lex" || name == "ts_lex_keywords" {
			p.parseLexFunc(name)
			return
		}
		p.pos = save
		p.skipDeclaration()
		return
	}
	p.accept("const")
	ctype := p.parseTypeName()
	name := p.advance().text
	var dims []int64
	for p.at("[") {
		p.advance()
		if p.at("]") {
			dims = append(dims, 0)
		} else {
			dims = append(dims, p.parseConstExpr())
		}
		p.expect("]")
	}
	if !p.accept("=") {
		p.accept(";")
		return
	}
	value := p.parseInitializer(name)
	p.accept(";")
	p.file.arrays[name] = &arrayDecl{name: name, ctype: ctype, dims: dims, value: value}
}

func (p *cParser) parseTypeName() string {
	var parts []string
	for {
		t := p.cur()
		if t.kind != tokIdent {
			break
		}
		if t.text == "unsigned" || t.text == "signed" || t.text == "const" ||
			t.text == "struct" || t.text == "char" || t.text == "int" ||
			t.text == "short" || t.text == "long" || t.text == "void" ||
			t.text == "bool" || strings.HasPrefix(t.text, "TS") ||
			strings.HasPrefix(t.text, "uint") || strings.HasPrefix(t.text, "int") {
			parts = append(parts, t.text)
			p.advance()
			continue
		}
		break
	}
	for p.at("*") {
		parts = append(parts, "*")
		p.advance()
		p.accept("const")
	}
	return strings.Join(parts, " ")
}

func (p *cParser) parseInitializer(owner string) *initValue {
	if p.at("{") {
		p.advance()
		v := &initValue{}
		for !p.at("}") {
			el := initElem{index: -1}
			switch {
			case p.at("["):
				p.advance()
				el.index = int(p.parseConstExpr())
				p.expect("]")
				p.expect("=")
			case p.at("."):
				p.advance()
				el.field = p.advance().text
				p.expect("=")
			}
			el.value = p.parseInitializer(owner)
			v.elems = append(v.elems, el)
			if !p.accept(",") {
				break
			}
		}
		p.expect("}")
		return v
	}

	t := p.cur()
	if t.kind == tokString {
		p.advance()
		return &initValue{isStr: true, str: t.text}
	}
	if t.kind == tokIdent {
		switch t.text {
		case "NULL":
			p.advance()
			return &initValue{isNull: true}
		case "SHIFT", "SHIFT_REPEAT", "SHIFT_EXTRA", "REDUCE", "RECOVER", "ACCEPT_INPUT":
			return &initValue{isAction: true, action: p.parseActionMacro()}
		}
	}
	return &initValue{isScalar: true, num: p.parseConstExpr()}
}

func (p *cParser) parseActionMacro() actionValue {
	name := p.advance().text
	var args []int64
	p.expect("(")
	for !p.at(")") {
		args = append(args, p.parseConstExpr())
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	switch name {
	case "SHIFT":
		return actionValue{kind: "shift", state: args[0]}
	case "SHIFT_REPEAT":
		return actionValue{kind: "shiftRepeat", state: args[0]}
	case "SHIFT_EXTRA":
		return actionValue{kind: "shiftExtra"}
	case "REDUCE":
		return actionValue{
			kind: "reduce", symbol: args[0], childCount: args[1],
			dynamicPrecedence: args[2], productionID: args[3],
		}
	case "RECOVER":
		return actionValue{kind: "recover"}
	default:
		return actionValue{kind: "accept"}
	}
}

func (p *cParser) parseLanguageFunc() {
	for !p.at("{") && p.cur().kind != tokEOF {
		t := p.advance()
		if t.kind == tokIdent && strings.HasPrefix(t.text, "tree_sitter_") {
			p.file.languageName = strings.TrimPrefix(t.text, "tree_sitter_")
		}
	}
	p.expect("{")
	p.accept("static")
	p.accept("const")
	p.accept("TSLanguage")
	p.accept("language")
	p.expect("=")
	p.expect("{")
	for !p.at("}") {
		p.expect(".")
		field := p.advance().text
		p.expect("=")
		p.file.lang[field] = p.parseLangValue()
		if !p.accept(",") {
			break
		}
	}
	p.expect("}")
	p.accept(";")
	p.skipDeclaration()
}

func (p *cParser) parseLangValue() langValue {
	if p.at("{") {
		if p.toks[p.pos+1].text != "." {
			depth := 0
			for {
				t := p.advance()
				if t.text == "{" {
					depth++
				} else if t.text == "}" {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			p.file.hasScanner = true
			return langValue{ref: "external_scanner"}
		}
		p.advance()
		out := langValue{elems: map[string]int64{}}
		for !p.at("}") {
			p.expect(".")
			field := p.advance().text
			p.expect("=")
			out.elems[field] = p.parseConstExpr()
			if !p.accept(",") {
				break
			}
		}
		p.expect("}")
		return out
	}
	if p.cur().kind == tokString {
		return langValue{isStr: true, str: p.advance().text}
	}
	start := p.pos
	depth := 0
	var name string
	for {
		t := p.cur()
		if t.kind == tokPunct {
			switch t.text {
			case "(", "[":
				depth++
			case ")", "]":
				depth--
			case ",", "}":
				if depth <= 0 {
					goto done
				}
			}
		}
		if t.kind == tokIdent && name == "" && t.text != "const" && t.text != "void" {
			name = t.text
		}
		p.advance()
	}
done:
	if name == "" {
		p.pos = start
		return langValue{isNum: true, num: p.parseConstExpr()}
	}
	if v, ok := p.file.consts[name]; ok {
		return langValue{isNum: true, num: v}
	}
	return langValue{ref: name}
}
