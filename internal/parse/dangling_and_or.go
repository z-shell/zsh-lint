package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parseDanglingAndOr adapts a native-Zsh list whose final operator has no right
// operand: `print a &&` at the end of a list is valid Zsh, and runs the left
// operand exactly as a bare `print a` would.
//
// Zsh accepts `&&` and `||` (but not `|`) as the last token of a list, where the
// list is closed by a terminator rather than continued by another statement. The
// retry replaces the operator with `; `, leaving the left operand as a complete
// statement and every later byte position unchanged.
//
// The empty right operand is why this is not the #327 empty-body family: there
// the operator has both operands and the loop is the left one.
func parseDanglingAndOr(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	// Two distinct reports describe the same source shape. When the operator ends
	// the outermost list, the parser stops at the operator itself. When a closer
	// follows, the parser consumes the operator, then reports the closer as
	// unexpected because the statement it was building is incomplete.
	//
	// The closer wording varies by keyword ("can only be used to close a block",
	// "can only be used in an `if`"), so match the shared prefix rather than any
	// one phrasing.
	text := parseErr.Text
	atOperator := strings.Contains(text, "must be followed by a statement")
	atCloser := strings.Contains(text, "can only be used") ||
		strings.Contains(text, "statement must end with")
	if !atOperator && !atCloser {
		return nil, firstErr
	}

	seed := int(parseErr.Pos.Offset())
	var offset int
	var ok bool
	if atOperator {
		offset, ok = findDanglingAndOr(src, seed)
	} else {
		offset, ok = findDanglingAndOrBefore(src, seed)
	}
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	// Replace the operator with a separator, not blanks. A closer on the same
	// line (`do print a && done`) needs the statement terminated, or the masked
	// source reads as `print a done` and the closer becomes an argument. A `;`
	// plus a space keeps every later byte position unchanged.
	masked[offset] = ';'
	masked[offset+1] = ' '

	// Re-enter the full chain: a dangling operator inside a `while` header leaves
	// an empty loop body, which is the #327 adapter's job.
	return parseWithAdapters(masked, name)
}

// findDanglingAndOrBefore locates a dangling operator that precedes a reported
// closer. Only list trivia may stand between the two: blanks, newlines, a `\`
// line continuation, and comments (#466). Anything else means the operator had
// a right operand and the error belongs to something this adapter must not
// touch.
//
// The walk back from the closer only proposes a candidate; skipListTrivia then
// confirms it forward from the operator, where Zsh's own reading of the same
// bytes is unambiguous. A proposal that does not reach the closer exactly is
// refused.
func findDanglingAndOrBefore(src []byte, seed int) (int, bool) {
	// seed is the closer's offset; len(src) stands for end of input.
	if seed > len(src) {
		return 0, false
	}
	start, ok := danglingOperatorCandidate(src, seed)
	if !ok {
		return 0, false
	}
	if skipListTrivia(src, start+2) != seed {
		return 0, false
	}
	if !danglingOperatorEndsList(src, start+2) {
		return 0, false
	}
	return start, true
}

// danglingOperatorCandidate walks back from seed over blanks, newlines and
// line continuations and returns the offset of the `&&` or `||` it lands on.
// When it lands inside a line instead, that line may end in a comment: the
// operator is then the last token before the line's first comment, and a line
// holding nothing but a comment is stepped over like a blank one.
//
// The comment test is a proposal, not a proof. A `#` inside quotes can look
// like a comment start here; the forward check in findDanglingAndOrBefore
// rejects any candidate whose following bytes are not really trivia.
func danglingOperatorCandidate(src []byte, seed int) (int, bool) {
	i := skipTriviaBackward(src, seed)
	// The operator that ends the closest line, read without any comment
	// handling. It is the fallback when the comment reading finds nothing:
	// a `#` line may be the inside of a multi-line quote (`'a⏎# x' ||`).
	last := i - 2
	for {
		lineStart := bytes.LastIndexByte(src[:i], '\n') + 1
		// Read the line as code ending at its first comment, so an operator
		// written inside a comment (`# c ||`) is never taken for the real one.
		if hash := firstCommentStart(src[lineStart:i]); hash >= 0 {
			before := max(lineStart, skipBlanksBackward(src, lineStart+hash))
			if before == lineStart {
				// The whole line is a comment: keep walking back.
				i = skipTriviaBackward(src, lineStart)
				continue
			}
			if isAndOrOperatorAt(src, before-2) {
				return before - 2, true
			}
			// The `#` may sit inside quotes (`"a #b" &&`); fall through and try
			// the line's real last token.
		}
		if isAndOrOperatorAt(src, i-2) {
			return i - 2, true
		}
		if isAndOrOperatorAt(src, last) {
			return last, true
		}
		return 0, false
	}
}

// skipTriviaBackward returns the offset just past the last byte before end that
// is not a blank, a newline, or part of a `\` line continuation.
func skipTriviaBackward(src []byte, end int) int {
	i := end
	for i > 0 {
		switch src[i-1] {
		case ' ', '	':
			i--
			continue
		case '\n':
			i--
			// Only an unescaped `\` continues the line: `\\` before the
			// newline is an escaped backslash, which is a word.
			n := 0
			for i-n > 0 && src[i-n-1] == '\\' {
				n++
			}
			if n%2 == 1 {
				i--
			}
			continue
		}
		return i
	}
	return i
}

// firstCommentStart returns the offset in line of the first `#` that begins a
// word, or -1. A `#` glued to a preceding word byte (`a#b`, `$#`, `${#x}`,
// `(#i)`) does not start a comment. An operator byte ends a word, so `||#c`
// does.
func firstCommentStart(line []byte) int {
	for j, b := range line {
		if b != '#' {
			continue
		}
		if j == 0 {
			return 0
		}
		switch line[j-1] {
		case ' ', '	', ';', '&', '|':
			return j
		}
	}
	return -1
}

// isAndOrOperatorAt reports whether src holds `&&` or `||` at i.
func isAndOrOperatorAt(src []byte, i int) bool {
	return i >= 0 && i+1 < len(src) && src[i] == src[i+1] && (src[i] == '&' || src[i] == '|')
}

// skipListTrivia returns the first offset at or after i that is not a blank, a
// newline, a comment, or a `\` line continuation: the bytes Zsh reads between
// a list operator and whatever follows it.
func skipListTrivia(src []byte, i int) int {
	for {
		i = skipSpacesAndComments(src, i)
		if i+1 < len(src) && src[i] == '\\' && src[i+1] == '\n' {
			i += 2
			continue
		}
		return i
	}
}

// findDanglingAndOr reports the offset of a `&&` or `||` that ends its list.
//
// The reported error position is the operator itself, but the same message is
// produced for operators that are NOT dangling — among them `a && && b` and a
// dangling `|`, both of which native Zsh rejects. So the operator is confirmed
// by what follows it: only a list terminator makes it dangling.
//
// The lookahead is defense in depth rather than the only thing holding the line.
// Disabling it changes no verdict across 216 generated operator/closer
// combinations, because an operator that has a right operand parses cleanly and
// never reaches an adapter, and a masked invalid source fails again downstream.
// It is kept because it is what makes this function correct in isolation: the
// predicate is unit-tested directly, and a later gate change could make it
// load-bearing without warning.
func findDanglingAndOr(src []byte, seed int) (int, bool) {
	if seed < 0 || seed >= len(src) {
		return 0, false
	}

	i := seed
	// The parser reports the operator start, but tolerate a seed inside it.
	if i > 0 && (src[i] == '&' || src[i] == '|') && src[i-1] == src[i] {
		i--
	}
	if i+1 >= len(src) {
		return 0, false
	}
	// `|` alone is a pipe: Zsh requires a right operand, so it is never dangling.
	if src[i] != src[i+1] || (src[i] != '&' && src[i] != '|') {
		return 0, false
	}

	if !danglingOperatorEndsList(src, i+2) {
		return 0, false
	}
	// No `if`/`elif` special case belongs here. An operator in a condition header
	// is either followed by `then`, which is valid Zsh the base parser already
	// accepts, or by a terminator that leaves the `if` incomplete, which still
	// fails after masking. Adding the check changed no verdict, and an
	// unreachable guard reads as real protection.
	return i, true
}

// danglingOperatorEndsList reports whether the bytes after an operator close its
// list instead of supplying a right operand.
//
// Newlines are skipped: Zsh reads an operator's right operand across a line
// break, so `print a &&\nprint b` is an ordinary two-statement list and only a
// terminator after the newline makes the operator dangling. A `\` line
// continuation is skipped for the same reason (#466).
func danglingOperatorEndsList(src []byte, i int) bool {
	i = skipListTrivia(src, i)
	if i >= len(src) {
		// End of input closes the outermost list.
		return true
	}
	switch src[i] {
	case ';':
		// `;` terminates the list; `;;` and `;&` terminate a case arm.
		return true
	case '}', ')':
		// A closing brace, subshell or case-arm paren.
		return true
	case '&':
		// `&` backgrounds the left operand and ends the list, but `&&` is a
		// second operator, which Zsh rejects.
		return i+1 >= len(src) || src[i+1] != '&'
	case '|':
		// Another operator with no left operand: invalid in Zsh.
		return false
	}
	// A keyword that closes the enclosing construct behaves as a terminator.
	for _, kw := range [...]string{"done", "fi", "esac", "then", "else", "elif"} {
		if matchSourceWord(src, i, kw) {
			return true
		}
	}
	// Anything else is a real right operand.
	return false
}
