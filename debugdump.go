package treesitter

import (
	"fmt"
	"os"
)

var traceFile *os.File

func tracef(format string, args ...any) {
	if traceFile == nil {
		traceFile, _ = os.Create("/tmp/ts-trace.log")
	}
	if traceFile != nil {
		fmt.Fprintf(traceFile, format, args...)
		traceFile.Sync()
	}
}

// DebugState reports every lookahead the given parse state accepts.
func DebugState(language *Language, state StateID) []string {
	var out []string
	if uint32(state) >= language.LargeStateCount {
		index := language.SmallParseTableMap[uint32(state)-language.LargeStateCount]
		data := language.SmallParseTable[index:]
		out = append(out, fmt.Sprintf("small state=%d index=%d groups=%d", state, index, data[0]))
	}
	iter := language.lookaheads(state)
	for iter.next() {
		var entry tableEntry
		language.tableEntry(state, iter.symbol, &entry)
		kinds := make([]uint8, 0, entry.actionCount)
		for j := uint32(0); j < entry.actionCount; j++ {
			kinds = append(kinds, entry.actions[j].Action.Type)
		}
		out = append(out, fmt.Sprintf("  sym=%d %q value=%d actions=%d kinds=%v next=%d",
			iter.symbol, language.SymbolName(iter.symbol), iter.tableValue,
			entry.actionCount, kinds, iter.nextState))
	}
	return out
}

// DebugFirstTokens reports what the grammar's lexer and tables do at the start
// of a source text. It exists to compare a translated grammar with upstream.
func DebugFirstTokens(language *Language, source []byte, count int) []string {
	p := NewParser()
	if !p.SetLanguage(language) {
		return []string{"the parser refused the language"}
	}
	in := &stringInput{data: source}
	p.lexer.setInput(Input{Read: in.read, Encoding: EncodingUTF8})

	var out []string
	state := p.stack.state(0)
	for i := 0; i < count; i++ {
		mode := language.lexModeForState(state)
		token := p.lex(0, state)
		if token == nil {
			out = append(out, fmt.Sprintf("state=%d mode=%v token=nil", state, mode))
			break
		}
		var entry tableEntry
		language.tableEntry(state, subtreeSymbol(token), &entry)
		kinds := make([]uint8, 0, entry.actionCount)
		next := make([]StateID, 0, entry.actionCount)
		for j := uint32(0); j < entry.actionCount; j++ {
			kinds = append(kinds, entry.actions[j].Action.Type)
			next = append(next, entry.actions[j].Action.State)
		}
		out = append(out, fmt.Sprintf(
			"state=%d mode=%v token=%q sym=%d pad=%d size=%d actions=%d kinds=%v next=%v",
			state, mode, language.SymbolName(subtreeSymbol(token)), subtreeSymbol(token),
			subtreePadding(token).Bytes, subtreeSize(token).Bytes,
			entry.actionCount, kinds, next))
		if entry.actionCount == 0 {
			break
		}
		action := entry.actions[entry.actionCount-1].Action
		if action.Type != ParseActionTypeShift {
			break
		}
		p.stack.push(0, token, false, action.State)
		state = action.State
	}
	return out
}
