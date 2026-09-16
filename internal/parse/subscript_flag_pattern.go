package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	// mvdan/sh ends a flagged subscript pattern at the first `]`, so the
	// byte after a bracket expression's own `]` is reported as an operator
	// ("not a valid parameter expansion operator: `]`"), as a stray word
	// ("`line` cannot be followed by a word"), or, in an assignment, as a
	// missing `=`.
	invalidFlagPatternOperator = "not a valid parameter expansion operator:"
	invalidFlagPatternWord     = " cannot be followed by a word"
	invalidFlagPatternAssign   = "`a[b]` must be followed by `=`"
)

// parseSubscriptFlagBracketPattern retries only a flagged subscript
// `name[(flags)pattern]` whose pattern holds a bracket expression or an
// escaped bracket. mvdan/sh ends the raw pattern at the first `]`, while Zsh
// ends it at the `]` that balances the nesting. The retry masks every
// non-delimiting `[` and `]` (and every `,` inside a bracket expression) with
// `_` so the raw pattern literal reaches the real closing `]`, then restores
// the literal byte for byte.
func parseSubscriptFlagBracketPattern(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	seed := int(parseErr.Pos.Offset())
	if seed < 0 || seed >= len(src) {
		return nil, firstErr
	}

	var patternStart int
	var ok bool
	assign := false
	switch {
	case strings.HasPrefix(parseErr.Text, invalidFlagPatternOperator),
		strings.HasSuffix(parseErr.Text, invalidFlagPatternWord):
		patternStart, ok = flagPatternBeforePrematureClose(src, seed)
	case parseErr.Text == invalidFlagPatternAssign:
		patternStart, ok = flagPatternAfterAssignName(src, seed)
		assign = true
	default:
		return nil, firstErr
	}
	if !ok {
		return nil, firstErr
	}

	edits, close, ok := scanFlagPatternBrackets(src, patternStart)
	if !ok {
		return nil, firstErr
	}
	if assign {
		if close+1 >= len(src) || (src[close+1] != '=' && !bytes.HasPrefix(src[close+1:], []byte("+="))) {
			return nil, firstErr
		}
	} else if !editsContain(edits, seed-1) {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	for _, edit := range edits {
		masked[edit.offset] = edit.replacement
	}
	tree, err := parseWithAdapters(masked, name)
	if err != nil {
		return nil, err
	}
	if err := restorePatternEdits(tree, src, edits); err != nil {
		return nil, err
	}
	return tree, nil
}

// flagPatternBeforePrematureClose locates the flagged subscript whose pattern
// mvdan/sh cut at the `]` just before seed, and returns the pattern start.
func flagPatternBeforePrematureClose(src []byte, seed int) (int, bool) {
	if seed < 1 || src[seed-1] != ']' {
		return 0, false
	}
	prematureClose := seed - 1
	open := -1
	for i := prematureClose - 1; i > 0; i-- {
		if src[i] == '\n' || src[i] == ']' {
			return 0, false
		}
		if src[i] == '(' && src[i-1] == '[' {
			open = i - 1
			break
		}
	}
	if open < 1 || !isIdentByte(src[open-1]) {
		return 0, false
	}
	patternStart, ok := flagPatternStart(src, open)
	if !ok || patternStart > prematureClose {
		return 0, false
	}
	return patternStart, true
}

// flagPatternAfterAssignName locates the flagged subscript of the assignment
// whose name starts at seed and returns the pattern start.
func flagPatternAfterAssignName(src []byte, seed int) (int, bool) {
	open := seed
	for open < len(src) && isIdentByte(src[open]) {
		open++
	}
	if open == seed || open+1 >= len(src) || src[open] != '[' || src[open+1] != '(' {
		return 0, false
	}
	return flagPatternStart(src, open)
}

// flagPatternStart returns the offset after the `(flags)` group that opens at
// open+1. A flag letter may carry a delimited argument such as `n:2:`.
func flagPatternStart(src []byte, open int) (int, bool) {
	i := open + 2
	for i < len(src) {
		b := src[i]
		switch {
		case isFlagLetter(b):
			i++
		case i > open+2 && isFlagLetter(src[i-1]) && b != ')' && b != '\n' && !isFlagLetter(b):
			end := bytes.IndexByte(src[i+1:], b)
			if end < 0 {
				return 0, false
			}
			i += end + 2
		default:
			if i == open+2 || b != ')' {
				return 0, false
			}
			return i + 1, true
		}
	}
	return 0, false
}

func isFlagLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// scanFlagPatternBrackets walks a flagged subscript pattern from start to its
// real closing `]`, collecting the mask for every `[` and `]` that does not
// delimit the subscript and every `,` inside a bracket expression. Zsh
// delimits a subscript by bracket nesting with backslash escapes, so the
// scanner counts `[` and `]` and skips escaped bytes. It reports false when
// the pattern holds anything whose extent it cannot decide, such as a nested
// expansion or a command substitution in either form.
func scanFlagPatternBrackets(src []byte, start int) ([]patternEdit, int, bool) {
	var edits []patternEdit
	depth := 0
	mask := func(offset int) {
		edits = append(edits, patternEdit{offset: offset, original: src[offset], replacement: '_'})
	}
	for i := start; i < len(src); {
		b := src[i]
		switch {
		case b == '\\':
			if i+1 >= len(src) || src[i+1] == '\n' {
				return nil, 0, false
			}
			if src[i+1] == '[' || src[i+1] == ']' {
				mask(i + 1)
			}
			i += 2
		case b == '\n':
			return nil, 0, false
		case b == '`', b == '$' && i+1 < len(src) && (src[i+1] == '{' || src[i+1] == '('):
			return nil, 0, false
		case b == '[':
			depth++
			mask(i)
			i++
		case b == ']':
			if depth == 0 {
				if len(edits) == 0 {
					return nil, 0, false
				}
				return edits, i, true
			}
			depth--
			mask(i)
			i++
		case b == ',' || b == '}':
			if depth == 0 {
				return nil, 0, false
			}
			if b == ',' {
				mask(i)
			}
			i++
		default:
			i++
		}
	}
	return nil, 0, false
}

func editsContain(edits []patternEdit, offset int) bool {
	for _, edit := range edits {
		if edit.offset == offset {
			return true
		}
	}
	return false
}
