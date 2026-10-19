// Copyright (c) 2016, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package syntax

import (
	"bytes"
	"io"
	"unicode/utf8"
)

func asciiLetter[T rune | byte](r T) bool {
	return ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
}

func asciiDigit[T rune | byte](r T) bool {
	return r >= '0' && r <= '9'
}

// bytes that form or start a token
func regOps(r rune) bool {
	switch r {
	case ';', '"', '\'', '(', ')', '$', '|', '&', '>', '<', '`':
		return true
	}
	return false
}

// tokenize these inside parameter expansions
func paramOps(r rune) bool {
	switch r {
	case '}', '#', '!', ':', '-', '+', '=', '?', '%', '[', ']', '/', '^',
		',', '@', '*':
		return true
	}
	return false
}

// tokenize these inside arithmetic expansions
func arithmOps(r rune) bool {
	switch r {
	case '+', '-', '!', '~', '*', '/', '%', '(', ')', '^', '<', '>', ':', '=',
		',', '?', '|', '&', '[', ']', '#', '.':
		return true
	}
	return false
}

func bquoteEscaped(b byte) bool {
	switch b {
	case '$', '`', '\\':
		return true
	}
	return false
}

// Sentinel rune values used by [Parser.rune]. They are above [utf8.MaxRune]
// so that they can never collide with runes decoded from the input.
const (
	runeEOF rune = utf8.MaxRune + 1 // the input ran out, or we stopped early
	escNewl rune = utf8.MaxRune + 2 // an escaped newline, "\\\n"
)

func (p *Parser) rune() rune {
	if p.r == '\n' || p.r == escNewl {
		// p.r instead of b so that newline
		// character positions don't have col 0.
		p.line++
		p.col = 0
	}
	p.col += int64(p.w)
	bquotes := 0
retry:
	if p.bsp >= uint(len(p.bs)) && p.fill() == 0 {
		if len(p.bs) == 0 {
			// Necessary for the last position to be correct.
			// TODO: this is not exactly intuitive; figure out a better way.
			p.bsp = 1
		}
		p.r = runeEOF
		p.w = 1
		return p.r
	}
	if b := p.bs[p.bsp]; b < utf8.RuneSelf {
		p.bsp++
		if p.zshRecOn > 0 {
			// Every byte consumed, dropped ones included, so that the
			// record lines up with source offsets (#531).
			p.zshRec = append(p.zshRec, b)
		}
		switch b {
		case '\x00':
			// Ignore null bytes while parsing, like bash.
			p.col++
			goto retry
		case '\r':
			if p.peek() == '\n' { // \r\n turns into \n
				p.col++
				goto retry
			}
		case '\\':
			if p.r == '\\' {
			} else if p.peek() == '\n' {
				p.bsp++
				if p.zshRecOn > 0 {
					p.zshRec = append(p.zshRec, '\n')
				}
				p.w, p.r = 1, escNewl
				return escNewl
			} else if p1, p2 := p.peekTwo(); p1 == '\r' && p2 == '\n' { // \\\r\n turns into \\\n
				p.col++
				p.bsp += 2
				if p.zshRecOn > 0 {
					p.zshRec = append(p.zshRec, '\r', '\n')
				}
				p.w, p.r = 2, escNewl
				return escNewl
			}
			// TODO: why is this necessary to ensure correct position info?
			p.readEOF = false
			if p.openBquotes > 0 && p.bsp < uint(len(p.bs)) &&
				((bquotes < p.openBquotes && bquoteEscaped(p.bs[p.bsp])) ||
					// Backquotes within double quotes also escape double quotes.
					(bquotes < p.openBquoteDbls && p.bs[p.bsp] == '"')) {
				// We turn backquote command substitutions into $(),
				// so we remove the extra backslashes needed by the backquotes.
				bquotes++
				p.col++
				goto retry
			}
		}
		if b == '`' {
			p.lastBquoteEsc = bquotes
		}
		if p.litBs != nil {
			p.litBs = append(p.litBs, b)
		}
		p.w, p.r = 1, rune(b)
		return p.r
	}
decodeRune:
	var w int
	p.r, w = utf8.DecodeRune(p.bs[p.bsp:])
	if p.r == utf8.RuneError && !utf8.FullRune(p.bs[p.bsp:]) {
		// we need more bytes to read a full non-ascii rune
		if p.fill() > 0 {
			goto decodeRune
		}
	}
	if p.litBs != nil {
		p.litBs = append(p.litBs, p.bs[p.bsp:p.bsp+uint(w)]...)
	}
	if p.zshRecOn > 0 {
		p.zshRec = append(p.zshRec, p.bs[p.bsp:p.bsp+uint(w)]...)
	}
	p.bsp += uint(w)
	if p.r == utf8.RuneError && w == 1 {
		p.posErr(p.nextPos(), "invalid UTF-8 encoding")
	}
	p.w = w
	return p.r
}

// fill reads more bytes from the input src into readBuf.
// Any bytes that had not yet been used at the end of the buffer
// are slid into the beginning of the buffer.
// The number of read bytes is returned, which is at least one
// unless a read error occurred, such as [io.EOF].
func (p *Parser) fill() (n int) {
	if p.readEOF || p.r == runeEOF {
		// If the reader already gave us [io.EOF], do not try again.
		// If we decided to stop for any reason, do not bother reading either.
		return 0
	}
	p.offs += int64(p.bsp)
	left := len(p.bs) - int(p.bsp)
	copy(p.readBuf[:left], p.readBuf[p.bsp:])
readAgain:
	n, err := 0, p.readErr
	if err == nil {
		n, err = p.src.Read(p.readBuf[left:])
		p.readErr = err
		if err == io.EOF {
			p.readEOF = true
		}
	}
	if n == 0 {
		if err == nil {
			goto readAgain
		}
		// don't use p.errPass as we don't want to overwrite p.tok
		if err != io.EOF {
			p.err = err
		}
		if left > 0 {
			p.bs = p.readBuf[:left]
		} else {
			p.bs = nil
		}
	} else {
		p.bs = p.readBuf[:left+n]
	}
	p.bsp = 0
	return n
}

func (p *Parser) nextKeepSpaces() {
	r := p.r
	if p.quote != hdocBody && p.quote != hdocBodyTabs {
		// Heredocs handle escaped newlines in a special way, but others do not.
		for r == escNewl {
			r = p.rune()
		}
	}
	p.pos = p.nextPos()
	switch p.quote {
	case runeByRune:
		p.tok = illegalTok
	case dblQuotes:
		switch r {
		case '`', '"', '$':
			p.tok = p.dqToken(r)
		default:
			p.advanceLitDquote(r)
		}
	case hdocBody, hdocBodyTabs:
		switch r {
		case '`', '$':
			p.tok = p.dqToken(r)
		default:
			p.advanceLitHdoc(r)
		}
	case paramExpRepl:
		if r == '/' {
			p.rune()
			p.tok = slash
			break
		}
		fallthrough
	case paramExpExp:
		switch r {
		case '}':
			if p.zshParamWordBrace() {
				// A `}` that closes a `{` of the word is text (#518).
				p.advanceLitOther(r)
				break
			}
			p.tok = p.paramToken(r)
		case '\'':
			if p.zshDquoteParam {
				p.advanceLitOther(r)
				break
			}
			p.tok = p.regToken(r)
		case '`', '"', '$':
			p.tok = p.regToken(r)
		default:
			p.advanceLitOther(r)
		}
	}
	if p.err != nil {
		p.tok = _EOF
	}
}

func (p *Parser) next() {
	if p.r == runeEOF {
		p.tok = _EOF
		return
	}
	p.spaced = false
	if p.quote&allKeepSpaces != 0 {
		p.nextKeepSpaces()
		return
	}
	r := p.r
	for r == escNewl {
		r = p.rune()
	}
skipSpace:
	for {
		switch r {
		case runeEOF:
			p.tok = _EOF
			return
		case escNewl:
			r = p.rune()
		case ' ', '\t', '\r':
			if r != '\r' && p.zshCaseBlank() {
				// Pattern text in a Zsh case item's opened group,
				// as in `(a b)` and `(a[$x b])` (#483, #578).
				break skipSpace
			}
			p.spaced = true
			r = p.rune()
		case '\n':
			if p.zshCaseBlank() {
				// Zsh keeps a newline there too (#578).
				break skipSpace
			}
			if p.tok == _Newl {
				// merge consecutive newline tokens
				r = p.rune()
				continue
			}
			p.spaced = true
			p.tok = _Newl
			if p.quote != hdocWord && len(p.heredocs) > p.buriedHdocs {
				p.doHeredocs()
			}
			return
		default:
			break skipSpace
		}
	}
	if p.stopAt != nil && (p.spaced || p.tok == illegalTok || p.stopToken()) {
		// Note that the buffer may have been refilled since we read r,
		// such as when peeking at the byte which follows a backslash,
		// in which case r's bytes are gone and we cannot match on them.
		w := uint(utf8.RuneLen(r))
		if p.bsp >= w && bytes.HasPrefix(p.bs[p.bsp-w:], p.stopAt) {
			p.r = runeEOF
			p.w = 1
			p.tok = _EOF
			return
		}
	}
	p.pos = p.nextPos()
	if p.spaced || p.tok == _Newl || p.tok == semicolon {
		p.zshWordBraces, p.zshAssignValue = 0, false
	}
	switch p.tok {
	case _Newl, semicolon, and, or, andAnd, orOr, orAnd, andPipe, andBang,
		dblSemicolon, semiAnd, dblSemiAnd, semiOr:
		p.zshCmdPos = true
		p.zshWordBraces, p.zshAssignValue = 0, false
	}
	switch {
	case p.quote&allRegTokens != 0:
		switch r {
		case ';', '"', '\'', '(', ')', '$', '|', '&', '>', '<', '`':
			if r == '<' && p.lang.in(LangZsh) && p.zshNumRange() {
				p.advanceLitNone(r)
				return
			}
			p.tok = p.regToken(r)
		case '#':
			// If we're parsing $foo#bar, ${foo}#bar, 'foo'#bar, or "foo"#bar,
			// #bar is a continuation of the same word, not a comment.
			// The same applies inside [[ ]] tests, where '#' has no comment meaning.
			if !p.spaced && (p.quote == unquotedWordCont || p.quote == testExpr || p.zshWordCont) {
				p.advanceLitNone(r)
				return
			}
			// In Zsh, a `#` glued to the `(` that opens a case item, to a
			// `|` inside that group, or to the `)` of a leading group is
			// pattern text, as in `(#i)x)` and `(x)#)` (#482). A blank
			// before a `#` in that group is pattern text too, so the `#`
			// is read inside the blank's literal, as in `( #i)x)` (#578).
			if p.zshCaseHash && !p.spaced && (p.tok == leftParen || p.tok == or || p.tok == rightParen) {
				p.advanceLitNone(r)
				return
			}
			r = p.rune()
			p.newLit(r)
		runeLoop:
			for {
				switch r {
				case '\n', runeEOF:
					break runeLoop
				case escNewl:
					p.litBs = append(p.litBs, '\\', '\n')
					break runeLoop
				case '`':
					if p.backquoteEnd() {
						break runeLoop
					}
				}
				r = p.rune()
			}
			if p.keepComments {
				*p.curComs = append(*p.curComs, Comment{
					Hash: p.pos,
					Text: p.endLit(),
				})
			} else {
				p.litBs = nil
			}
			p.next()
		case '[':
			// `[` only starts an `[idx]=val` element when it begins a new word;
			// otherwise it continues a glob like `foo[0-9]`. In Zsh a `;`
			// separates elements too (#684).
			if p.quote == arrayElems && (p.spaced || p.tok == leftParen || p.tok == _Newl ||
				p.tok == semicolon && p.lang.in(LangZsh)) {
				p.rune()
				p.tok = leftBrack
			} else {
				p.advanceLitNone(r)
			}
		case '=':
			if p.peek() == '(' {
				p.rune()
				p.rune()
				p.tok = assgnParen
			} else if p.quote == arrayElems && (!p.lang.in(LangZsh) || p.tok == rightBrack) {
				// In Zsh only a `=` after an `[idx]` assigns; any other `=`
				// is a literal element, as in `a=(-S =)` (#684).
				p.rune()
				p.tok = assgn
			} else {
				p.advanceLitNone(r)
			}
		case '?', '*', '+', '@', '!':
			if p.extendedGlob() {
				switch r {
				case '?':
					p.tok = globQuest
				case '*':
					p.tok = globStar
				case '+':
					p.tok = globPlus
				case '@':
					p.tok = globAt
				case '!':
					p.tok = globExcl
				}
				p.rune()
				p.rune()
			} else {
				p.advanceLitNone(r)
			}
		default:
			p.advanceLitNone(r)
		}
	case p.quote == paramExpArithm && r == '#':
		// '#' is mksh's unsigned prefix, which only [ArithmExp] and [ArithmCmd]
		// can represent, so in a subscript it starts a literal instead;
		// it can be part of an associative array key, like in ${args[cmd,#]}.
		p.advanceLitOther(r)
	case p.zshSubscriptBrace() && r != ']' && r != '$' && r != '\'' && r != '"' && r != '`':
		// A `{` of a Zsh subscript is open, as in `${h[{a b}]}`: blanks
		// and operators are text up to its `}` (#521).
		p.advanceLitOther(r)
	case p.quote&allArithmExpr != 0 && arithmOps(r):
		p.tok = p.arithmToken(r)
	case p.quote&allParamExp != 0 && paramOps(r):
		p.tok = p.paramToken(r)
	case p.quote == testExprRegexp:
		if !p.rxFirstPart && p.spaced {
			p.quote = testExpr
			goto skipSpace
		}
		p.rxFirstPart = false
		switch r {
		case ';', '"', '\'', '$', '&', '>', '<', '`':
			p.tok = p.regToken(r)
		case ')':
			if p.rxOpenParens > 0 {
				// continuation of open paren
				p.advanceLitRe(r)
			} else {
				p.tok = rightParen
				p.quote = testExpr
				p.rune() // we are tokenizing manually
			}
		default: // including '(', '|'
			p.advanceLitRe(r)
		}
	case regOps(r):
		p.tok = p.regToken(r)
	default:
		p.advanceLitOther(r)
	}
	if p.err != nil {
		p.tok = _EOF
	}
}

// extendedGlob determines whether we're parsing a Bash extended globbing expression.
// For example, whether `*` or `@` are followed by `(` to form `@(foo)`.
func (p *Parser) extendedGlob() bool {
	if p.lang.in(LangZsh) {
		// Zsh supports Bash extended globs via the KSH_GLOB option.
		// In Bash we would parse extended globs as [ExtGlob] nodes,
		// but trying to do that in Zsh would cause ambiguity with glob qualifiers.
		// Just like glob qualifiers, parse extended globs as literals in Zsh.
		return false
	}
	if p.val == "function" {
		// We don't support e.g. `function @() { ... }` at the moment, but we could.
		return false
	}
	if p.peek() == '(' {
		// NOTE: empty pattern list is a valid globbing syntax like `@()`,
		// but we'll operate on the "likelihood" that it is a function;
		// only tokenize if its a non-empty pattern list.
		// We do this after peeking for just one byte, so that the input `echo *`
		// followed by a newline does not hang an interactive shell parser until
		// another byte is input.
		_, p2 := p.peekTwo()
		return p2 != ')'
	}
	return false
}

func (p *Parser) peek() byte {
	if int(p.bsp) >= len(p.bs) {
		p.fill()
	}
	if int(p.bsp) >= len(p.bs) {
		return utf8.RuneSelf
	}
	return p.bs[p.bsp]
}

func (p *Parser) peekTwo() (byte, byte) {
	// TODO: This should loop for slow readers, e.g. those providing one byte at
	// a time. Use a loop and test it with [testing/iotest.OneByteReader].
	if int(p.bsp+1) >= len(p.bs) {
		p.fill()
	}
	if int(p.bsp) >= len(p.bs) {
		return utf8.RuneSelf, utf8.RuneSelf
	}
	if int(p.bsp+1) >= len(p.bs) {
		return p.bs[p.bsp], utf8.RuneSelf
	}
	return p.bs[p.bsp], p.bs[p.bsp+1]
}

func (p *Parser) regToken(r rune) token {
	switch r {
	case '\'':
		p.rune()
		return sglQuote
	case '"':
		p.rune()
		return dblQuote
	case '`':
		// Don't call p.rune, as we need to work out p.openBquotes to
		// properly handle backslashes in the lexer.
		return bckQuote
	case '&':
		switch p.rune() {
		case '&':
			p.rune()
			return andAnd
		case '>':
			switch p.rune() {
			case '|':
				p.rune()
				return rdrAllClob
			case '!':
				if p.lang.in(LangZsh) {
					p.rune()
					return rdrAllClob
				}
			case '>':
				switch p.rune() {
				case '|':
					p.rune()
					return appAllClob
				case '!':
					if p.lang.in(LangZsh) {
						p.rune()
						return appAllClob
					}
				}
				return appAll
			}
			return rdrAll
		case '|':
			if p.lang.in(LangZsh) {
				p.rune()
				return andPipe
			}
		case '!':
			if p.lang.in(LangZsh) {
				p.rune()
				return andBang
			}
		}
		return and
	case '|':
		switch p.rune() {
		case '|':
			p.rune()
			return orOr
		case '&':
			if !p.lang.in(langBashLike | LangMirBSDKorn | LangZsh) {
				break
			}
			p.rune()
			return orAnd
		}
		return or
	case '$':
		switch p.rune() {
		case '\'':
			if !p.lang.in(langBashLike | LangMirBSDKorn | LangZsh) {
				break
			}
			p.rune()
			return dollSglQuote
		case '"':
			if !p.lang.in(langBashLike | LangMirBSDKorn) {
				break
			}
			p.rune()
			return dollDblQuote
		case '{':
			p.rune()
			return dollBrace
		case '[':
			if !p.lang.in(langBashLike) {
				// latter to not tokenise ${$[@]} as $[
				break
			}
			p.rune()
			return dollBrack
		case '(':
			if p.rune() == '(' {
				if p.lang.in(LangZsh) {
					if end, math := p.zshMathSubstAhead(); !math {
						return dollParen
					} else if p.zshMathScan && end.r == ')' && p.zshRecOn == 0 {
						p.zshMathEnd = end
					}
				}
				p.rune()
				return dollDblParen
			}
			return dollParen
		}
		return dollar
	case '(':
		if p.rune() == '(' && p.lang.in(langBashLike|LangMirBSDKorn|LangZsh) && p.quote != testExpr {
			p.rune()
			return dblLeftParen
		}
		return leftParen
	case ')':
		p.rune()
		return rightParen
	case ';':
		switch p.rune() {
		case ';':
			if p.rune() == '&' && p.lang.in(langBashLike) {
				p.rune()
				return dblSemiAnd
			}
			return dblSemicolon
		case '&':
			if !p.lang.in(langBashLike | LangMirBSDKorn | LangZsh) {
				break
			}
			p.rune()
			return semiAnd
		case '|':
			if !p.lang.in(LangMirBSDKorn | LangZsh) {
				break
			}
			p.rune()
			return semiOr
		}
		return semicolon
	case '<':
		switch p.rune() {
		case '<':
			switch p.rune() {
			case '-':
				p.rune()
				return dashHdoc
			case '<':
				p.rune()
				return wordHdoc
			}
			return hdoc
		case '>':
			p.rune()
			return rdrInOut
		case '&':
			p.rune()
			return dplIn
		case '(':
			if !p.lang.in(langBashLike | LangZsh) {
				break
			}
			p.rune()
			return cmdIn
		}
		return rdrIn
	case '>':
		switch p.rune() {
		case '>':
			switch p.rune() {
			case '|':
				p.rune()
				return appClob
			case '!':
				if p.lang.in(LangZsh) {
					p.rune()
					return appClob
				}
			case '&':
				if !p.lang.in(LangZsh) {
					break
				}
				switch p.rune() {
				case '|':
					p.rune()
					return appAllClob // >>&| is an alias for &>>|
				case '!':
					p.rune()
					return appAllClob // >>&! is an alias for &>>|
				}
				return appAll // >>& is an alias for &>>
			}
			return appOut
		case '&':
			r = p.rune()
			if p.lang.in(LangZsh) {
				switch r {
				case '|':
					p.rune()
					return rdrAllClob // >&| is an alias for &>|
				case '!':
					p.rune()
					return rdrAllClob // >&! is an alias for &>|
				}
			}
			return dplOut
		case '|':
			p.rune()
			return rdrClob
		case '!':
			if p.lang.in(LangZsh) {
				p.rune()
				return rdrClob
			}
		case '(':
			if !p.lang.in(langBashLike | LangZsh) {
				break
			}
			p.rune()
			return cmdOut
		}
		return rdrOut
	}
	panic("unreachable")
}

func (p *Parser) dqToken(r rune) token {
	switch r {
	case '"':
		p.rune()
		return dblQuote
	case '`':
		// Don't call p.rune, as we need to work out p.openBquotes to
		// properly handle backslashes in the lexer.
		return bckQuote
	case '$':
		switch p.rune() {
		case '{':
			p.rune()
			return dollBrace
		case '[':
			if !p.lang.in(langBashLike) {
				break
			}
			p.rune()
			return dollBrack
		case '(':
			if p.rune() == '(' {
				if p.lang.in(LangZsh) {
					if end, math := p.zshMathSubstAhead(); !math {
						return dollParen
					} else if p.zshMathScan && end.r == ')' && p.zshRecOn == 0 {
						p.zshMathEnd = end
					}
				}
				p.rune()
				return dollDblParen
			}
			return dollParen
		}
		return dollar
	}
	panic("unreachable")
}

// zshMathSubstAhead distinguishes `$((expr))` from `$( (list) ... )`
// without consuming the second `(` at p.r (#266). Zsh's cmd_or_math in
// Src/lex.c reads a math string, then requires adjacent closing `))`;
// expression validity does not decide the reading. Quotes are text in that
// math string, while nested substitutions own their parentheses.
// The returned lexer is at the first closing `)` when the reading is math,
// so a containing lookahead can adopt it rather than parse the expression
// again. This avoids exponential work for nested arithmetic expansions.
func (p *Parser) zshMathSubstAhead() (*Parser, bool) {
	key := zshMathKey{p.offs + int64(p.bsp) - int64(p.w), p.openBquotes, p.openBquoteDbls}
	if p.zshCmdParens == nil {
		p.zshCmdParens = map[zshMathKey]struct{}{}
	} else if _, ok := p.zshCmdParens[key]; ok {
		return nil, false
	}
	look := p.zshInnerParser()
	look.f = &File{}
	look.zshMathScan = true
	look.zshCmdParens = p.zshCmdParens
	look.r, look.w, look.bsp = p.r, p.w, p.bsp
	look.offs, look.line, look.col = p.offs, p.line, p.col
	look.readEOF, look.readErr = p.readEOF, p.readErr
	look.openBquotes, look.openBquoteDbls = p.openBquotes, p.openBquoteDbls
	look.quote = runeByRune
	look.bs = look.readBuf[:len(p.bs)]
	copy(look.bs, p.bs)
	var read bytes.Buffer
	look.src = io.TeeReader(p.src, &read)
	defer func() {
		p.zshFoldInner(look)
		if p.readErr == nil && look.readErr != nil {
			// Preserve a terminal read error too: some readers refuse a
			// second read after EOF, and non-EOF errors must not be lost.
			p.src = zshLookaheadError{look.readErr}
		}
		// A lookahead may cross any number of input buffers. Replay only
		// newly read bytes; p still owns its original buffered bytes.
		if read.Len() > 0 {
			p.src = io.MultiReader(bytes.NewReader(read.Bytes()), p.src)
		}
	}()
	math := look.zshMathSubstEnd()
	look.src = p.src // stop recording before the caller adopts the lexer
	if !math {
		p.zshCmdParens[key] = struct{}{}
	}
	return look, math
}

// zshMathKey names a `$((` by the offset of its second `(` and the
// backquote levels around it, which together fix how it is read.
type zshMathKey struct {
	offset                      int64
	openBquotes, openBquoteDbls int
}

func (look *Parser) zshMathSubstEnd() bool {
	parens, brackets := 0, 0
	tick := false
	look.rune()
	for look.r != runeEOF {
		switch look.r {
		case '\\':
			switch look.rune() {
			case '$', '\\', ')', '`':
			default:
				// As in dquote_parse with ')' as its end character,
				// other escaped bytes still affect math nesting.
				continue
			}
		case '$':
			if !tick && look.peek() == '(' {
				if !look.zshMathNestedSubst() {
					return true
				}
				continue // cmdSubst leaves p.r after the closer
			}
			var nest []byte
			if !tick && look.peek() == '{' {
				// Parameter words are read as in double quotes: `'` is
				// text, and their parentheses do not count as math.
				nest = []byte{'}'}
			}
			if nest != nil {
				look.rune()
				look.rune()
				for look.r != runeEOF && len(nest) > 0 {
					top := nest[len(nest)-1]
					if look.r == '$' && look.peek() == '(' && top != '\'' && top != '`' {
						if !look.zshMathNestedSubst() {
							return true
						}
						continue
					}
					nest = look.zshGroupRune(nest)
					if len(nest) == 0 {
						break
					}
					look.rune()
				}
			}
		case '`':
			tick = !tick
		case '(':
			parens++
		case ')':
			if parens == 0 && brackets == 0 && !tick {
				return look.peek() == ')'
			}
			parens--
			if parens < 0 {
				return false
			}
		case '[':
			brackets++
		case ']':
			brackets--
			if brackets < 0 {
				return false
			}
		}
		look.rune()
	}
	// Leave incomplete input to the arithmetic parser's existing error.
	return true
}

// zshMathNestedSubst skips the substitution at `$` using the grammar:
// case patterns and heredocs may contain unmatched closers as text.
func (look *Parser) zshMathNestedSubst() bool {
	pos := look.nextPos()
	look.rune()
	look.rune()
	if look.r == '(' {
		if sub, math := look.zshMathSubstAhead(); math {
			// sub's reads are already folded into look; keep them.
			read := look.zshAssignRead
			*look = *sub
			look.zshAssignRead = look.zshAssignRead || read
			look.rune() // second closing `)`
			look.rune()
			return true
		}
	}
	look.pos, look.tok = pos, dollParen
	look.cmdSubst()
	return look.err == nil // cmdSubst leaves p.r after the closer
}

type zshLookaheadError struct{ err error }

func (r zshLookaheadError) Read([]byte) (int, error) { return 0, r.err }

func (p *Parser) paramToken(r rune) token {
	switch r {
	case '}':
		p.rune()
		return rightBrace
	case ':':
		switch p.rune() {
		case '+':
			p.rune()
			return colPlus
		case '-':
			p.rune()
			return colMinus
		case '?':
			p.rune()
			return colQuest
		case '=':
			p.rune()
			return colAssgn
		case '#':
			p.rune()
			return colHash
		case '|':
			p.rune()
			return colPipe
		case '*':
			p.rune()
			return colStar
		case '^':
			// Zsh array zipping, ${a:^b} and ${a:^^b} (#671).
			if p.rune() == '^' {
				p.rune()
				return colDblCaret
			}
			return colCaret
		}
		return colon
	case '+':
		p.rune()
		return plus
	case '-':
		p.rune()
		return minus
	case '?':
		p.rune()
		return quest
	case '=':
		p.rune()
		return assgn
	case '%':
		if p.rune() == '%' {
			p.rune()
			return dblPerc
		}
		return perc
	case '#':
		if p.rune() == '#' {
			p.rune()
			return dblHash
		}
		return hash
	case '!':
		p.rune()
		return exclMark
	case ']':
		p.rune()
		return rightBrack
	case '/':
		if p.rune() == '/' {
			p.rune()
			return dblSlash
		}
		return slash
	case '^':
		if p.rune() == '^' {
			p.rune()
			return dblCaret
		}
		return caret
	case ',':
		if p.rune() == ',' {
			p.rune()
			return dblComma
		}
		return comma
	case '@':
		p.rune()
		return at
	case '*':
		p.rune()
		return star

	// This func gets called by the parser in [runeByRune] mode;
	// we need to handle EOF and unexpected runes.
	case runeEOF:
		return _EOF
	default:
		return illegalTok
	}
}

func (p *Parser) arithmToken(r rune) token {
	switch r {
	case '!':
		if p.rune() == '=' {
			p.rune()
			return nequal
		}
		return exclMark
	case '=':
		if p.rune() == '=' {
			p.rune()
			return equal
		}
		return assgn
	case '~':
		p.rune()
		return tilde
	case '(':
		p.rune()
		return leftParen
	case ')':
		p.rune()
		return rightParen
	case '&':
		switch p.rune() {
		case '&':
			if p.rune() == '=' && p.lang.in(LangZsh) {
				p.rune()
				return andBoolAssgn
			}
			return andAnd
		case '=':
			p.rune()
			return andAssgn
		}
		return and
	case '|':
		switch p.rune() {
		case '|':
			if p.rune() == '=' && p.lang.in(LangZsh) {
				p.rune()
				return orBoolAssgn
			}
			return orOr
		case '=':
			p.rune()
			return orAssgn
		}
		return or
	case '<':
		switch p.rune() {
		case '<':
			if p.rune() == '=' {
				p.rune()
				return shlAssgn
			}
			return hdoc
		case '=':
			p.rune()
			return lequal
		}
		return rdrIn
	case '>':
		switch p.rune() {
		case '>':
			if p.rune() == '=' {
				p.rune()
				return shrAssgn
			}
			return appOut
		case '=':
			p.rune()
			return gequal
		}
		return rdrOut
	case '+':
		switch p.rune() {
		case '+':
			p.rune()
			return addAdd
		case '=':
			p.rune()
			return addAssgn
		}
		return plus
	case '-':
		switch p.rune() {
		case '-':
			p.rune()
			return subSub
		case '=':
			p.rune()
			return subAssgn
		}
		return minus
	case '%':
		if p.rune() == '=' {
			p.rune()
			return remAssgn
		}
		return perc
	case '*':
		switch p.rune() {
		case '*':
			if p.rune() == '=' && p.lang.in(LangZsh) {
				p.rune()
				return powAssgn
			}
			return power
		case '=':
			p.rune()
			return mulAssgn
		}
		return star
	case '/':
		if p.rune() == '=' {
			p.rune()
			return quoAssgn
		}
		return slash
	case '^':
		switch p.rune() {
		case '^':
			if p.rune() == '=' && p.lang.in(LangZsh) {
				p.rune()
				return xorBoolAssgn
			}
			return dblCaret
		case '=':
			p.rune()
			return xorAssgn
		}
		return caret
	case '[':
		p.rune()
		return leftBrack
	case ']':
		p.rune()
		return rightBrack
	case ',':
		p.rune()
		return comma
	case '?':
		p.rune()
		return quest
	case ':':
		p.rune()
		return colon
	case '#':
		p.rune()
		return hash
	case '.':
		p.rune()
		return period
	}
	panic("unreachable")
}

func (p *Parser) newLit(r rune) {
	switch {
	case r < utf8.RuneSelf:
		p.litBs = p.litBuf[:1]
		p.litBs[0] = byte(r)
	case r == runeEOF || r == escNewl:
		// sentinel runes not present in the input as-is
		p.litBs = p.litBuf[:0]
	default:
		w := utf8.RuneLen(r)
		p.litBs = append(p.litBuf[:0], p.bs[p.bsp-uint(w):p.bsp]...)
	}
}

func (p *Parser) endLit() (s string) {
	if p.r == runeEOF || p.r == escNewl {
		s = string(p.litBs)
	} else {
		s = string(p.litBs[:len(p.litBs)-p.w])
	}
	p.litBs = nil
	return s
}

func (p *Parser) isLitRedir() bool {
	lit := p.litBs[:len(p.litBs)-1]
	if lit[0] == '{' && lit[len(lit)-1] == '}' {
		name := lit[1 : len(lit)-1]
		// Bash also allows an array element such as {name[idx]}.
		if p.lang.in(langBashLike) && len(name) > 0 && name[len(name)-1] == ']' {
			if i := bytes.IndexByte(name, '['); i > 0 && i < len(name)-2 {
				name = name[:i]
			}
		}
		return ValidName(string(name))
	}
	return numberLiteral(lit)
}

func positionalRuneParam[T rune | byte](r T) bool {
	switch r {
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}
	return false
}

func singleRuneParam[T rune | byte](r T) bool {
	switch r {
	case '@', '*', '#', '$', '?', '!', '-':
		return true
	}
	return positionalRuneParam(r)
}

func paramNameRune[T rune | byte](r T) bool {
	return asciiLetter(r) || asciiDigit(r) || r == '_'
}

// paramNameStartRune reports whether a parameter name can begin with r,
// be it a single-rune name like $@ or a regular name like $foo.
func paramNameStartRune[T rune | byte](r T) bool {
	return singleRuneParam(r) || paramNameRune(r)
}

func (p *Parser) advanceLitOther(r rune) {
	tok := _LitWord
loop:
	for p.newLit(r); r != runeEOF; r = p.rune() {
		switch r {
		case '\\': // escaped byte follows
			p.rune()
		case '\'', '"', '`', '$':
			if r == '\'' && p.zshDquoteParam && p.quote&(paramExpExp|paramExpRepl) != 0 {
				continue // Zsh: a quote in a double-quoted ${...} word is text
			}
			tok = _Lit
			break loop
		case '}':
			if p.zshParamWordBrace() {
				p.zshParamBraces--
				continue
			}
			if p.zshSubscriptBrace() {
				p.zshSubBraces--
				continue
			}
			if p.quote&allParamExp != 0 {
				break loop
			}
		case '{':
			if p.zshParamWord() {
				p.zshParamBraces++
			}
			if p.zshSubscriptWord() {
				if p.zshSubBraces == 0 {
					p.zshSubBraceOpen = p.nextPos()
				}
				p.zshSubBraces++
			}
		case '/':
			if p.quote != paramExpExp && !p.zshSubscriptBrace() {
				break loop
			}
		case ':', '=', '%', '^', ',', '?', '!', '~', '*':
			if p.quote&allArithmExpr != 0 && !p.zshSubscriptBrace() {
				break loop
			}
		case '.':
			if !p.lang.in(LangZsh) && p.quote&allArithmExpr != 0 {
				break loop
			}
		case '[', ']':
			if r == '[' && p.zshSubscriptBrace() {
				continue // only `]` ends the text of a Zsh subscript (#521)
			}
			if p.lang.in(langBashLike|LangMirBSDKorn|LangZsh) && p.quote&allArithmExpr != 0 {
				break loop
			}
			fallthrough
		case '+', '-', ' ', '\t', ';', '&', '>', '<', '|', '(', ')', '\n', '\r':
			if p.quote&allKeepSpaces == 0 && !p.zshSubscriptBrace() {
				break loop
			}
		}
	}
	p.tok, p.val = tok, p.endLit()
}

// zshParamWord reports whether a byte is being lexed as part of the word of
// an unquoted Zsh parameter expansion, where braces nest (#518). Inside
// double quotes the first `}` closes the expansion, so they do not.
func (p *Parser) zshParamWord() bool {
	return p.lang.in(LangZsh) && !p.zshDquoteParam && p.quote&(paramExpExp|paramExpRepl) != 0
}

// zshParamWordBrace reports whether a `}` at this point closes a `{` of the
// word of an unquoted Zsh parameter expansion rather than the expansion.
func (p *Parser) zshParamWordBrace() bool {
	return p.zshParamBraces > 0 && p.zshParamWord()
}

// zshSubscriptWord reports whether a byte is being lexed in the subscript of
// an unquoted Zsh `${...}`, where braces are counted (#521).
func (p *Parser) zshSubscriptWord() bool {
	return p.zshSubscript && p.quote == paramExpArithm
}

// zshSubscriptBrace reports whether a `{` of such a subscript is open.
func (p *Parser) zshSubscriptBrace() bool {
	return p.zshSubBraces > 0 && p.zshSubscriptWord()
}

// zshNumRange peeks at the bytes after '<' to check for a zsh numeric
// range glob pattern like <->, <5->, <-10>, or <5-10>.
func (p *Parser) zshNumRange() bool {
	// Peeking a handful of bytes here should be enough.
	// TODO: This should loop for slow readers, e.g. those providing one byte at
	// a time. Use a loop and test it with [testing/iotest.OneByteReader].
	if int(p.bsp) >= len(p.bs) {
		p.fill()
	}
	rest := p.bs[p.bsp:]
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		rest = rest[1:]
	}
	if len(rest) == 0 || rest[0] != '-' {
		return false
	}
	rest = rest[1:]
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		rest = rest[1:]
	}
	return len(rest) > 0 && rest[0] == '>'
}

func (p *Parser) advanceLitNone(r rune) {
	p.eqlOffs = -1
	tok := _LitWord
	wordStart := p.spaced || p.zshCmdPos || p.stopToken() && p.tok != rightParen
loop:
	for p.newLit(r); r != runeEOF; r = p.rune() {
		switch r {
		case '{':
			if p.lang.in(LangZsh) && p.zshCommandWord() {
				if len(p.litBs) == 1 && p.zshWordBraces == 0 && !p.zshAssignValue && p.zshCmdPos && !p.zshWordsOnly && (p.quote != unquotedWordCont || p.spaced) {
					// Zsh's gettokstr returns a command-position `{`
					// immediately, before reading the following command.
					p.rune()
					break loop
				}
				p.zshWordBraces++
			}
		case ' ', '\t', '\n':
			if p.zshCaseOpen {
				// Pattern text in a Zsh case item's opened group
				// (#483, #578); see zshCaseBlank.
				continue
			}
			break loop
		case escNewl:
			// A line continuation next to a blank of an opened case
			// group ends the literal, which the next token continues,
			// so the blanks caseItems strips never span the dropped
			// bytes and its column arithmetic stays exact (#578).
			if p.zshCaseOpen && len(p.litBs) > 0 && (zshCaseBlankByte(p.litBs[len(p.litBs)-1]) || zshCaseBlankByte(p.peek())) {
				tok = _Lit
				break loop
			}
		case '\r', '&', '|', ';', ')':
			break loop
		case '(':
			break loop
		case '\\': // escaped byte follows
			p.rune()
		case '>', '<':
			if r == '<' && p.lang.in(LangZsh) && p.zshNumRange() {
				// Zsh numeric range glob like <-> or <1-100>; consume until '>'.
				for {
					if r = p.rune(); r == '>' || r == runeEOF {
						break
					}
				}
				continue
			}
			if p.peek() == '(' {
				tok = _Lit
			} else if p.isLitRedir() {
				tok = _LitRedir
			}
			break loop
		case '`':
			if p.quote != subCmdBckquo {
				tok = _Lit
			}
			break loop
		case '"', '\'', '$':
			tok = _Lit
			break loop
		case '?', '*', '+', '@', '!':
			if p.extendedGlob() {
				tok = _Lit
				break loop
			}
		case '=':
			if p.eqlOffs < 0 {
				p.eqlOffs = len(p.litBs) - 1
			}
		case '}':
			if p.lang.in(LangZsh) && p.zshCommandWord() {
				if p.zshWordBraces > 0 {
					p.zshWordBraces--
				} else if !p.zshAssignValue && p.zshBraceWordEnd() {
					if len(p.litBs) == 1 {
						if !p.spaced && (!p.stopToken() || p.tok == rightParen) {
							tok = rightBrace
						}
						p.rune()
					}
					break loop
				}
			}
			if p.quote == subCmdBraces && len(p.litBs) == 1 {
				// A word-initial `}` closes the substitution even if
				// more characters follow, as in `${ foo;}bar`.
				p.rune()
				break loop
			}
		case '[':
			if p.litBs[0] == '{' && p.lang.in(langBashLike) {
				// In bash, words beginning with '{' are always kept whole,
				// so that {name[idx]} literals can prefix a redirect
				// operator below.
				break
			}
			if p.lang.in(langBashLike|LangMirBSDKorn|LangZsh) && len(p.litBs) > 1 && p.litBs[0] != '[' {
				tok = _Lit
				break loop
			}
		}
	}
	p.tok, p.val = tok, p.endLit()
	if p.lang.in(LangZsh) && p.zshCommandWord() && wordStart && p.val == "}" && p.r == '}' {
		// Splitting `}}` leaves the reserved word `}`, whereas `a}}`
		// leaves the literal word `a}` followed by its closer.
		p.tok = rightBrace
	}
}

func (p *Parser) zshCommandWord() bool {
	return p.quote&(noState|unquotedWordCont|subCmd|subCmdBckquo) != 0 && !p.zshCaseOpen && !p.zshBraceIndex
}

func (p *Parser) zshBraceWordEnd() bool {
	b := p.peek()
	if int(p.bsp) >= len(p.bs) {
		return true
	}
	switch b {
	case ' ', '\t', '\r', '\n', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

// zshCaseBlank reports whether a blank at the start of the next token is
// pattern text: in a Zsh case item's opened group, which Zsh lexes as one
// word (Src/lex.c gettokstr breaks at a blank only outside parentheses), a
// space, tab or newline continues the word, as in `(a b)`, `(a $x)` and
// `(a[$x b])` (#483, #578). caseItems later strips the blanks par_case
// strips. advanceLitNone keeps every blank while zshCaseOpen is set, so a
// blank this reports is always consumed and the lexer cannot stall on it.
func (p *Parser) zshCaseBlank() bool {
	return p.zshCaseOpen && p.quote&allRegTokens != 0
}

// zshCaseBlankByte reports whether b is a blank par_case may strip.
func zshCaseBlankByte(b byte) bool {
	return b == ' ' || b == '\t'
}

func (p *Parser) advanceLitDquote(r rune) {
	tok := _LitWord
loop:
	for p.newLit(r); r != runeEOF; r = p.rune() {
		switch r {
		case '"':
			break loop
		case '\\': // escaped byte follows
			p.rune()
		case escNewl, '`', '$':
			tok = _Lit
			break loop
		}
	}
	p.tok, p.val = tok, p.endLit()
}

func (p *Parser) advanceLitHdoc(r rune) {
	// Unlike the rest of nextKeepSpaces quote states, we handle escaped
	// newlines here. If lastTok==_Lit, then we know we're following an
	// escaped newline, so the first line can't end the heredoc.
	lastTok := p.tok
	for r == escNewl {
		r = p.rune()
		lastTok = _Lit
	}
	p.pos = p.nextPos()

	p.tok = _Lit
	p.newLit(r)
	for p.quote == hdocBodyTabs && r == '\t' {
		r = p.rune()
	}
	lStart := len(p.litBs) - p.w
	atLineStart := true
	var lPos Pos
	stop := p.hdocStops[len(p.hdocStops)-1]
	for ; ; r = p.rune() {
		if atLineStart {
			lPos = p.nextPos() // the first rune in the line
			atLineStart = false
		}
		switch r {
		case escNewl, '$':
			p.val = p.endLit()
			return
		case '\\': // escaped byte follows
			p.rune()
		case '`':
			if !p.backquoteEnd() {
				p.val = p.endLit()
				return
			}
			fallthrough
		case '\n', runeEOF:
			if lStart == 0 && lastTok == _Lit {
				// This line starts right after an escaped
				// newline, so it should never end the heredoc.
			} else if lStart >= 0 {
				// Compare the current line with the stop word.
				line := p.litBs[lStart:]
				if r != runeEOF && len(line) > 0 {
					line = line[:len(line)-1] // minus trailing character
				}
				if bytes.Equal(line, stop) {
					p.tok = _LitWord
					p.val = p.endLit()[:lStart]
					if p.val == "" {
						p.tok = _Newl
					}
					if stop != nil {
						// Not the newline lexed right after the closing word,
						// which matches as an empty line with a nil stop word.
						p.hdocClosePos = lPos
					}
					p.hdocStops[len(p.hdocStops)-1] = nil
					return
				}
			}
			if r != '\n' {
				// Like other shells, end an unclosed heredoc
				// at EOF or at a closing backquote.
				if p.val = p.endLit(); p.val == "" {
					p.next()
				}
				return
			}
			for p.quote == hdocBodyTabs && p.peek() == '\t' {
				p.rune()
			}
			lStart = len(p.litBs)
			atLineStart = true
		}
	}
}

func (p *Parser) quotedHdocWord() *Word {
	r := p.r
	p.newLit(r)
	pos := p.nextPos()
	stop := p.hdocStops[len(p.hdocStops)-1]
	for ; r != runeEOF; r = p.rune() {
		for p.quote == hdocBodyTabs && r == '\t' {
			r = p.rune()
		}
		lStart := len(p.litBs) - p.w
		lPos := p.nextPos()
	runeLoop:
		for {
			switch r {
			case runeEOF, '\n':
				break runeLoop
			case '`':
				if p.backquoteEnd() {
					break runeLoop
				}
			case escNewl:
				p.litBs = append(p.litBs, '\\', '\n')
				break runeLoop
			}
			r = p.rune()
		}
		if lStart < 0 {
			continue
		}
		// Compare the current line with the stop word.
		line := p.litBs[lStart:]
		if r != runeEOF && len(line) > 0 {
			line = line[:len(line)-1] // minus \n
		}
		if bytes.Equal(line, stop) {
			p.hdocClosePos = lPos
			p.hdocStops[len(p.hdocStops)-1] = nil
			val := p.endLit()[:lStart]
			if val == "" {
				return nil
			}
			return p.wordOne(p.lit(pos, val))
		}
		if r == '`' {
			break // closing backquote
		}
	}
	// Like other shells, end an unclosed heredoc
	// at EOF or at a closing backquote.
	if val := p.endLit(); val != "" {
		return p.wordOne(p.lit(pos, val))
	}
	return nil
}

func (p *Parser) advanceLitRe(r rune) {
	for p.newLit(r); ; r = p.rune() {
		switch r {
		case '\\':
			p.rune()
		case '(':
			if p.lang.in(LangZsh) && p.peek() == ')' {
				// `()` ends the word and is a token of its own, at any
				// depth (#522); the operator check then reports its `(`.
				p.tok, p.val = _LitWord, p.endLit()
				p.quote = testExpr
				return
			}
			if p.rxOpenParens == 0 {
				p.rxGroupStart = p.nextPos()
			}
			p.rxOpenParens++
		case ')':
			if p.rxOpenParens--; p.rxOpenParens < 0 {
				p.tok, p.val = _LitWord, p.endLit()
				p.quote = testExpr
				return
			}
		case ';', '&', '>', '<':
			if p.rxOpenParens > 0 && p.lang.in(LangZsh) {
				// Zsh lexes the operand as an ordinary word (gettokstr in
				// Src/lex.c), so these end it even inside a group, unless
				// `<(`/`>(` starts a process substitution or `<m-n>` is a
				// numeric glob (#517, as #511 for other words).
				if (r == '<' || r == '>') && p.peek() == '(' {
					// A process substitution stays text in the group,
					// as before. Its `(` is read here, so an empty one,
					// `<()`, is not taken for a `()` token (#522).
					p.rune()
					p.rxOpenParens++
					continue
				}
				if r == '<' {
					switch p.zshNumGlobAhead() {
					case numGlobYes:
						for p.r != '>' && p.r != runeEOF {
							p.rune()
						}
						continue
					case numGlobLong:
						// Too long to peek: read it, as #511 does.
						if p.zshNumGlobConsume() {
							continue
						}
						p.tok, p.val = _LitWord, p.endLit()
						return
					}
				}
				p.zshBrokenGroup = p.rxGroupStart
				p.tok, p.val = _LitWord, p.endLit()
				p.quote = testExpr
				return
			}
			if p.rxOpenParens <= 0 {
				p.tok, p.val = _LitWord, p.endLit()
				p.quote = testExpr
				return
			}
		case ' ', '\t', '\r', '\n':
			if p.rxOpenParens <= 0 {
				p.tok, p.val = _LitWord, p.endLit()
				p.quote = testExpr
				return
			}
		case '"', '\'', '$', '`':
			p.tok, p.val = _Lit, p.endLit()
			return
		case runeEOF:
			p.tok, p.val = _LitWord, p.endLit()
			p.quote = noState
			return
		}
	}
}

func testUnaryOp(val string) UnTestOperator {
	switch val {
	case "!":
		return TsNot
	case "-e", "-a":
		return TsExists
	case "-f":
		return TsRegFile
	case "-d":
		return TsDirect
	case "-c":
		return TsCharSp
	case "-b":
		return TsBlckSp
	case "-p":
		return TsNmPipe
	case "-S":
		return TsSocket
	case "-L", "-h":
		return TsSmbLink
	case "-k":
		return TsSticky
	case "-g":
		return TsGIDSet
	case "-u":
		return TsUIDSet
	case "-G":
		return TsGrpOwn
	case "-O":
		return TsUsrOwn
	case "-N":
		return TsModif
	case "-r":
		return TsRead
	case "-w":
		return TsWrite
	case "-x":
		return TsExec
	case "-s":
		return TsNoEmpty
	case "-t":
		return TsFdTerm
	case "-z":
		return TsEmpStr
	case "-n":
		return TsNempStr
	case "-o":
		return TsOptSet
	case "-v":
		return TsVarSet
	case "-R":
		return TsRefVar
	default:
		return 0
	}
}

func testBinaryOp(val string) BinTestOperator {
	switch val {
	case "=":
		return TsMatchShort
	case "==":
		return TsMatch
	case "!=":
		return TsNoMatch
	case "=~":
		return TsReMatch
	case "-nt":
		return TsNewer
	case "-ot":
		return TsOlder
	case "-ef":
		return TsDevIno
	case "-eq":
		return TsEql
	case "-ne":
		return TsNeq
	case "-le":
		return TsLeq
	case "-ge":
		return TsGeq
	case "-lt":
		return TsLss
	case "-gt":
		return TsGtr
	default:
		return 0
	}
}
