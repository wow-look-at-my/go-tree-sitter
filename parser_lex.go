package treesitter

func (p *Parser) lex(version stackVersion, parseState StateID) subtree {
	lexMode := p.language.lexModeForState(parseState)
	if lexMode.LexState == 0xFFFF {
		return nil
	}

	startPosition := p.stack.position(version)
	externalToken := p.stack.lastExternalToken(version)

	foundExternalToken := false
	errorMode := parseState == errorState
	skippedError := false
	calledGetColumn := false
	var firstErrorCharacter int32
	errorStartPosition := lengthZero()
	errorEndPosition := lengthZero()
	lookaheadEndByte := uint32(0)
	externalScannerStateLen := uint32(0)
	externalScannerStateChanged := false
	p.lexer.reset(startPosition)

	for {
		foundToken := false
		currentPosition := p.lexer.currentPosition
		savedColumnData := p.lexer.columnData

		if lexMode.ExternalLexState != 0 {
			p.lexer.start()
			p.externalScannerDeserialize(externalToken)
			foundToken = p.externalScannerScan(uint32(lexMode.ExternalLexState))
			if p.hasScannerError {
				return nil
			}
			p.lexer.finish(&lookaheadEndByte)

			if foundToken {
				externalScannerStateLen = p.externalScannerSerialize()
				externalScannerStateChanged = !subtreeExternalScannerState(externalToken).
					eq(p.serializationBuffer[:externalScannerStateLen])

				if p.lexer.tokenEndPosition.Bytes <= currentPosition.Bytes &&
					!externalScannerStateChanged {
					symbol := p.language.ExternalScannerSymbol[p.lexer.ResultSymbol]
					nextParseState := p.language.nextState(parseState, symbol)
					tokenIsExtra := nextParseState == parseState
					if errorMode || !p.stack.hasAdvancedSinceError(version) || tokenIsExtra {
						foundToken = false
					}
				}
			}

			if foundToken {
				foundExternalToken = true
				calledGetColumn = p.lexer.didGetColumn
				break
			}

			p.lexer.reset(currentPosition)
			p.lexer.columnData = savedColumnData
		}

		p.lexer.start()
		foundToken = p.language.LexFn(&p.lexer, lexMode.LexState)
		p.lexer.finish(&lookaheadEndByte)
		if foundToken {
			break
		}

		if !errorMode {
			errorMode = true
			lexMode = p.language.lexModeForState(errorState)
			p.lexer.reset(startPosition)
			continue
		}

		if !skippedError {
			skippedError = true
			errorStartPosition = p.lexer.tokenStartPosition
			errorEndPosition = p.lexer.tokenStartPosition
			firstErrorCharacter = p.lexer.Lookahead
		}

		if p.lexer.currentPosition.Bytes == errorEndPosition.Bytes {
			if p.lexer.EOF() {
				p.lexer.ResultSymbol = BuiltinSymError
				break
			}
			p.lexer.Advance(false)
		}

		errorEndPosition = p.lexer.currentPosition
	}

	var result subtree
	if skippedError {
		padding := lengthSub(errorStartPosition, startPosition)
		size := lengthSub(errorEndPosition, errorStartPosition)
		lookaheadBytes := lookaheadEndByte - errorEndPosition.Bytes
		result = newErrorSubtree(
			firstErrorCharacter, padding, size, lookaheadBytes, parseState, p.language,
		)
	} else {
		isKeyword := false
		symbol := p.lexer.ResultSymbol
		padding := lengthSub(p.lexer.tokenStartPosition, startPosition)
		size := lengthSub(p.lexer.tokenEndPosition, p.lexer.tokenStartPosition)
		lookaheadBytes := lookaheadEndByte - p.lexer.tokenEndPosition.Bytes

		if foundExternalToken {
			symbol = p.language.ExternalScannerSymbol[symbol]
		} else if symbol == p.language.KeywordCaptureToken && symbol != 0 {
			endByte := p.lexer.tokenEndPosition.Bytes
			p.lexer.reset(p.lexer.tokenStartPosition)
			p.lexer.start()

			isKeyword = p.language.KeywordLexFn(&p.lexer, 0)

			if isKeyword && p.lexer.tokenEndPosition.Bytes == endByte &&
				(p.language.hasActions(parseState, p.lexer.ResultSymbol) ||
					p.language.isReservedWord(parseState, p.lexer.ResultSymbol)) {
				symbol = p.lexer.ResultSymbol
			}
		}

		result = newLeafSubtree(
			symbol, padding, size, lookaheadBytes, parseState,
			foundExternalToken, calledGetColumn, isKeyword, p.language,
		)

		if foundExternalToken {
			result.scannerState.init(p.serializationBuffer[:externalScannerStateLen])
			result.hasExternalScannerStateChange = externalScannerStateChanged
		}
	}

	return result
}
