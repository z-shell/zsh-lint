package parse

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	invalidSubscriptExpression = "`[` must be followed by an expression"
	invalidSubscriptTernary    = "ternary operator missing `?` before `:`"
	invalidSubscriptArithmetic = "not a valid arithmetic operator:"
	// mvdan/sh reports a key byte read as a binary operator without a left
	// operand as "`<` must follow an expression" and one without a right
	// operand as "`-` must be followed by an expression".
	invalidSubscriptLeftOperand  = " must follow an expression"
	invalidSubscriptRightOperand = " must be followed by an expression"
)

// associativeKeyMask replaces every masked key byte. mvdan/sh keeps `#`
// inside an arithmetic literal without joining it to a preceding bare
// parameter name, so `$M#b` stays a parameter followed by a literal where
// `$M_b` would become one longer parameter name.
const associativeKeyMask = '#'

// parseAssociativeSubscript retries only native-Zsh bare associative keys that
// mvdan/sh mistakes for arithmetic syntax. The retry masks the key's own
// punctuation with same-width literal bytes, leaves any parameter expansion or
// command substitution nested in the key untouched, and restores every changed
// AST literal.
func parseAssociativeSubscript(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseAssociativeSubscriptWithParser(src, name, firstErr, parseWithAdapters)
}

func parseAssociativeSubscriptWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || !isSubscriptParseError(parseErr.Text) {
		return nil, firstErr
	}

	key, ok := findBareAssociativeKey(src, int(parseErr.Pos.Offset()), parseErr.Text)
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	edits := make([]patternEdit, 0, len(key.punctuation))
	for _, offset := range key.punctuation {
		edits = append(edits, patternEdit{offset: offset, original: masked[offset], replacement: associativeKeyMask})
		masked[offset] = associativeKeyMask
	}
	if len(edits) == 0 {
		return nil, firstErr
	}

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if err := restorePatternEdits(tree, src, edits); err != nil {
		return nil, err
	}
	return tree, nil
}

func isSubscriptParseError(text string) bool {
	if text == invalidSubscriptExpression || text == invalidSubscriptTernary ||
		strings.HasPrefix(text, invalidSubscriptArithmetic) {
		return true
	}
	_, ok := subscriptOperatorError(text)
	return ok
}

// subscriptOperatorError returns the single punctuation byte that mvdan/sh
// reported as an arithmetic operator missing an operand.
func subscriptOperatorError(text string) (byte, bool) {
	rest, ok := strings.CutPrefix(text, "`")
	if !ok || len(rest) < 2 || rest[1] != '`' {
		return 0, false
	}
	op, suffix := rest[0], rest[2:]
	if suffix != invalidSubscriptLeftOperand && suffix != invalidSubscriptRightOperand {
		return 0, false
	}
	if !isBareKeyPunctuation(op) {
		return 0, false
	}
	return op, true
}

// isBareKeyPunctuation reports the punctuation the retry masks inside a bare
// associative key. The set stays narrow so arithmetic subscripts on ordinary
// arrays keep their operators.
func isBareKeyPunctuation(b byte) bool {
	switch b {
	case '.', '-', ':', '@', '<', '>', '/':
		return true
	}
	return false
}

// associativeKey is one bare associative subscript located from a parser
// error: the offsets of its brackets and of every punctuation byte at the
// key's own level, outside any expansion nested inside the key.
type associativeKey struct {
	open        int
	close       int
	punctuation []int
}

func findBareAssociativeKey(src []byte, seed int, errorText string) (associativeKey, bool) {
	if seed < 0 || seed >= len(src) {
		return associativeKey{}, false
	}

	open := seed
	if errorText == invalidSubscriptExpression && src[open] != '[' {
		// In an assignment mvdan/sh positions this error one byte after the
		// name start instead of at `[`, so walk the rest of the name first.
		if seed < 1 || !isIdentByte(src[seed-1]) || (seed > 1 && isIdentByte(src[seed-2])) {
			return associativeKey{}, false
		}
		for open < len(src) && isIdentByte(src[open]) {
			open++
		}
		if open >= len(src) || src[open] != '[' {
			return associativeKey{}, false
		}
		seed = open
	}
	if src[open] != '[' {
		// Walk back to the subscript's own `[`, stepping over any expansion
		// closed before the seed such as `${MATCH%:}` or `${ICE[svn]}`.
		depth := 0
		for src[open] != '[' || depth > 0 {
			switch src[open] {
			case '}', ')':
				depth++
			case '{', '(':
				if depth == 0 {
					return associativeKey{}, false
				}
				depth--
			case ']':
				if depth == 0 {
					return associativeKey{}, false
				}
			case '\n':
				return associativeKey{}, false
			}
			if open == 0 {
				return associativeKey{}, false
			}
			open--
		}
	}
	if open == 0 || !isIdentByte(src[open-1]) {
		return associativeKey{}, false
	}

	key, ok := scanBareAssociativeKey(src, open)
	if !ok {
		return associativeKey{}, false
	}
	first := src[open+1]
	switch {
	case errorText == invalidSubscriptExpression:
		if first != '.' || seed != open {
			return associativeKey{}, false
		}
	case errorText == invalidSubscriptTernary:
		if src[seed] != ':' || !slices.Contains(key.punctuation, seed) {
			return associativeKey{}, false
		}
	case strings.HasPrefix(errorText, invalidSubscriptArithmetic):
		if first != '@' || key.close-key.open <= 2 {
			return associativeKey{}, false
		}
	default:
		op, ok := subscriptOperatorError(errorText)
		if !ok || src[seed] != op || !slices.Contains(key.punctuation, seed) {
			return associativeKey{}, false
		}
	}
	return key, true
}

// scanBareAssociativeKey reads the key that starts after the `[` at open. It
// records the punctuation bytes to mask and steps over `$name`, `${...}`, and
// `$(...)` without masking inside them. Any other byte, including quotes,
// backticks, whitespace, and special parameters, leaves the key unrecognized.
func scanBareAssociativeKey(src []byte, open int) (associativeKey, bool) {
	key := associativeKey{open: open}
	for offset := open + 1; offset < len(src); offset++ {
		b := src[offset]
		switch {
		case b == ']':
			if offset == open+1 {
				return associativeKey{}, false
			}
			key.close = offset
			return key, true
		case isIdentByte(b):
		case isBareKeyPunctuation(b):
			key.punctuation = append(key.punctuation, offset)
		case b == '$':
			end, ok := skipKeyExpansion(src, offset)
			if !ok {
				return associativeKey{}, false
			}
			offset = end
		default:
			return associativeKey{}, false
		}
	}
	return associativeKey{}, false
}

// skipKeyExpansion returns the offset of the last byte of the expansion that
// starts with the `$` at start: a bare name, a braced expansion, or a command
// or arithmetic substitution. Nested bodies must balance their own delimiters
// and stay on one line without quotes or backticks.
func skipKeyExpansion(src []byte, start int) (int, bool) {
	next := start + 1
	if next >= len(src) {
		return 0, false
	}
	var opener, closer byte
	switch {
	case isIdentByte(src[next]):
		for next < len(src) && isIdentByte(src[next]) {
			next++
		}
		return next - 1, true
	case src[next] == '{':
		opener, closer = '{', '}'
	case src[next] == '(':
		opener, closer = '(', ')'
	default:
		return 0, false
	}
	depth := 0
	for offset := next; offset < len(src); offset++ {
		switch src[offset] {
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return offset, true
			}
		case '\n', '\'', '"', '`':
			return 0, false
		}
	}
	return 0, false
}
