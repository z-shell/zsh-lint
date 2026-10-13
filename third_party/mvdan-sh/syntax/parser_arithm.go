package syntax

// compact specifies whether we allow spaces between expressions.
// This is true for let
func (p *Parser) arithmExpr(compact bool) ArithmExpr {
	return p.arithmExprComma(compact)
}

// These function names are inspired by Bash's expr.c

func (p *Parser) arithmExprComma(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprAssign, Comma)
}

func (p *Parser) arithmExprAssign(compact bool) ArithmExpr {
	// Assign is different from the other binary operators because it's
	// right-associative and needs to check that it's placed after a name
	value := p.arithmExprTernary(compact)
	switch BinAritOperator(p.tok) {
	case AddAssgn, SubAssgn, MulAssgn, QuoAssgn, RemAssgn, AndAssgn,
		OrAssgn, XorAssgn, ShlAssgn, ShrAssgn, Assgn,
		AndBoolAssgn, OrBoolAssgn, XorBoolAssgn, PowAssgn:
		if compact && p.spaced {
			return value
		}
		if !p.isArithName(value) {
			p.posErr(p.pos, "%#q must follow a name", p.tok)
		}
		pos := p.pos
		tok := p.tok
		p.nextArithOp(compact)
		y := p.arithmExprAssign(compact)
		if y == nil {
			p.followErrExp(pos, tok)
		}
		return &BinaryArithm{
			OpPos: pos,
			Op:    BinAritOperator(tok),
			X:     value,
			Y:     y,
		}
	}
	return value
}

func (p *Parser) arithmExprTernary(compact bool) ArithmExpr {
	value := p.arithmExprLor(compact)
	if BinAritOperator(p.tok) != TernQuest || (compact && p.spaced) {
		return value
	}

	if value == nil {
		p.curErr("%#q must follow an expression", p.tok)
	}
	questPos := p.pos
	p.nextArithOp(compact)
	if BinAritOperator(p.tok) == TernColon {
		p.followErrExp(questPos, TernQuest)
	}
	trueExpr := p.arithmExpr(compact)
	if trueExpr == nil {
		p.followErrExp(questPos, TernQuest)
	}
	if BinAritOperator(p.tok) != TernColon {
		p.posErr(questPos, "ternary operator missing %#q after %#q", colon, quest)
	}
	colonPos := p.pos
	p.nextArithOp(compact)
	falseExpr := p.arithmExprTernary(compact)
	if falseExpr == nil {
		p.followErrExp(colonPos, TernColon)
	}
	return &BinaryArithm{
		OpPos: questPos,
		Op:    TernQuest,
		X:     value,
		Y: &BinaryArithm{
			OpPos: colonPos,
			Op:    TernColon,
			X:     trueExpr,
			Y:     falseExpr,
		},
	}
}

func (p *Parser) arithmExprLor(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprLand, OrArit, XorBool)
}

func (p *Parser) arithmExprLand(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprBor, AndArit)
}

func (p *Parser) arithmExprBor(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprBxor, Or)
}

func (p *Parser) arithmExprBxor(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprBand, Xor)
}

func (p *Parser) arithmExprBand(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprEquality, And)
}

func (p *Parser) arithmExprEquality(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprComparison, Eql, Neq)
}

func (p *Parser) arithmExprComparison(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprShift, Lss, Gtr, Leq, Geq)
}

func (p *Parser) arithmExprShift(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprAddition, Shl, Shr)
}

func (p *Parser) arithmExprAddition(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprMultiplication, Add, Sub)
}

func (p *Parser) arithmExprMultiplication(compact bool) ArithmExpr {
	return p.arithmExprBinary(compact, p.arithmExprPower, Mul, Quo, Rem)
}

func (p *Parser) arithmExprPower(compact bool) ArithmExpr {
	// Power is different from the other binary operators because it's right-associative
	value := p.arithmExprDynamic(compact)
	if BinAritOperator(p.tok) != Pow || (compact && p.spaced) {
		return value
	}

	if value == nil {
		p.curErr("%#q must follow an expression", p.tok)
	}

	op := p.tok
	pos := p.pos
	p.nextArithOp(compact)
	y := p.arithmExprPower(compact)
	if y == nil {
		p.followErrExp(pos, op)
	}
	return &BinaryArithm{
		OpPos: pos,
		Op:    BinAritOperator(op),
		X:     value,
		Y:     y,
	}
}

func (p *Parser) arithmExprUnary(compact bool) ArithmExpr {
	if !compact {
		p.got(_Newl)
	}

	switch UnAritOperator(p.tok) {
	case Not, BitNegation, Plus, Minus:
		ue := &UnaryArithm{OpPos: p.pos, Op: UnAritOperator(p.tok)}
		p.nextArithOp(compact)
		if ue.X = p.arithmExprUnary(compact); ue.X == nil {
			p.followErrExp(ue.OpPos, ue.Op)
		}
		return ue
	}
	return p.arithmExprValue(compact)
}

// arithmExprDynamic reads Zsh operands joined only by blanks, as in
// $(( 1 $op 2 )): Zsh substitutes expansions before it evaluates the text, so
// an expansion at a blank may supply the operator or operand between two
// others. Each blank needs an expansion on one side of it; `1 2` stays an
// error, with the message and position upstream gives it.
func (p *Parser) arithmExprDynamic(compact bool) ArithmExpr {
	x := p.arithmExprUnary(compact)
	if x == nil || !p.zshExpandedArithm() {
		return x
	}
	var seq *DynamicArithm
	for last := x; ; {
		open := arithmExpansionRight(last)
		if !(open && p.arithmOperandStart()) && !p.arithmExpansionStart() {
			break
		}
		pos, tok := p.pos, p.tok
		y := p.arithmExprUnary(compact)
		if y == nil { // an error inside the operand is already recorded
			break
		}
		if !open && !arithmExpansionLeft(y) {
			p.posErr(pos, "not a valid arithmetic operator: %#q", tok)
		}
		if seq == nil {
			seq = &DynamicArithm{Items: []ArithmExpr{x}}
		}
		seq.Items = append(seq.Items, y)
		last = y
	}
	if seq == nil {
		return x
	}
	return seq
}

// arithmOperandStart reports whether the current token can begin an operand
// after an expansion that may supply an operator.
func (p *Parser) arithmOperandStart() bool {
	switch p.tok {
	case _Lit, _LitWord, leftParen, exclMark, tilde, hash, dollDblParen:
		return true
	}
	return p.arithmExpansionStart()
}

// arithmExpansionStart reports whether the current token can begin a word
// whose first part is an expansion. Zsh keeps single quotes and $'...' in
// arithmetic text, where they are errors, so those do not count.
func (p *Parser) arithmExpansionStart() bool {
	switch p.tok {
	case dollar, dollBrace, dollParen, bckQuote, dblQuote:
		return true
	}
	return false
}

// arithmExpansionRight reports whether x ends in an expansion whose value may
// supply text after it. An output format such as [#16] adds no text.
func arithmExpansionRight(x ArithmExpr) bool {
	switch x := x.(type) {
	case *Word:
		return expansionPart(x.Parts[len(x.Parts)-1], false)
	case *UnaryArithm:
		return !x.Post && arithmExpansionRight(x.X)
	case *OutputBaseArithm:
		return arithmExpansionRight(x.X)
	}
	return false
}

// arithmExpansionLeft reports whether x begins with an expansion whose value
// may supply text before it.
func arithmExpansionLeft(x ArithmExpr) bool {
	switch x := x.(type) {
	case *Word:
		return expansionPart(x.Parts[0], true)
	case *UnaryArithm:
		return x.Post && arithmExpansionLeft(x.X)
	case *OutputBaseArithm:
		return arithmExpansionLeft(x.X)
	}
	return false
}

// expansionPart reports whether a word part is an expansion whose value is
// unknown text, looking into the first or last part of a double-quoted string.
// An arithmetic expansion yields a number, never an operator.
func expansionPart(part WordPart, first bool) bool {
	switch part := part.(type) {
	case *ParamExp:
		return part.Dollar.IsValid() // not a naked a[i] name
	case *CmdSubst:
		return true
	case *DblQuoted:
		if len(part.Parts) == 0 {
			return false
		}
		if first {
			return expansionPart(part.Parts[0], first)
		}
		return expansionPart(part.Parts[len(part.Parts)-1], first)
	}
	return false
}

func (p *Parser) arithmExprValue(compact bool) ArithmExpr {
	var x ArithmExpr
	switch p.tok {
	case addAdd, subSub:
		ue := &UnaryArithm{OpPos: p.pos, Op: UnAritOperator(p.tok)}
		p.nextArith(compact)
		literal := p.tok == _LitWord
		if !literal && !p.zshExpandedArithm() {
			p.followErr(ue.OpPos, ue.Op, noQuote("a literal"))
		}
		ue.X = p.arithmExprValue(compact)
		if !literal && !p.isArithName(ue.X) {
			p.followErr(ue.OpPos, ue.Op, noQuote("a literal"))
		}
		return ue
	case leftParen:
		if p.quote == paramExpArithm && p.lang.in(LangZsh) {
			x = p.zshSubFlags()
			break
		}
		pe := &ParenArithm{Lparen: p.pos}
		p.nextArithOp(compact)
		pe.X = p.followArithm(leftParen, pe.Lparen)
		pe.Rparen = p.matched(pe.Lparen, leftParen, rightParen)
		if p.quote == paramExpArithm && p.tok == _LitWord {
			p.checkLang(pe.Lparen, LangZsh, "subscript flags")
		}
		x = pe
	case leftBrack:
		if p.zshArithmFormat() {
			b := p.outputBaseArithm()
			b.X = p.arithmExprUnary(compact)
			return b
		}
		p.curErr("%#q must follow a name like a[i]", p.tok)
	case hash:
		if !p.zshExpandedArithm() {
			return nil
		}
		c := &CharArithm{Hash: p.pos, Literal: p.r == '#'}
		if c.Literal {
			p.rune()
			if p.r != '$' && p.r != '`' {
				c.Value = p.zshArithmCharacter()
				p.next()
				x = c
				break
			}
		}
		p.next()
		c.Value = p.getWord()
		if !c.Literal && p.tok == leftBrack && c.Value != nil && !p.spaced {
			if lit := c.Value.Lit(); lit != "" {
				pe := &ParamExp{Short: true, Param: c.Value.Parts[0].(*Lit)}
				pe.Index = p.eitherIndex()
				c.Value = p.wordOne(pe)
			}
		}
		x = c
	case colon:
		p.curErr("ternary operator missing %#q before %#q", quest, colon)
	case _LitWord:
		l := p.getLit()
		if p.tok != leftBrack || p.zshArithmFormat() && (p.spaced || !ValidName(l.Value)) {
			x = p.wordOne(l)
			break
		}
		pe := &ParamExp{Short: true, Param: l}
		pe.Index = p.eitherIndex()
		x = p.wordOne(pe)
	case bckQuote:
		if p.quote == arithmExprLet && p.openBquotes > 0 {
			return nil
		}
		fallthrough
	default:
		if w := p.getWord(); w != nil {
			x = w
			if p.zshExpandedArithm() && !p.spaced && p.tok == leftBrack {
				pe := &ParamExp{Short: true, ArithmName: w}
				pe.Index = p.eitherIndex()
				x = p.wordOne(pe)
			}
		} else {
			return nil
		}
	}
	for p.zshArithmFormat() {
		b := p.outputBaseArithm()
		b.Post, b.X = true, x
		x = b
	}

	if compact && p.spaced {
		return x
	}
	if !compact {
		p.got(_Newl)
	}

	// we want real nil, not (*Word)(nil) as that
	// sets the type to non-nil and then x != nil
	if p.tok == addAdd || p.tok == subSub {
		if !p.isArithName(x) {
			p.curErr("%#q must follow a name", p.tok)
		}
		u := &UnaryArithm{
			Post:  true,
			OpPos: p.pos,
			Op:    UnAritOperator(p.tok),
			X:     x,
		}
		p.nextArith(compact)
		x = u
		for p.zshArithmFormat() {
			b := p.outputBaseArithm()
			b.Post, b.X = true, x
			x = b
		}
	}
	return x
}

// nextArith consumes a token.
// It returns true if compact and the token was followed by spaces
func (p *Parser) nextArith(compact bool) bool {
	p.next()
	if compact && p.spaced {
		return true
	}
	if !compact {
		p.got(_Newl)
	}
	return false
}

func (p *Parser) nextArithOp(compact bool) {
	pos := p.pos
	tok := p.tok
	if p.nextArith(compact) {
		p.followErrExp(pos, tok)
	}
}

// arithmExprBinary is used for all left-associative binary operators
func (p *Parser) arithmExprBinary(compact bool, nextOp func(bool) ArithmExpr, operators ...BinAritOperator) ArithmExpr {
	value := nextOp(compact)
	for {
		var foundOp BinAritOperator
		for _, op := range operators {
			if p.tok == token(op) {
				foundOp = op
				break
			}
		}

		if token(foundOp) == illegalTok || (compact && p.spaced) {
			return value
		}

		if value == nil {
			p.curErr("%#q must follow an expression", p.tok)
		}

		pos := p.pos
		p.nextArithOp(compact)
		y := nextOp(compact)
		if y == nil {
			p.followErrExp(pos, foundOp)
		}

		value = &BinaryArithm{
			OpPos: pos,
			Op:    foundOp,
			X:     value,
			Y:     y,
		}
	}
}

func isArithName(left ArithmExpr) bool {
	w, ok := left.(*Word)
	if !ok || len(w.Parts) != 1 {
		return false
	}
	switch wp := w.Parts[0].(type) {
	case *Lit:
		return ValidName(wp.Value)
	case *ParamExp:
		return wp.nakedIndex()
	default:
		return false
	}
}

func (p *Parser) isArithName(x ArithmExpr) bool {
	if p.zshExpandedArithm() {
		switch n := x.(type) {
		case *OutputBaseArithm:
			return p.isArithName(n.X)
		case *CharArithm:
			return !n.Literal && n.Value != nil && p.isArithName(n.Value)
		}
	}
	if isArithName(x) {
		return true
	}
	w, ok := x.(*Word)
	if !ok || !p.zshExpandedArithm() {
		return false
	}
	// Zsh expands the word before the arithmetic evaluator resolves its name.
	// Keep those expansions typed; their runtime value is not known here.
	for _, part := range w.Parts {
		switch part.(type) {
		case *ParamExp, *CmdSubst, *ArithmExp, *DblQuoted, *SglQuoted:
			return true
		}
	}
	return false
}

func (p *Parser) zshArithmFormat() bool {
	return p.zshExpandedArithm() && p.tok == leftBrack && p.r == '#'
}

func (p *Parser) outputBaseArithm() *OutputBaseArithm {
	b := &OutputBaseArithm{Lbrack: p.pos}
	pos := p.nextPos()
	for p.newLit(p.r); p.r != ']' && p.r != runeEOF; p.rune() {
		if p.r == '$' || p.r == '`' {
			p.curErr("shell expansions in arithmetic output formats are not supported")
			break
		}
	}
	b.Format = p.lit(pos, p.endLit())
	b.Rbrack = p.nextPos()
	if p.r != ']' {
		p.matchingErr(b.Lbrack, leftBrack, rightBrack)
	} else {
		p.rune()
		p.next()
	}
	return b
}

// A literal character may itself be an arithmetic operator. Read it before
// asking the arithmetic lexer for the following token. Control/meta prefixes
// belong to the character, as in ##^A and ##\M-\C-x.
func (p *Parser) zshArithmCharacter() *Word {
	pos := p.nextPos()
	for p.newLit(p.r); ; {
		switch p.r {
		case '(', ')':
			p.curErr("parenthesis must be escaped in a character literal")
			return p.wordOne(p.lit(pos, p.endLit()))
		case '^':
			p.rune()
		case '\\':
			p.rune()
			if (p.r == 'M' || p.r == 'C') && p.peek() == '-' {
				p.rune()
				p.rune()
				continue
			}
			n, hex := 0, true
			switch p.r {
			case 'x':
				n = 2
			case 'u':
				n = 4
			case 'U':
				n = 8
			case '0', '1', '2', '3', '4', '5', '6', '7':
				n, hex = 2, false
			}
			for ; n > 0; n-- {
				b := p.peek()
				if hex && (asciiDigit(b) || 'a' <= b && b <= 'f' || 'A' <= b && b <= 'F') || !hex && '0' <= b && b <= '7' {
					p.rune()
				} else {
					break
				}
			}
		}
		if p.r == runeEOF {
			p.curErr("character value must follow `##`")
			break
		}
		p.rune()
		break
	}
	return p.wordOne(p.lit(pos, p.endLit()))
}

func (p *Parser) zshExpandedArithm() bool {
	// Subscripts may be associative keys read by compatibility adapters;
	// unquoted let arguments also retain their existing grammar.
	return p.lang.in(LangZsh) && p.quote&(arithmExpr|arithmExprCmd) != 0
}

func (p *Parser) followArithm(ftok token, fpos Pos) ArithmExpr {
	x := p.arithmExpr(false)
	if x == nil {
		p.followErrExp(fpos, ftok)
	}
	return x
}

func (p *Parser) peekArithmEnd() bool {
	return p.tok == rightParen && p.r == ')'
}

func (p *Parser) arithmMatchingErr(pos Pos, left, right token) {
	switch p.tok {
	case _Lit, _LitWord:
		p.curErr("not a valid arithmetic operator: %#q", p.val)
	case leftBrack:
		p.curErr("%#q must follow a name like a[i]", leftBrack)
	case colon:
		p.curErr("ternary operator missing %#q before %#q", quest, colon)
	case rightParen, _EOF:
		p.matchingErr(pos, left, right)
	case period:
		p.checkLang(p.pos, LangZsh, `floating point arithmetic`)
	default:
		if p.quote&allArithmExpr != 0 {
			p.curErr("not a valid arithmetic operator: %#q", p.tok)
		}
		p.matchingErr(pos, left, right)
	}
}

func (p *Parser) matchedArithm(lpos Pos, left, right token) {
	if !p.got(right) {
		p.arithmMatchingErr(lpos, left, right)
	}
}

func (p *Parser) arithmEnd(ltok token, lpos Pos, old saveState) Pos {
	if !p.peekArithmEnd() {
		if p.recoverError() {
			return recoveredPos
		}
		p.arithmMatchingErr(lpos, ltok, dblRightParen)
	}
	p.rune()
	p.postNested(old)
	pos := p.pos
	p.next()
	return pos
}
