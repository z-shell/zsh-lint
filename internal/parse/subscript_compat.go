package parse

import (
	"bytes"
	"errors"
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

// parseAssociativeSubscript retries only native-Zsh bare associative keys that
// mvdan/sh mistakes for arithmetic syntax. The retry masks punctuation with
// same-width identifier bytes and restores every changed AST literal.
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

	open, close, ok := findBareAssociativeKey(src, int(parseErr.Pos.Offset()), parseErr.Text)
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	edits := make([]patternEdit, 0, close-open-1)
	for offset := open + 1; offset < close; offset++ {
		if isIdentByte(masked[offset]) {
			continue
		}
		edits = append(edits, patternEdit{offset: offset, original: masked[offset], replacement: '_'})
		masked[offset] = '_'
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

func findBareAssociativeKey(src []byte, seed int, errorText string) (int, int, bool) {
	if seed < 0 || seed >= len(src) {
		return 0, 0, false
	}

	open := seed
	if errorText == invalidSubscriptExpression && src[open] != '[' {
		// In an assignment mvdan/sh positions this error one byte after the
		// name start instead of at `[`, so walk the rest of the name first.
		if seed < 1 || !isIdentByte(src[seed-1]) || (seed > 1 && isIdentByte(src[seed-2])) {
			return 0, 0, false
		}
		for open < len(src) && isIdentByte(src[open]) {
			open++
		}
		if open >= len(src) || src[open] != '[' {
			return 0, 0, false
		}
		seed = open
	}
	if src[open] != '[' {
		for open > 0 && src[open] != '[' && src[open] != ']' && src[open] != '\n' {
			open--
		}
	}
	if src[open] != '[' || open == 0 || !isIdentByte(src[open-1]) {
		return 0, 0, false
	}

	relativeSeed := seed - open - 1
	closeRelative := bytes.IndexByte(src[open+1:], ']')
	if closeRelative <= 0 {
		return 0, 0, false
	}
	close := open + 1 + closeRelative
	key := src[open+1 : close]
	for _, b := range key {
		if !isIdentByte(b) && !isBareKeyPunctuation(b) {
			return 0, 0, false
		}
	}

	switch {
	case errorText == invalidSubscriptExpression:
		if key[0] != '.' || seed != open {
			return 0, 0, false
		}
	case errorText == invalidSubscriptTernary:
		if relativeSeed < 0 || relativeSeed >= len(key) || key[relativeSeed] != ':' {
			return 0, 0, false
		}
	case strings.HasPrefix(errorText, invalidSubscriptArithmetic):
		if key[0] != '@' || len(key) <= 1 {
			return 0, 0, false
		}
	default:
		op, ok := subscriptOperatorError(errorText)
		if !ok || relativeSeed < 0 || relativeSeed >= len(key) || key[relativeSeed] != op {
			return 0, 0, false
		}
	}
	return open, close, true
}
