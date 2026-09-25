package parse

// Source scanning helpers the compatibility adapters share. They read raw
// source bytes to find a construct's sites before the parser sees them.

// statementSeparatorRequired is the parser error a construct gives when the
// parser reads it as one command followed by another on the same line.
const statementSeparatorRequired = "statements must be separated by &, ; or a newline"

func matchSourceWord(src []byte, i int, word string) bool {
	if i+len(word) > len(src) {
		return false
	}
	if string(src[i:i+len(word)]) != word {
		return false
	}
	if i+len(word) < len(src) {
		next := src[i+len(word)]
		if next != ' ' && next != '\t' && next != '\n' && next != ';' && next != '&' && next != '|' && next != '(' && next != '{' {
			return false
		}
	}
	return true
}

// skipInlineSpaces skips spaces, tabs, and line continuations, stopping at a
// newline, comment, or any other byte.
func skipInlineSpaces(src []byte, i int) int {
	for i < len(src) {
		switch {
		case src[i] == ' ' || src[i] == '\t':
			i++
		case src[i] == '\\' && i+1 < len(src) && src[i+1] == '\n':
			i += 2
		default:
			return i
		}
	}
	return i
}

func skipSpacesAndComments(src []byte, i int) int {
	for i < len(src) {
		b := src[i]
		if b == ' ' || b == '\t' || b == '\n' {
			i++
			continue
		}
		if b == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		break
	}
	return i
}

func skipAlternateConditionSpaces(src []byte, i int) int {
	for {
		next := skipSpacesAndComments(src, i)
		if next < len(src) && src[next] == '\\' && next+1 < len(src) && src[next+1] == '\n' {
			i = next + 2
			continue
		}
		return next
	}
}

// scanClosingDoubleBracket returns the offset just past the `]]` that closes
// the conditional expression opened at start, or -1. Like the parser, it
// accepts `]]` only as a whole word: `x]]`, `[^\]]`, `([]])`, `(x)]]`, and
// `"]]"` are part of the pattern or string that contains them. A `]]` counts
// only at a word start or right after the `)` that closes a condition group,
// and before whitespace, a separator, `)`, or the end of the source.
//
// A `(` opens a condition group only where a condition may start: after
// `[[`, another group open, `&&`, `||`, or `!`. Elsewhere it is part of a
// pattern word, as in `$a == (x)]] ]]`, and its `)` does not end the group.
func scanClosingDoubleBracket(src []byte, start int) int {
	var (
		inSingle, inDouble, inANSIC bool
		groupDepth                  int // open condition groups
		patternDepth                int // open pattern parens in the current word
		wordLen                     int
		wordIsBang                  bool
		atWordStart                 = true
		atConditionStart            = true // a `(` here opens a group
		afterGroupClose             bool
	)
	endWord := func() {
		if wordLen > 0 {
			atConditionStart = wordIsBang
		}
		wordLen = 0
		wordIsBang = false
		patternDepth = 0
		atWordStart = true
	}
	wordByte := func(b byte) {
		wordIsBang = wordLen == 0 && b == '!'
		wordLen++
		atWordStart = false
		afterGroupClose = false
	}
	for i := start + 2; i < len(src); i++ {
		b := src[i]
		switch {
		case inSingle:
			inSingle = b != '\''
		case inANSIC:
			switch b {
			case '\\':
				i++
			case '\'':
				inANSIC = false
			}
		case inDouble:
			switch b {
			case '\\':
				i++
			case '"':
				inDouble = false
			}
		case b == '\\':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
				endWord()
				continue
			}
			wordByte(b)
			i++
		case b == '\'':
			inSingle = true
			wordByte(b)
		case b == '"':
			inDouble = true
			wordByte(b)
		case b == '$' && i+1 < len(src) && src[i+1] == '\'':
			inANSIC = true
			wordByte(b)
			i++
		case isConditionWordSpace(b):
			// Whitespace inside `$( f )`, `$(( a + 1 ))`, or `(x|y z)` stays
			// inside the word.
			if patternDepth == 0 {
				endWord()
			}
		case (b == '&' || b == '|') && i+1 < len(src) && src[i+1] == b:
			endWord()
			atConditionStart = true
			afterGroupClose = false
			i++
		case b == '(':
			if atWordStart && atConditionStart {
				groupDepth++
				afterGroupClose = false
				continue
			}
			patternDepth++
			wordByte(b)
		case b == ')':
			if patternDepth > 0 {
				patternDepth--
				wordByte(b)
				continue
			}
			if groupDepth == 0 {
				return -1
			}
			groupDepth--
			endWord()
			atConditionStart = false
			afterGroupClose = true
		case b == ']' && i+1 < len(src) && src[i+1] == ']' && (atWordStart || afterGroupClose) && (i+2 == len(src) || isConditionWordEnd(src[i+2])):
			return i + 2
		default:
			wordByte(b)
		}
	}
	return -1
}

func scanClosingDoubleParen(src []byte, start int) int {
	i := start + 2
	depth := 1
	for i < len(src) {
		if src[i] == '(' && i+1 < len(src) && src[i+1] == '(' {
			depth++
			i += 2
			continue
		}
		if src[i] == ')' && i+1 < len(src) && src[i+1] == ')' {
			depth--
			if depth == 0 {
				return i + 2
			}
			i += 2
			continue
		}
		i++
	}
	return -1
}

func scanClosingBrace(src []byte, start int) int {
	i := start + 1
	depth := 1
	for i < len(src) {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
		i++
	}
	return -1
}

func isConditionWordSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n'
}

func isConditionWordEnd(b byte) bool {
	return isConditionWordSpace(b) || b == ';' || b == '&' || b == '|' || b == ')'
}
