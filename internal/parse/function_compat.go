package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parseFunctionSemicolonBody adapts the native-Zsh function definition form
// `function name; { list }`, where an optional semicolon precedes the body
// brace. The retry masks the semicolon with a same-width space and re-parses
// with unchanged byte positions.
func parseFunctionSemicolonBody(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseFunctionSemicolonBodyWithParser(src, name, firstErr, parseWithAdapters)
}

func parseFunctionSemicolonBodyWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || !strings.Contains(parseErr.Text, "must be followed by a statement") {
		return nil, firstErr
	}

	semiOffset, ok := findFunctionSemicolonBody(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[semiOffset] = ' '

	return parse(masked, name)
}

func findFunctionSemicolonBody(src []byte, seed int) (int, bool) {
	if seed < 0 || seed >= len(src) {
		seed = 0
	}

	i := seed
	// Rewind to command start if seed is in the middle of the function declaration
	for i > 0 && src[i-1] != '\n' && src[i-1] != ';' {
		i--
	}
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}

	if !matchSourceWord(src, i, "function") {
		idx := bytes.Index(src[seed:], []byte("function"))
		if idx < 0 {
			return 0, false
		}
		i = seed + idx
	}

	i += len("function")
	if i >= len(src) || (src[i] != ' ' && src[i] != '\t') {
		return 0, false
	}

	hasName := false
	for i < len(src) {
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
		if i >= len(src) || src[i] == ';' || src[i] == '{' || src[i] == '(' || src[i] == '\n' {
			break
		}
		nameStart := i
		for i < len(src) && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' &&
			src[i] != ';' && src[i] != '(' && src[i] != ')' && src[i] != '{' && src[i] != '}' {
			i++
		}
		if i > nameStart {
			hasName = true
		}
	}
	if !hasName {
		return 0, false
	}

	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}

	// Optional ()
	if i+1 < len(src) && src[i] == '(' && src[i+1] == ')' {
		i += 2
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
	}

	// Semicolon before body
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n') {
		i++
	}
	if i >= len(src) || src[i] != ';' {
		return 0, false
	}
	semiOffset := i
	i++

	// After semicolon, must be followed by '{'
	i = skipSpacesAndComments(src, i)
	if i >= len(src) || src[i] != '{' {
		return 0, false
	}

	return semiOffset, true
}
