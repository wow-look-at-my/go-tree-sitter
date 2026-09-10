package treesitter

// LoadGrammar decodes a grammar's table blob and wires its lexer. A grammar
// package holds the blob, its external scanner when it has one, and a call to
// this. Pass a nil scanner for a grammar with no external tokens.
//
// It panics on a blob this runtime cannot read: a grammar that parses nothing
// is worse to hand back than a stop at the point the fault is visible.
func LoadGrammar(name string, blob []byte, scanner ExternalScanner) *Language {
	tables, err := DecodeTables(blob)
	if err != nil {
		panic(name + ": reading the grammar tables: " + err.Error())
	}
	language := &tables.Language
	sets := tables.CharacterSets
	if p := tables.Lex; p != nil {
		language.LexFn = func(l *Lexer, s StateID) bool { return p.Run(sets, l, s) }
	}
	if p := tables.KeywordLex; p != nil {
		language.KeywordLexFn = func(l *Lexer, s StateID) bool { return p.Run(sets, l, s) }
	}
	if scanner != nil {
		language.Scanner = scanner
	}
	return language
}
