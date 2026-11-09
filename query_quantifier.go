// query_quantifier.go holds the capture quantifier algebra of query.c (v0.27.2).

package treesitter

// quantifierMul is quantifier_mul.
func quantifierMul(left, right Quantifier) Quantifier {
	switch left {
	case QuantifierZero:
		return QuantifierZero
	case QuantifierZeroOrOne:
		switch right {
		case QuantifierZero:
			return QuantifierZero
		case QuantifierZeroOrOne, QuantifierOne:
			return QuantifierZeroOrOne
		case QuantifierZeroOrMore, QuantifierOneOrMore:
			return QuantifierZeroOrMore
		}
	case QuantifierZeroOrMore:
		if right == QuantifierZero {
			return QuantifierZero
		}
		return QuantifierZeroOrMore
	case QuantifierOne:
		return right
	case QuantifierOneOrMore:
		switch right {
		case QuantifierZero:
			return QuantifierZero
		case QuantifierZeroOrOne, QuantifierZeroOrMore:
			return QuantifierZeroOrMore
		case QuantifierOne, QuantifierOneOrMore:
			return QuantifierOneOrMore
		}
	}
	return QuantifierZero
}

// quantifierJoin is quantifier_join.
func quantifierJoin(left, right Quantifier) Quantifier {
	switch left {
	case QuantifierZero:
		switch right {
		case QuantifierZero:
			return QuantifierZero
		case QuantifierZeroOrOne, QuantifierOne:
			return QuantifierZeroOrOne
		case QuantifierZeroOrMore, QuantifierOneOrMore:
			return QuantifierZeroOrMore
		}
	case QuantifierZeroOrOne:
		switch right {
		case QuantifierZero, QuantifierZeroOrOne, QuantifierOne:
			return QuantifierZeroOrOne
		case QuantifierZeroOrMore, QuantifierOneOrMore:
			return QuantifierZeroOrMore
		}
	case QuantifierZeroOrMore:
		return QuantifierZeroOrMore
	case QuantifierOne:
		switch right {
		case QuantifierZero, QuantifierZeroOrOne:
			return QuantifierZeroOrOne
		case QuantifierZeroOrMore:
			return QuantifierZeroOrMore
		case QuantifierOne:
			return QuantifierOne
		case QuantifierOneOrMore:
			return QuantifierOneOrMore
		}
	case QuantifierOneOrMore:
		switch right {
		case QuantifierZero, QuantifierZeroOrOne, QuantifierZeroOrMore:
			return QuantifierZeroOrMore
		case QuantifierOne, QuantifierOneOrMore:
			return QuantifierOneOrMore
		}
	}
	return QuantifierZero
}

// quantifierAdd is quantifier_add.
func quantifierAdd(left, right Quantifier) Quantifier {
	switch left {
	case QuantifierZero:
		return right
	case QuantifierZeroOrOne:
		switch right {
		case QuantifierZero:
			return QuantifierZeroOrOne
		case QuantifierZeroOrOne, QuantifierZeroOrMore:
			return QuantifierZeroOrMore
		case QuantifierOne, QuantifierOneOrMore:
			return QuantifierOneOrMore
		}
	case QuantifierZeroOrMore:
		switch right {
		case QuantifierZero, QuantifierZeroOrOne, QuantifierZeroOrMore:
			return QuantifierZeroOrMore
		case QuantifierOne, QuantifierOneOrMore:
			return QuantifierOneOrMore
		}
	case QuantifierOne:
		if right == QuantifierZero {
			return QuantifierOne
		}
		return QuantifierOneOrMore
	case QuantifierOneOrMore:
		return QuantifierOneOrMore
	}
	return QuantifierZero
}

// captureQuantifiers is CaptureQuantifiers: a quantifier per capture id.
type captureQuantifiers []uint8

// forID is capture_quantifier_for_id.
func (c captureQuantifiers) forID(id uint16) Quantifier {
	if int(id) >= len(c) {
		return QuantifierZero
	}
	return Quantifier(c[id])
}

// growTo pads the list with QuantifierZero up to n entries.
func (c *captureQuantifiers) growTo(n int) {
	for len(*c) < n {
		*c = append(*c, uint8(QuantifierZero))
	}
}

// addForID is capture_quantifiers_add_for_id.
func (c *captureQuantifiers) addForID(id uint16, q Quantifier) {
	c.growTo(int(id) + 1)
	(*c)[id] = uint8(quantifierAdd(Quantifier((*c)[id]), q))
}

// addAll is capture_quantifiers_add_all.
func (c *captureQuantifiers) addAll(other captureQuantifiers) {
	c.growTo(len(other))
	for id, q := range other {
		(*c)[id] = uint8(quantifierAdd(Quantifier((*c)[id]), Quantifier(q)))
	}
}

// mul is capture_quantifiers_mul.
func (c captureQuantifiers) mul(q Quantifier) {
	for id := range c {
		c[id] = uint8(quantifierMul(Quantifier(c[id]), q))
	}
}

// joinAll is capture_quantifiers_join_all.
func (c *captureQuantifiers) joinAll(other captureQuantifiers) {
	c.growTo(len(other))
	for id, q := range other {
		(*c)[id] = uint8(quantifierJoin(Quantifier((*c)[id]), Quantifier(q)))
	}
	for id := len(other); id < len(*c); id++ {
		(*c)[id] = uint8(quantifierJoin(Quantifier((*c)[id]), QuantifierZero))
	}
}

// replace is capture_quantifiers_replace.
func (c *captureQuantifiers) replace(other captureQuantifiers) {
	*c = append((*c)[:0], other...)
}
