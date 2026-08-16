package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

func isIdentStartByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

// parseFdVarRedirect retries Zsh dynamic file-descriptor redirection of the
// form {varname}>... or {varname}>&- that mvdan/sh LangZsh rejects as a bash-only
// feature. The retry masks the {varname} delimiter with a same-width numeric file
// descriptor and restores the AST literal node on the resulting syntax.Redirect.
func parseFdVarRedirect(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseFdVarRedirectWithParser(src, name, firstErr, parseFdVarRedirects)
}

func parseFdVarRedirects(src []byte, name string) (*syntax.File, error) {
	tree, err := parseTree(src, name)
	if err != nil {
		return parseFdVarRedirectWithParser(src, name, err, parseFdVarRedirects)
	}
	return tree, nil
}

func parseFdVarRedirectWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var langErr syntax.LangError
	if !errors.As(firstErr, &langErr) || langErr.Feature != "`{varname}` redirects" || langErr.LangUsed != syntax.LangZsh {
		return nil, firstErr
	}

	seed := int(langErr.Pos.Offset())
	if seed < 0 || seed >= len(src) || src[seed] != '{' {
		return nil, firstErr
	}

	closeRel := bytes.IndexByte(src[seed:], '}')
	if closeRel < 2 {
		return nil, firstErr
	}
	closeOffset := seed + closeRel
	varname := src[seed+1 : closeOffset]
	if !isIdentStartByte(varname[0]) {
		return nil, firstErr
	}
	for i := 1; i < len(varname); i++ {
		if !isIdentByte(varname[i]) {
			return nil, firstErr
		}
	}

	width := closeOffset - seed + 1
	if closeOffset+1 >= len(src) {
		return nil, firstErr
	}
	next := src[closeOffset+1]
	if next != '>' && next != '<' {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	edits := make([]patternEdit, width)
	for i := 0; i < width; i++ {
		offset := seed + i
		edits[i] = patternEdit{
			offset:      offset,
			original:    src[offset],
			replacement: '9',
		}
		masked[offset] = '9'
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
