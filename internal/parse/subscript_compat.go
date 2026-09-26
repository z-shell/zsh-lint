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
	// A doubled sign in a key, `g[${o}--$s]`, is read as a decrement or
	// increment. mvdan/sh reports the postfix form applied to something
	// that is not a name as "`--` must follow a name" and the prefix form
	// applied to an expansion as "`--` must be followed by a literal"
	// (issue #362). Both name the operator, so the gate can check that the
	// reported bytes are that operator inside the key.
	invalidSubscriptPostfixOperand = " must follow a name"
	invalidSubscriptPrefixOperand  = " must be followed by a literal"
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
	if _, ok := subscriptOperatorError(text); ok {
		return true
	}
	_, ok := subscriptDoubledSignError(text)
	return ok
}

// subscriptDoubledSignError returns the sign byte of the `--` or `++` that
// mvdan/sh reported as a decrement or increment with no name to apply to.
func subscriptDoubledSignError(text string) (byte, bool) {
	for _, op := range []string{"`--`", "`++`"} {
		suffix, ok := strings.CutPrefix(text, op)
		if ok && (suffix == invalidSubscriptPostfixOperand || suffix == invalidSubscriptPrefixOperand) {
			return op[1], true
		}
	}
	return 0, false
}

// isDoubledSignAt reports whether the two bytes at offset are one doubled
// sign, `--` or `++`, both masked as key punctuation.
func isDoubledSignAt(src []byte, key associativeKey, offset int) bool {
	if offset < 0 || offset+1 >= len(src) {
		return false
	}
	sign := src[offset]
	return (sign == '-' || sign == '+') && src[offset+1] == sign &&
		slices.Contains(key.punctuation, offset) && slices.Contains(key.punctuation, offset+1)
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
// arrays keep their operators; masking only ever happens after an error the
// gate ties to this key, so a subscript that parses keeps every operator.
func isBareKeyPunctuation(b byte) bool {
	switch b {
	case '.', '-', '+', ':', '@', '<', '>', '/':
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
	spaces      []int
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
		if first != '.' || seed != open || len(key.spaces) > 0 {
			return associativeKey{}, false
		}
	case errorText == invalidSubscriptTernary:
		if src[seed] != ':' || !slices.Contains(key.punctuation, seed) || len(key.spaces) > 0 {
			return associativeKey{}, false
		}
	case strings.HasPrefix(errorText, invalidSubscriptArithmetic):
		// `g[@x]`, or an operand straight after a postfix doubled sign,
		// `g[a--b]`: the decrement took `a`, and `b` has no operator.
		atKey := first == '@' && key.close-key.open > 2
		switch {
		case len(key.spaces) > 0:
			// `g[a b]`: a key word straight after another across blanks, where
			// arithmetic cannot continue (#372). Only this shape masks blanks;
			// every other key with a blank stays arithmetic, as `${map[$M :b]}`.
			if !isBareKeyWordAfterSpaceAt(src, key, seed) {
				return associativeKey{}, false
			}
			key.punctuation = append(key.punctuation, key.spaces...)
		case !atKey && !isDoubledSignAt(src, key, seed-2):
			return associativeKey{}, false
		}
	default:
		if sign, ok := subscriptDoubledSignError(errorText); ok {
			// The lexer read a doubled-sign token at seed, and the walk
			// back and the scan leave seed at depth 0 inside the key, so
			// src[seed] == sign and isDoubledSignAt both hold whenever the
			// key is recognized: dropping either check changed no verdict
			// over every key of up to four bytes from the key alphabet.
			// They stay as the statement of what the retry relies on.
			if src[seed] != sign || !isDoubledSignAt(src, key, seed) || len(key.spaces) > 0 {
				return associativeKey{}, false
			}
			break
		}
		op, ok := subscriptOperatorError(errorText)
		if !ok || src[seed] != op || !slices.Contains(key.punctuation, seed) || len(key.spaces) > 0 {
			return associativeKey{}, false
		}
	}
	return key, true
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t'
}

// isBareKeyWordAfterSpaceAt reports whether seed is the start of a bare-key
// word that follows another word across whitespace, where native Zsh reads the
// subscript as associative key text because arithmetic cannot continue. The
// caller guarantees key.open < seed <= key.close (findBareAssociativeKey walks
// back from seed to the key's `[` and refuses to cross a `]`) and a `[` at
// key.open, so src[key.close] is `]` and the walk back stops at the `[`.
func isBareKeyWordAfterSpaceAt(src []byte, key associativeKey, seed int) bool {
	if !isSpaceByte(src[seed-1]) || !isIdentByte(src[seed]) {
		return false
	}
	p := seed - 1
	for isSpaceByte(src[p]) {
		p--
	}
	return isIdentByte(src[p])
}

// isInsideBracedParamExp reports whether the subscript at open belongs to a
// braced parameter expansion `${...}`: the name before open is preceded only
// by `${`, optionally with the `+ ~ = # ^ !` prefixes and `(...)` flag groups.
// The caller guarantees open > 0 and a name byte at open-1.
func isInsideBracedParamExp(src []byte, open int) bool {
	p := open - 1
	for p >= 0 && isIdentByte(src[p]) {
		p--
	}
	for p >= 0 {
		switch src[p] {
		case '+', '~', '=', '#', '^', '!':
			p--
			continue
		case ')':
			// A flag group; its delimited arguments may hold any byte,
			// including a newline, as in `${(j:\n:)Z[a b]}`.
			depth := 1
			for p--; p >= 0 && depth > 0; p-- {
				switch src[p] {
				case ')':
					depth++
				case '(':
					depth--
				}
			}
			continue
		}
		break
	}
	return p >= 1 && src[p-1] == '$' && src[p] == '{'
}

// scanBareAssociativeKey reads the key that starts after the `[` at open. It
// records the punctuation and space bytes to mask and steps over `$name`,
// `${...}`, and `$(...)` without masking inside them. Any other byte,
// including quotes, backticks, and special parameters, leaves the key
// unrecognized. Whitespace is permitted only inside a braced parameter
// expansion.
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
		case isSpaceByte(b):
			if !isInsideBracedParamExp(src, open) {
				return associativeKey{}, false
			}
			key.spaces = append(key.spaces, offset)
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
