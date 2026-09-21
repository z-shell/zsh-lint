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
// closer, scanning back over whitespace and comments only. Anything else between
// the operator and the closer means the operator had a right operand and the
// error belongs to something this adapter must not touch.
func findDanglingAndOrBefore(src []byte, seed int) (int, bool) {
	if seed <= 0 || seed > len(src) {
		return 0, false
	}
	i := seed - 1
	for i > 0 {
		switch src[i] {
		case ' ', '	', '\n', '\r':
			i--
			continue
		}
		break
	}
	if i < 1 {
		return 0, false
	}
	// i now rests on the last non-space byte before the closer.
	start := i - 1
	if src[start] != src[i] || (src[i] != '&' && src[i] != '|') {
		return 0, false
	}
	if !danglingOperatorEndsList(src, i+1) {
		return 0, false
	}
	return start, true
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
// terminator after the newline makes the operator dangling.
func danglingOperatorEndsList(src []byte, i int) bool {
	for i < len(src) {
		i = skipSpacesAndComments(src, i)
		if i >= len(src) {
			// End of input closes the outermost list.
			return true
		}
		switch src[i] {
		case '\n':
			i++
			continue
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
	return true
}
