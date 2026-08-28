package parse

import (
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

const tryAlwaysParseError = "statements must be separated by &, ; or a newline"

// parseTryAlways retries the primary parser only for the documented Zsh
// try/always boundary. The fixed-width transformation keeps every original
// byte offset valid for the returned syntax tree.
func parseTryAlways(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseTryAlwaysWithParser(src, name, firstErr, parseWithAdapters)
}

func parseTryAlwaysWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != tryAlwaysParseError {
		return nil, firstErr
	}

	edits, ok := scanTryAlwaysEdits(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}

	transformed := applyTryAlwaysEdits(src, edits)
	if len(transformed) != len(src) {
		return nil, firstErr
	}
	return parse(transformed, name)
}

type tryAlwaysEdit struct {
	separator  int
	alwaysFrom int
	alwaysTo   int
}

// scanTryAlwaysEdits recognizes only `}` followed by horizontal whitespace,
// `always`, horizontal whitespace, and `{` in syntactically active source.
// Newlines and semicolons are intentionally excluded because native Zsh
// rejects them between the try block and the always keyword.
func scanTryAlwaysEdits(src []byte, seedOffset int) ([]tryAlwaysEdit, bool) {
	var edits []tryAlwaysEdit
	seedRecognized := false
	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false
	var heredocs []pendingHeredoc
	arithmeticDepth := 0

	for i := 0; i < len(src); i++ {
		b := src[i]
		if escaped {
			escaped = false
			continue
		}
		if inSingleQuote {
			if b == '\'' {
				inSingleQuote = false
			}
			continue
		}
		if inDoubleQuote {
			switch b {
			case '\\':
				escaped = true
			case '"':
				inDoubleQuote = false
			}
			continue
		}
		if inANSICQuote {
			switch b {
			case '\\':
				escaped = true
			case '\'':
				inANSICQuote = false
			}
			continue
		}
		if inComment {
			if b == '\n' {
				inComment = false
			}
			continue
		}
		if b == '\n' && len(heredocs) > 0 {
			next, ok := consumeHeredocBodies(src, i+1, heredocs)
			if !ok {
				return nil, false
			}
			heredocs = nil
			i = next - 1
			continue
		}
		if b == '(' && i+1 < len(src) && src[i+1] == '(' {
			arithmeticDepth++
			i++
			continue
		}
		if arithmeticDepth > 0 && b == ')' && i+1 < len(src) && src[i+1] == ')' {
			arithmeticDepth--
			i++
			continue
		}

		switch b {
		case '\\':
			escaped = true
			continue
		case '\'':
			inSingleQuote = true
			continue
		case '"':
			inDoubleQuote = true
			continue
		case '#':
			if i == 0 || isTryAlwaysBoundary(src[i-1]) {
				inComment = true
			}
			continue
		case '$':
			if i+1 < len(src) && src[i+1] == '\'' {
				inANSICQuote = true
				i++
				continue
			}
		}
		if arithmeticDepth == 0 && b == '<' && i+1 < len(src) && src[i+1] == '<' {
			stripTabs := i+2 < len(src) && src[i+2] == '-'
			delimiterStart := i + 2
			if stripTabs {
				delimiterStart++
			}
			delimiter, end, ok := parseHeredocDelimiter(src, delimiterStart)
			if !ok {
				return nil, false
			}
			heredocs = append(heredocs, pendingHeredoc{
				delimiter: delimiter,
				stripTabs: stripTabs,
			})
			i = end - 1
			continue
		}

		if b != '}' {
			continue
		}
		separator := i + 1
		if separator >= len(src) || !isTryAlwaysSpace(src[separator]) {
			continue
		}
		alwaysFrom := separator
		for alwaysFrom < len(src) && isTryAlwaysSpace(src[alwaysFrom]) {
			alwaysFrom++
		}
		alwaysTo := alwaysFrom + len("always")
		if alwaysTo > len(src) || string(src[alwaysFrom:alwaysTo]) != "always" {
			continue
		}
		if alwaysTo < len(src) && isIdentByte(src[alwaysTo]) {
			continue
		}
		openBrace := alwaysTo
		if openBrace >= len(src) || !isTryAlwaysSpace(src[openBrace]) {
			continue
		}
		for openBrace < len(src) && isTryAlwaysSpace(src[openBrace]) {
			openBrace++
		}
		if openBrace >= len(src) || src[openBrace] != '{' {
			continue
		}

		edits = append(edits, tryAlwaysEdit{
			separator:  separator,
			alwaysFrom: alwaysFrom,
			alwaysTo:   alwaysTo,
		})
		if seedOffset >= i && seedOffset <= openBrace {
			seedRecognized = true
		}
	}

	return edits, seedRecognized && len(edits) > 0
}

func isTryAlwaysSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

func isTryAlwaysBoundary(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == ';' || b == '{' || b == '}'
}

func applyTryAlwaysEdits(src []byte, edits []tryAlwaysEdit) []byte {
	transformed := append([]byte(nil), src...)
	for _, edit := range edits {
		transformed[edit.separator] = ';'
		for i := edit.alwaysFrom; i < edit.alwaysTo; i++ {
			transformed[i] = ' '
		}
	}
	return transformed
}
