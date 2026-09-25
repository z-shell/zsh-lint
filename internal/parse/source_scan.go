package parse

// Source scanning helpers the compatibility adapters share. They read raw
// source bytes to find a construct's sites before the parser sees them.

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

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// scanCommandWords calls visit for every identifier word in command position
// in syntactically active source. Native Zsh recognises a reserved word at
// the start of a command: after a separator, an opening `(` or `{`, a case
// pattern's `)`, a `!`, or a reserved word that itself precedes a command.
// Quoted text, comments, heredoc bodies and arithmetic never hold one. A
// visit that consumes the word's operands returns the offset to resume at
// and true; the next word is then in command position again. Otherwise the
// word is treated by its own meaning: a reserved word that precedes a
// command keeps command position, any other word ends it.
func scanCommandWords(src []byte, visit func(start, end int, word string) (int, bool)) {
	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false
	inBacktick := false
	var heredocs []pendingHeredoc
	arithmeticDepth := 0
	// parens records, for each open `(`, whether it opened a subshell or a
	// command substitution, whose `)` ends a command, rather than a case
	// pattern, an array value or a glob group, whose `)` may precede one.
	var parens []bool
	atCommandStart := true
	// repeatCount tracks the count word after a `repeat` in command
	// position: 1 before it starts, 2 inside it. The count is not a command,
	// but the word after it is (`repeat 2 do`), as native par_repeat reads.
	repeatCount, repeatParens := 0, 0

	for i := 0; i < len(src); i++ {
		b := src[i]
		if escaped {
			escaped = false
			continue
		}
		if inSingleQuote {
			if b == '\'' {
				inSingleQuote = false
			}
			continue
		}
		if inDoubleQuote {
			switch b {
			case '\\':
				escaped = true
			case '"':
				inDoubleQuote = false
			}
			continue
		}
		if inANSICQuote {
			switch b {
			case '\\':
				escaped = true
			case '\'':
				inANSICQuote = false
			}
			continue
		}
		if inComment {
			if b == '\n' {
				inComment = false
				atCommandStart = true
			}
			continue
		}
		if b == '\n' && len(heredocs) > 0 {
			next, ok := consumeHeredocBodies(src, i+1, heredocs)
			if !ok {
				return
			}
			heredocs = nil
			i = next - 1
			atCommandStart = true
			continue
		}
		if b == '(' && i+1 < len(src) && src[i+1] == '(' {
			arithmeticDepth++
			i++
			continue
		}
		if arithmeticDepth > 0 {
			if b == ')' && i+1 < len(src) && src[i+1] == ')' {
				arithmeticDepth--
				i++
			}
			continue
		}

		if repeatCount == 1 && b != ' ' && b != '\t' {
			repeatCount, repeatParens = 2, len(parens)
		}
		switch b {
		case '\\':
			escaped = true
			if i+1 >= len(src) || src[i+1] != '\n' {
				atCommandStart = false
			}
			continue
		case '\'':
			inSingleQuote = true
			atCommandStart = false
			continue
		case '"':
			atCommandStart = false
			if end, ok := skipDoubleQuotedString(src, i); ok {
				i = end
				continue
			}
			inDoubleQuote = true
			continue
		case '#':
			if i == 0 || isCommandWordBoundary(src[i-1]) {
				inComment = true
			} else {
				atCommandStart = false
			}
			continue
		case '$':
			if i+1 < len(src) {
				switch src[i+1] {
				case '\'':
					inANSICQuote = true
					i++
					atCommandStart = false
					continue
				case '{':
					i++
					atCommandStart = false
					continue
				case '(':
					if i+2 < len(src) && src[i+2] == '(' {
						arithmeticDepth++
						i += 2
						continue
					}
					parens = append(parens, true)
					i++
					atCommandStart = true
					continue
				}
			}
			atCommandStart = false
			continue
		case ' ', '\t':
			if repeatCount == 2 && len(parens) == repeatParens {
				repeatCount = 0
				atCommandStart = true
			}
			continue
		case '\n', ';', '&', '|', '{':
			repeatCount = 0
			atCommandStart = true
			continue
		case '}':
			atCommandStart = false
			continue
		case '(':
			// An array value `name=(...)` holds words, not commands; any
			// other `(` may open a subshell, so its first word is a site.
			if i > 0 && src[i-1] == '=' {
				parens = append(parens, false)
				atCommandStart = false
				continue
			}
			parens = append(parens, atCommandStart)
			atCommandStart = true
			continue
		case ')':
			endsCommand := false
			if len(parens) > 0 {
				endsCommand = parens[len(parens)-1]
				parens = parens[:len(parens)-1]
			}
			atCommandStart = !endsCommand
			continue
		case '`':
			inBacktick = !inBacktick
			atCommandStart = inBacktick
			continue
		case '!':
			if i+1 >= len(src) || (src[i+1] != ' ' && src[i+1] != '\t') {
				atCommandStart = false
			}
			continue
		case '<', '>':
			if b == '<' && i+1 < len(src) && src[i+1] == '<' {
				heredoc, end, isHeredoc, ok := heredocAt(src, i)
				if !ok {
					return
				}
				if !isHeredoc {
					i += 2
					atCommandStart = false
					continue
				}
				heredocs = append(heredocs, heredoc)
				i = end - 1
				atCommandStart = false
				continue
			}
			if i+1 < len(src) && src[i+1] == '&' {
				i++
			}
			atCommandStart = false
			continue
		}
		if !isIdentByte(b) {
			atCommandStart = false
			continue
		}
		start := i
		for i+1 < len(src) && isIdentByte(src[i+1]) {
			i++
		}
		word := string(src[start : i+1])
		glued := start > 0 && !isCommandWordBoundary(src[start-1])
		followed := i+1 < len(src) && !isCommandWordEnd(src[i+1])
		switch {
		case glued || followed:
			atCommandStart = false
		case atCommandStart:
			if resume, ok := visit(start, i+1, word); ok {
				i = resume - 1
				continue
			}
			if word == "repeat" {
				repeatCount = 1
			}
			if !commandPrefixWords[word] {
				atCommandStart = false
			}
		default:
			atCommandStart = false
		}
	}
}

// isCommandWordBoundary reports whether b separates words and leaves the
// next one in a position where a reserved word is recognised.
func isCommandWordBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', '(', '`':
		return true
	}
	return false
}

// isCommandWordEnd reports whether b ends a command word.
func isCommandWordEnd(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', ')', '(', '`', '<', '>':
		return true
	}
	return false
}

// commandPrefixWords are the reserved words after which the next word
// is still in command position, so a reserved word there is recognised.
var commandPrefixWords = map[string]bool{
	"then":  true,
	"else":  true,
	"do":    true,
	"if":    true,
	"elif":  true,
	"while": true,
	"until": true,
	"time":  true,
}
