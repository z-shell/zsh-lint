package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

const (
	invalidReverseGlob     = "`*` must follow an expression"
	invalidParamGlobToggle = "not a valid parameter expansion operator: `~`"
)

func parseParamGlobToggle(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseParamGlobToggleWithParser(src, name, firstErr, parseParamGlobToggles)
}

func parseParamGlobToggles(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		return parseParamGlobToggleWithParser(src, name, err, parseParamGlobToggles)
	}
	return tree, nil
}

func parseParamGlobToggleWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidParamGlobToggle {
		return nil, firstErr
	}
	seed := int(parseErr.Pos.Offset())
	if seed < 0 || seed >= len(src) || src[seed] != '~' {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[seed] = 'x'
	edit := patternEdit{offset: seed, original: '~', replacement: 'x'}
	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if err := restorePatternEdits(tree, src, []patternEdit{edit}); err != nil {
		return nil, err
	}
	return tree, nil
}

func parseRCExpandCaret(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseRCExpandCaretWithParser(src, name, firstErr, parseRCExpandCarets)
}

func parseRCExpandCarets(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		return parseRCExpandCaretWithParser(src, name, err, parseRCExpandCarets)
	}
	return tree, nil
}

func parseRCExpandCaretWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var langErr syntax.LangError
	if !errors.As(firstErr, &langErr) || langErr.Feature != "this expansion operator" || langErr.LangUsed != syntax.LangZsh {
		return nil, firstErr
	}
	seed := int(langErr.Pos.Offset())
	offset := -1
	for candidate := seed - 1; candidate <= seed+1; candidate++ {
		if candidate >= 2 && candidate < len(src) && src[candidate] == '^' && src[candidate-1] == '{' && src[candidate-2] == '$' {
			offset = candidate
			break
		}
	}
	if offset < 0 {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[offset] = 'x'
	edit := patternEdit{offset: offset, original: '^', replacement: 'x'}
	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if err := restorePatternEdits(tree, src, []patternEdit{edit}); err != nil {
		return nil, err
	}
	return tree, nil
}

func parseReverseSubscript(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseReverseSubscriptWithParser(src, name, firstErr, parseReverseSubscripts)
}

func parseReverseSubscripts(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		return parseReverseSubscriptWithParser(src, name, err, parseReverseSubscripts)
	}
	return tree, nil
}

func parseReverseSubscriptWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidReverseGlob {
		return nil, firstErr
	}
	seed := int(parseErr.Pos.Offset())
	if seed < 0 || seed >= len(src) || src[seed] != '*' {
		return nil, firstErr
	}
	open := seed
	for open > 0 && src[open] != '[' && src[open] != ']' && src[open] != '\n' {
		open--
	}
	closeRelative := bytes.IndexByte(src[seed:], ']')
	if src[open] != '[' || closeRelative < 1 {
		return nil, firstErr
	}
	close := seed + closeRelative
	key := src[open+1 : close]
	if len(key) <= 3 || key[0] != '(' || key[2] != ')' || !bytes.ContainsRune([]byte("iIrR"), rune(key[1])) {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	var edits []patternEdit
	for offset := open + 4; offset < close; offset++ {
		if isIdentByte(masked[offset]) {
			continue
		}
		if masked[offset] != '-' && masked[offset] != '*' && masked[offset] != '?' {
			return nil, firstErr
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
