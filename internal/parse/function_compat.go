package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parseFunctionSemicolonBody adapts the native-Zsh function definition forms
// `function name; { list }` and `name ... () ; { list }`, where an optional
// separator precedes the body brace. The retry masks the semicolon with a
// same-width space and re-parses with unchanged byte positions.
//
// zshmisc spells the body as `word ... () [ term ] { list }`, so the separator
// is the same optional `term` in both spellings; only the head differs. The
// `()` head may carry more than one name, which composes with the multi-name
// adapter: this one removes the separator, that one splits the names.
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

// parenHeadAt reports whether a `name ... ()` definition head starts at off.
//
// The `()` spelling has no keyword to anchor on, so the head is confirmed by
// shape: one or more plain words, then `()`. A word carrying shell syntax is
// not a function name, which keeps `a | b () ...` and redirections out.
func parenHeadAt(src []byte, off int) bool {
	i := off
	names := 0
	for i < len(src) {
		for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
			i++
		}
		if i+1 < len(src) && src[i] == '(' && src[i+1] == ')' {
			return names > 0
		}
		start := i
		for i < len(src) && !isParenHeadDelimiter(src[i]) {
			i++
		}
		if i == start {
			return false
		}
		names++
	}
	return false
}

func isParenHeadDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '(', ')', '{', '}', '|', '&', '<', '>', '#', '`', '=':
		return true
	}
	return false
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

	if matchSourceWord(src, i, "function") {
		i += len("function")
		if i >= len(src) || (src[i] != ' ' && src[i] != '\t') {
			return 0, false
		}
	} else if !parenHeadAt(src, i) {
		// Not a `()` head at the command start either. Fall back to the old
		// search so a seed pointing past the keyword still finds it.
		idx := bytes.Index(src[seed:], []byte("function"))
		if idx < 0 {
			return 0, false
		}
		i = seed + idx + len("function")
		if i >= len(src) || (src[i] != ' ' && src[i] != '\t') {
			return 0, false
		}
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
