package parse

// Double-quoted strings hold command substitutions, parameter expansions and
// backquoted commands, and each of those starts its own quoting context: in
// `"$(print "it's")"` the inner `"` opens a nested string and the `'` inside
// it is a literal byte. A scanner that leaves a double-quoted string at the
// next `"` reads that inner `"` as the end, then takes the `'` for the start
// of a single-quoted region reaching far down the file, and misses every
// construct it holds (#393).
//
// skipDoubleQuotedString is the shared rule the site scanners use instead. It
// returns false where it cannot tell the extent for certain (an unterminated
// string, a heredoc or a `case` pattern inside a substitution), and callers
// then fall back to their byte-at-a-time handling, which is what they did
// before.

// skipDoubleQuotedString reports the offset of the `"` closing the string
// opened at open.
func skipDoubleQuotedString(src []byte, open int) (int, bool) {
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '"':
			return i, true
		case '`':
			end, ok := skipBackquoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '$':
			end, ok := skipDollarExpansion(src, i, true)
			if !ok {
				return 0, false
			}
			i = end
		}
	}
	return 0, false
}

// skipBackquoted reports the offset of the backquote closing the one at open.
// Zsh finds that end by text alone: only a backslash protects a backquote, so
// a quote inside the body does not.
func skipBackquoted(src []byte, open int) (int, bool) {
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '`':
			return i, true
		}
	}
	return 0, false
}

// skipDollarExpansion reports the offset of the last byte of the expansion
// introduced by the `$` at dollar: `$(...)`, `$((...))`, `${...}` or
// `$[...]`. A `$` introducing none of them is its own last byte. quoted
// reports whether the `$` stands inside a double-quoted string, where a `'`
// in `${...}` is an ordinary byte.
func skipDollarExpansion(src []byte, dollar int, quoted bool) (int, bool) {
	next := dollar + 1
	if next >= len(src) {
		return dollar, true
	}
	switch src[next] {
	case '(':
		if next+1 < len(src) && src[next+1] == '(' {
			return skipArithmeticExpansion(src, next+1)
		}
		return skipCommandSubstitution(src, next)
	case '{':
		return skipBracedParameter(src, next, quoted)
	case '[':
		return skipArithmeticExpansion(src, next)
	}
	return dollar, true
}

// skipArithmeticExpansion reports the offset of the last byte closing the
// arithmetic expansion whose inner opener is at open: the second `)` of
// `$((...))`, or the `]` of `$[...]`. A quote or a nested expansion inside is
// refused rather than modelled; `$(( (a) ))` and a command substitution that
// merely starts with a subshell, `$( (a) )`, differ only by a space.
func skipArithmeticExpansion(src []byte, open int) (int, bool) {
	closer := byte(')')
	if src[open] == '[' {
		closer = ']'
	}
	depth := 0
	for i := open + 1; i < len(src); i++ {
		switch b := src[i]; b {
		case '\'', '"', '`', '$', '\\':
			return 0, false
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
				continue
			}
			if b != closer {
				return 0, false
			}
			if closer == ']' {
				return i, true
			}
			if i+1 < len(src) && src[i+1] == ')' {
				return i + 1, true
			}
			return 0, false
		}
	}
	return 0, false
}

// skipBracedParameter reports the offset of the `}` closing the parameter
// expansion whose `{` is at open. Outside double quotes Zsh nests braces in
// the word, so `${x:-{a}b}` is one expansion; inside them the first `}`
// closes it, so `"${x:-{a}b}"` expands to `{ab}` and `"${x:-{}"` is complete.
func skipBracedParameter(src []byte, open int, quoted bool) (int, bool) {
	depth := 0
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '{':
			if !quoted {
				depth++
			}
		case '}':
			if depth == 0 {
				return i, true
			}
			depth--
		case '\'':
			if quoted {
				continue
			}
			end, ok := skipSingleQuoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '"':
			end, ok := skipDoubleQuotedString(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '`':
			end, ok := skipBackquoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '$':
			if !quoted && i+1 < len(src) && src[i+1] == '\'' {
				// `$'...'` quotes as in a command; inside double quotes `$'` is literal.
				end, ok := skipANSICQuoted(src, i+1)
				if !ok {
					return 0, false
				}
				i = end
				continue
			}
			end, ok := skipDollarExpansion(src, i, quoted)
			if !ok {
				return 0, false
			}
			i = end
		}
	}
	return 0, false
}

// skipCommandSubstitution reports the offset of the `)` closing the command
// substitution whose `(` is at open. The body is a command list with its own
// quoting. A `case` word or a heredoc is refused: a case pattern's `)` opens
// nothing, and a heredoc body is not command text.
func skipCommandSubstitution(src []byte, open int) (int, bool) {
	depth := 0
	wordStart := true
	for i := open + 1; i < len(src); i++ {
		b := src[i]
		atWord := wordStart
		wordStart = false
		switch b {
		case ' ', '\t', '\n', ';', '&', '|':
			wordStart = true
		case '\\':
			i++
		case '(':
			depth++
			wordStart = true
		case ')':
			if depth == 0 {
				return i, true
			}
			depth--
			wordStart = true
		case '#':
			// `(#b)` and `(#q)` open glob flags, not a comment.
			if !atWord || src[i-1] == '(' {
				continue
			}
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case '<':
			if i+2 < len(src) && src[i+1] == '<' && src[i+2] == '<' {
				i += 2 // a herestring is one word of command text
				continue
			}
			if i+1 < len(src) && src[i+1] == '<' {
				return 0, false
			}
		case '\'':
			end, ok := skipSingleQuoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '"':
			end, ok := skipDoubleQuotedString(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '`':
			end, ok := skipBackquoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '$':
			if i+1 < len(src) && src[i+1] == '\'' {
				end, ok := skipANSICQuoted(src, i+1)
				if !ok {
					return 0, false
				}
				i = end
				continue
			}
			end, ok := skipDollarExpansion(src, i, false)
			if !ok {
				return 0, false
			}
			i = end
		default:
			if atWord && isCaseWordAt(src, i) {
				return 0, false
			}
		}
	}
	return 0, false
}

// skipSingleQuoted reports the offset of the `'` closing the one at open.
func skipSingleQuoted(src []byte, open int) (int, bool) {
	for i := open + 1; i < len(src); i++ {
		if src[i] == '\'' {
			return i, true
		}
	}
	return 0, false
}

// skipANSICQuoted reports the offset of the `'` closing the `$'...'` string
// whose opening `'` is at open; a backslash escapes the byte after it.
func skipANSICQuoted(src []byte, open int) (int, bool) {
	for i := open + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'':
			return i, true
		}
	}
	return 0, false
}

// isCaseWordAt reports whether the word starting at i is `case`.
func isCaseWordAt(src []byte, i int) bool {
	const word = "case"
	if i+len(word) > len(src) || string(src[i:i+len(word)]) != word {
		return false
	}
	end := i + len(word)
	if end == len(src) {
		return true
	}
	switch src[end] {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')':
		return true
	}
	return false
}
