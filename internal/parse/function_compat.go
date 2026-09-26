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

	// A non-brace body needs the `()` spelling. The upstream parser accepts
	// `function a() print z` but not `function a  print z`, even though Zsh
	// accepts both, so the keyword form is rewritten to carry `()` (#346).
	// The two bytes come from the separator plus the blank after it, which
	// keeps every later position unchanged.
	//
	// The brace test is an optimisation, not a guard: a brace body already
	// parses, and rewriting one anyway produces `function a(){ list }`, which
	// also parses and keeps every position.
	if body := skipSpacesAndComments(masked, semiOffset+1); body < len(masked) && masked[body] != '{' {
		if nameEnd := functionKeywordNameEnd(src, semiOffset); nameEnd > 0 {
			masked[nameEnd] = '('
			masked[nameEnd+1] = ')'
		}
	}

	return parse(masked, name)
}

// functionKeywordNameEnd reports where to write the synthetic `()` for a
// `function name;` head, or 0 when the head is not that shape.
//
// The `()` needs two bytes and must sit directly after the name, so they are
// taken from the separator plus the blank that follows it. Zsh needs no blank
// between `()` and the body, so `function a; print z` becomes
// `function a()print z` with every later byte at its original offset. A head
// that already carries `()` keeps the plain mask.
func functionKeywordNameEnd(src []byte, semiOffset int) int {
	if semiOffset+1 >= len(src) || !isFunctionHeadSpace(src[semiOffset+1]) {
		return 0
	}
	nameEnd := semiOffset
	for nameEnd > 0 && isFunctionHeadSpace(src[nameEnd-1]) {
		nameEnd--
	}
	nameStart := nameEnd
	for nameStart > 0 && !isFunctionHeadSpace(src[nameStart-1]) {
		nameStart--
	}
	keyword := nameStart
	for keyword > 0 && isFunctionHeadSpace(src[keyword-1]) {
		keyword--
	}
	keywordStart := keyword
	for keywordStart > 0 && !isFunctionHeadSpace(src[keywordStart-1]) {
		keywordStart--
	}
	if !matchSourceWord(src, keywordStart, "function") {
		return 0
	}
	// The `()` goes where the name ends. For `function a; body` that is the
	// separator itself; a head with a blank before the separator spends the
	// blank instead, which keeps the two bytes adjacent to the name either way.
	return nameEnd
}

func isFunctionHeadSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n'
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
		// A `)` or `}` ends the head without being a separator, as in
		// `(function a)`. The scan consumed nothing, so stop: the head has
		// no `;` before a body, and looping again would never advance (#480).
		if i == nameStart {
			break
		}
		hasName = true
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

	// After the separator a body must follow. zshmisc spells the body as a
	// `list`, and a brace group is only one way to write one: `a () ; print z`
	// defines `a` with `print z` as its body (#346). So anything that can open
	// a statement is accepted, and only what cannot is refused.
	i = skipSpacesAndComments(src, i)
	if !opensFunctionBody(src, i) {
		return 0, false
	}

	return semiOffset, true
}

// opensFunctionBody reports whether a function body can start at off.
//
// Native Zsh reads the body as one `list`, so the test is whether a statement
// can begin here at all. End of input is not a body, and neither is a further
// separator: `a () ;` and `a () ; ;` are both rejected by Zsh. A reserved word
// that only ever *closes* or *continues* an enclosing construct cannot open one
// either, so it is refused rather than masked into a shape that would parse.
func opensFunctionBody(src []byte, off int) bool {
	if off >= len(src) {
		return false
	}
	switch src[off] {
	case ';', '&', '|', ')', '}':
		return false
	}
	for _, word := range functionBodyNeverOpens {
		if matchSourceWord(src, off, word) {
			return false
		}
	}
	return true
}

// functionBodyNeverOpens are the reserved words that close or continue a
// construct, so none of them can be the first word of a body.
var functionBodyNeverOpens = []string{
	"then", "else", "elif", "fi", "do", "done", "esac", "always",
}
