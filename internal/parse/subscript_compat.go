package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

const (
	invalidSubscriptExpression = "`[` must be followed by an expression"
	invalidSubscriptTernary    = "ternary operator missing `?` before `:`"
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
	if !errors.As(firstErr, &parseErr) ||
		(parseErr.Text != invalidSubscriptExpression && parseErr.Text != invalidSubscriptTernary) {
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

func findBareAssociativeKey(src []byte, seed int, errorText string) (int, int, bool) {
	if seed < 0 || seed >= len(src) {
		return 0, 0, false
	}

	open := seed
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
		if !isIdentByte(b) && b != '.' && b != '-' && b != ':' && b != '@' {
			return 0, 0, false
		}
	}

	switch errorText {
	case invalidSubscriptExpression:
		if key[0] != '.' || seed != open {
			return 0, 0, false
		}
	case invalidSubscriptTernary:
		if relativeSeed < 0 || relativeSeed >= len(key) || key[relativeSeed] != ':' {
			return 0, 0, false
		}
	}
	return open, close, true
}
