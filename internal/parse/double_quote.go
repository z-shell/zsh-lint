package parse

import "strings"

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
// quoting. A `case` command or a heredoc is refused: a case pattern's `)`
// opens nothing, and a heredoc body is not command text.
//
// Whether a `#` starts a comment depends on whether the `(` before it is a
// token or part of a word, and that depends on the grammar position.
// `( # c )` and `(# c )` in command position open a subshell holding a
// comment, as do `<(# c )` and an array assignment's `a=(# c )`, while
// `print (#i)x` and `[[ a == (#b)(*) ]]` hold glob flags. The scan tracks that
// position, reads glob and arithmetic groups as literal text, and refuses a
// comment inside a `(` whose role it cannot tell.
func skipCommandSubstitution(src []byte, open int) (int, bool) {
	var frames []substFrame
	pos := substCommand
	wordStart := true
	assignParen := -1
	for i := open + 1; i < len(src); i++ {
		b := src[i]
		atWord := wordStart
		wordStart = false
		kind := frameCommand // the substitution's own list
		if len(frames) > 0 {
			kind = frames[len(frames)-1].kind
		}
		literal := kind == frameGlob || kind == frameArithmetic
		if atWord && !literal && !isSubstWordBoundary(b) {
			next, paren, ok := classifySubstWord(src, i, pos, kind)
			if !ok {
				return 0, false
			}
			pos, assignParen = next, paren
		}
		switch b {
		case ' ', '\t':
			wordStart = true
		case '\n', ';', '&', '|':
			wordStart = true
			if literal || kind == frameArray || kind == frameUnknown {
				continue
			}
			if (b == '&' || b == '|') && i > 0 && (src[i-1] == '<' || src[i-1] == '>') {
				continue // `>&2`, `>|` and `<&0` are redirections
			}
			pos = substCommand
		case '\\':
			i++
		case '(':
			wordStart = true
			frame := substFrame{kind: frameUnknown, outer: pos, open: i}
			switch {
			case literal:
				frame.kind = frameGlob
			case !atWord && (src[i-1] == '<' || src[i-1] == '>'):
				frame.kind = frameCommand // process substitution
			case !atWord && i == assignParen:
				frame.kind = frameArray
			case !atWord && src[i-1] == '=':
				if i-2 <= open || isSubstWordBoundary(src[i-2]) {
					frame.kind = frameCommand // `=(...)` process substitution
				}
			case kind == frameArray:
				frame.kind = frameGlob // `a=( (#i)x *(N) )`
			case atWord && pos == substCommand:
				frame.kind = frameCommand
				if i+1 < len(src) && src[i+1] == '(' {
					frame.kind = frameArithmetic
					i++
				}
			case pos == substArgument || pos == substDeclaration:
				frame.kind = frameGlob // `print (#i)x`, `local a (#i)x`
			}
			frames = append(frames, frame)
			switch frame.kind {
			case frameCommand:
				pos = substCommand
			case frameUnknown:
				pos = substUnknown
			}
		case ')':
			wordStart = true
			if len(frames) == 0 {
				return i, true
			}
			top := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			pos = substUnknown
			switch {
			case top.kind == frameArithmetic:
				if i+1 >= len(src) || src[i+1] != ')' {
					return 0, false
				}
				i++
			case top.kind == frameArray:
				// `local a=(x) b=(y)`: the next word keeps the position
				// the assignment had.
				pos = top.outer
			case top.open == i-1:
				// `f ()` and `print a ()` define functions: the body is
				// a command.
				pos = substCommand
			case top.kind == frameGlob:
				// A glob group ends inside the word it belongs to.
				pos = top.outer
				wordStart = false
			}
		case '#':
			if literal || !atWord {
				continue
			}
			if kind == frameUnknown {
				return 0, false
			}
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case '<', '>':
			if literal {
				continue
			}
			if b == '<' && i+2 < len(src) && src[i+1] == '<' && src[i+2] == '<' {
				i += 2 // a herestring is one word of command text
				continue
			}
			if b == '<' && i+1 < len(src) && src[i+1] == '<' {
				return 0, false
			}
		case '\'':
			if kind == frameArithmetic {
				return 0, false
			}
			end, ok := skipSingleQuoted(src, i)
			if !ok {
				return 0, false
			}
			i = end
		case '"':
			if kind == frameArithmetic {
				return 0, false
			}
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
		}
	}
	return 0, false
}

// substPos is the grammar position of the next word inside a command
// substitution.
type substPos int

const (
	substCommand     substPos = iota // a command may start here
	substArgument                    // an argument of a simple command
	substDeclaration                 // an argument of a declaration builtin
	substUnknown                     // not decidable from the bytes scanned
)

// substFrameKind is the role of a `(` inside a command substitution.
type substFrameKind int

const (
	frameCommand    substFrameKind = iota // subshell or process substitution
	frameArray                            // array assignment: words and comments
	frameGlob                             // glob group: literal text
	frameArithmetic                       // `(( ... ))` command: literal text
	frameUnknown                          // a `(` the scan cannot place
)

type substFrame struct {
	kind  substFrameKind
	outer substPos
	open  int
}

// substReservedWords are the reserved words and precommand modifiers after
// which the next word is not an ordinary argument. keepsCommandPosition holds
// the ones after which a command may still start.
var (
	substReservedWords = map[string]bool{
		"!": true, "[[": true, "]]": true, "{": true, "}": true, "-": true,
		"always": true, "builtin": true, "case": true, "command": true,
		"coproc": true, "declare": true, "do": true, "done": true, "elif": true,
		"else": true, "end": true, "esac": true, "exec": true, "export": true,
		"fi": true, "float": true, "for": true, "foreach": true,
		"function": true, "if": true, "integer": true, "local": true,
		"nocorrect": true, "noglob": true, "readonly": true, "repeat": true,
		"select": true, "then": true, "time": true, "typeset": true,
		"until": true, "while": true,
	}
	precommandModifiers = map[string]bool{
		"-": true, "builtin": true, "command": true, "exec": true,
		"nocorrect": true, "noglob": true,
	}
	// declarationBuiltins parse `name=(...)` arguments as array
	// assignments.
	declarationBuiltins = map[string]bool{
		"declare": true, "export": true, "float": true, "integer": true,
		"local": true, "readonly": true, "typeset": true,
	}
	patternOperators = map[string]bool{
		"=": true, "==": true, "!=": true, "=~": true,
	}
	keepsCommandPosition = map[string]bool{
		"!": true, "{": true, "do": true, "elif": true, "else": true,
		"if": true, "then": true, "time": true, "until": true, "while": true,
	}
)

// isSubstWordBoundary reports whether b cannot start a word: a blank, a
// separator, a parenthesis, a comment or a redirection operator.
func isSubstWordBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', '(', ')', '#', '<', '>':
		return true
	}
	return false
}

// isSubstWordSpecial reports whether b starts quoting or an expansion, which
// ends the bare prefix of a word that classifySubstWord reads.
func isSubstWordSpecial(b byte) bool {
	switch b {
	case '\'', '"', '`', '$', '\\':
		return true
	}
	return false
}

// classifySubstWord reports the position after the word starting at i and,
// for an array assignment `name=(`, the offset of its `(`. It refuses a
// `case` command.
func classifySubstWord(src []byte, i int, pos substPos, kind substFrameKind) (substPos, int, bool) {
	if kind == frameArray {
		return pos, -1, true // array elements are data
	}
	end := i
	for end < len(src) && !isSubstWordBoundary(src[end]) && !isSubstWordSpecial(src[end]) {
		end++
	}
	word := string(src[i:end])
	separated := end == len(src)
	if !separated {
		switch src[end] {
		case ' ', '\t', '\n', ';', '&', '|':
			separated = true
		}
	}
	if word == "case" && separated && pos != substArgument && pos != substDeclaration {
		return 0, -1, false
	}
	if pos != substCommand && separated && patternOperators[word] {
		// The operand of `[[ a == pat ]]` is a pattern, so a `(` there
		// opens a glob group: `[[ a == (#b)(*) ]]`.
		return substArgument, -1, true
	}
	switch pos {
	case substCommand:
		switch {
		case word == "":
			return substUnknown, -1, true // a quoted or expanded command word
		case substReservedWords[word]:
			if separated && keepsCommandPosition[word] {
				return substCommand, -1, true
			}
			if separated && precommandModifiers[word] {
				// The next word names a command, and a `(` in it is a
				// word byte: `noglob (# c )` runs a command of that name.
				return substArgument, -1, true
			}
			if separated && declarationBuiltins[word] {
				return substDeclaration, -1, true
			}
			return substUnknown, -1, true
		case end < len(src) && src[end] == '(':
			// `name=(` assigns an array; `f()` defines a function.
			if isArrayAssignmentPrefix(word) {
				return substUnknown, end, true
			}
			return substUnknown, -1, true
		case strings.Contains(word, "="):
			return substUnknown, -1, true // an assignment prefix
		}
		return substArgument, -1, true
	case substArgument:
		if substReservedWords[word] {
			return substUnknown, -1, true
		}
	case substDeclaration:
		// `local a=(# c` assigns an array, as in command position; any
		// other `(` starting a word opens a glob group.
		if end < len(src) && src[end] == '(' && isArrayAssignmentPrefix(word) {
			return substDeclaration, end, true
		}
	}
	return pos, -1, true
}

// isArrayAssignmentPrefix reports whether word is exactly `name=` or
// `name+=`, the part of an array assignment before its `(`.
func isArrayAssignmentPrefix(word string) bool {
	name, ok := strings.CutSuffix(word, "=")
	if !ok {
		return false
	}
	name = strings.TrimSuffix(name, "+")
	if name == "" {
		return false
	}
	for k := 0; k < len(name); k++ {
		c := name[k]
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (k == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
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
