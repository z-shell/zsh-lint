package parse

import (
	"bytes"
	"errors"

	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// mvdan/sh reads a flagged subscript pattern as raw text only up to the first
// `,`; the expression after it is ordinary arithmetic, where a bracket
// expression, a leading `--`, or a `^` is an operator error (issue #277). The
// three texts are the ones the corpus and the minimized rows raise.
var patternAfterCommaErrors = []string{
	"`[` must follow a name like a[i]",
	"`--` must be followed by a literal",
	"`^` must follow an expression",
}

// parseSubscriptPatternAfterComma retries only a flagged subscript
// `name[(flags)pattern,expression]` whose expression after the `,` fails as
// arithmetic. Native Zsh reads that expression by the parameter's type (a
// pattern for an associative array, arithmetic or a flagged pattern for a
// plain array), which the parser cannot know, so the retry keeps the shape the
// parser already gives `${m[(r)a,b]}`: a `BinaryArithm` `,` whose right operand
// is one `Word`. The retry masks every byte of the expression with `_` so the
// parser reads it as one literal, then restores the bytes. Like the flagged
// pattern before the `,`, the literal keeps a `$name` reference as raw text.
func parseSubscriptPatternAfterComma(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	if !patternAfterCommaError(parseErr.Text) {
		return nil, firstErr
	}
	seed := int(parseErr.Pos.Offset())
	if seed < 0 || seed >= len(src) {
		return nil, firstErr
	}
	site, ok := locatePatternAfterComma(src, seed)
	if !ok {
		site, ok = locateLaterSubscriptAfterFlags(src, seed)
	}
	if !ok {
		return nil, firstErr
	}
	masked := bytes.Clone(src)
	for _, edit := range site.edits {
		masked[edit.offset] = edit.replacement
	}
	tree, err := parseWithAdapters(masked, name)
	if err != nil {
		return nil, err
	}
	if !site.verify(tree) {
		return nil, firstErr
	}
	if err := restorePatternEdits(tree, src, site.edits); err != nil {
		return nil, err
	}
	return tree, nil
}

func patternAfterCommaError(text string) bool {
	for _, want := range patternAfterCommaErrors {
		if text == want {
			return true
		}
	}
	return false
}

// patternAfterCommaSite is one `name[(flags)pattern,expression]` subscript:
// the offset of its `,`, the span of the expression after it, and the mask
// that turns that expression into one literal. For a later subscript after a
// flagged one, `name[(flags)pattern][expression]`, later is set and comma is
// the offset of the later subscript's `[`.
type patternAfterCommaSite struct {
	comma int
	start int
	end   int
	edits []patternEdit
	later bool
}

// locatePatternAfterComma locates the flagged subscript whose expression after
// the `,` holds the error at seed. It walks back from seed to each `name[(`
// opener on the same line and keeps the first whose expression span contains
// seed, so a subscript nested inside another one's expression is not confused
// with the outer subscript.
func locatePatternAfterComma(src []byte, seed int) (patternAfterCommaSite, bool) {
	for i := seed - 1; i > 0; i-- {
		if src[i] == '\n' {
			return patternAfterCommaSite{}, false
		}
		if src[i] != '(' || src[i-1] != '[' || i < 2 || !isIdentByte(src[i-2]) {
			continue
		}
		open := i - 1
		patternStart, ok := flagPatternStart(src, open)
		if !ok {
			continue
		}
		comma, ok := flagPatternComma(src, patternStart)
		if !ok || comma >= seed {
			continue
		}
		site, ok := scanPatternAfterComma(src, comma)
		if !ok || seed > site.end {
			continue
		}
		return site, true
	}
	return patternAfterCommaSite{}, false
}

// locateLaterSubscriptAfterFlags locates the subscript written directly
// after a flagged one, `name[(flags)pattern][expression]`, whose expression
// holds the error at seed (#252). Zsh applies it to the value the flagged
// subscript returns, so it reads the expression by that value's type, which
// the parser cannot know, as it cannot for the expression after a flagged
// pattern's `,`; the same literal reading applies. Before later subscripts
// became typed children, the second-subscript mask rewrote these sources into
// that comma form.
func locateLaterSubscriptAfterFlags(src []byte, seed int) (patternAfterCommaSite, bool) {
	for i := seed - 1; i > 0; i-- {
		if src[i] == '\n' {
			return patternAfterCommaSite{}, false
		}
		if src[i] != '(' || src[i-1] != '[' || i < 2 || !isIdentByte(src[i-2]) {
			continue
		}
		patternStart, ok := flagPatternStart(src, i-1)
		if !ok {
			continue
		}
		end, closer, ok := flagPatternEnd(src, patternStart)
		if !ok || closer != ']' || end+1 >= seed || src[end+1] != '[' {
			continue
		}
		site, ok := scanPatternAfterComma(src, end+1)
		if !ok || seed > site.end {
			continue
		}
		site.later = true
		return site, true
	}
	return patternAfterCommaSite{}, false
}

// flagPatternComma walks a flagged pattern from start to the `,` that ends
// it, counting bracket expressions the way scanFlagPatternBrackets does. It
// reports false when the pattern ends at `]` without a `,`, or holds anything
// whose extent the scanner cannot decide.
func flagPatternComma(src []byte, start int) (int, bool) {
	end, closer, ok := flagPatternEnd(src, start)
	return end, ok && closer == ','
}

// flagPatternEnd walks a flagged pattern from start to the `,` or `]` that
// ends it, as flagPatternComma describes, and returns its offset and byte.
func flagPatternEnd(src []byte, start int) (int, byte, bool) {
	depth := 0
	for i := start; i < len(src); {
		b := src[i]
		switch {
		case b == '\\':
			if i+1 >= len(src) || src[i+1] == '\n' {
				return 0, 0, false
			}
			i += 2
		case b == '\n', b == '`', b == '}' && depth == 0:
			return 0, 0, false
		case b == '$' && i+1 < len(src) && (src[i+1] == '{' || src[i+1] == '('):
			return 0, 0, false
		case b == '[':
			depth++
			i++
		case b == ']':
			if depth == 0 {
				return i, b, true
			}
			depth--
			i++
		case b == ',' && depth == 0:
			return i, b, true
		default:
			i++
		}
	}
	return 0, 0, false
}

// scanPatternAfterComma walks the expression after the `,` at comma to the `]`
// that closes the subscript, masking every byte of the expression with `_`.
// Bracket expressions nest by counting, and a quoted string (`'...'`, `"..."`,
// `$'...'`) is masked whole, so a `,` inside it does not end the scan. A
// backslash escapes the next byte, except that inside `'...'` the lexer keeps
// it literal and only the subscript reader honours `\]` and `\\`. Native Zsh
// counts `[` and `]` inside a quoted string too, so a `]` at depth 0 inside one
// ends the subscript before the quote closes (`bad substitution`) and the scan
// reports false, as it does for an unterminated quoted string, an empty
// expression, a second `,` outside a bracket expression or a quoted string, a
// `}`, a newline, a backtick, and a nested expansion or command substitution,
// whose extent it does not decide.
func scanPatternAfterComma(src []byte, comma int) (patternAfterCommaSite, bool) {
	site := patternAfterCommaSite{comma: comma, start: comma + 1}
	depth := 0
	var quote byte  // the byte that closes the open quoted string, or 0
	escapes := true // whether a backslash escapes the next byte here
	dollar := false // whether the previous byte is an unescaped `$`
	mask := func(offset int) {
		if src[offset] != '_' {
			site.edits = append(site.edits, patternEdit{offset: offset, original: src[offset], replacement: '_'})
		}
	}
	for i := site.start; i < len(src); {
		b := src[i]
		afterDollar := dollar
		dollar = false
		switch {
		case b == '\\' && (escapes || i+1 < len(src) && (src[i+1] == ']' || src[i+1] == '\\')):
			if i+1 >= len(src) || src[i+1] == '\n' {
				return patternAfterCommaSite{}, false
			}
			mask(i)
			mask(i + 1)
			i += 2
		case b == '\n', b == '`', b == '}' && depth == 0, b == ',' && depth == 0 && quote == 0:
			return patternAfterCommaSite{}, false
		case b == '$' && i+1 < len(src) && (src[i+1] == '{' || src[i+1] == '('):
			return patternAfterCommaSite{}, false
		case quote != 0 && b == quote:
			quote = 0
			escapes = true
			mask(i)
			i++
		case quote == 0 && (b == '\'' || b == '"'):
			quote = b
			escapes = b == '"' || afterDollar
			mask(i)
			i++
		case b == '[':
			depth++
			mask(i)
			i++
		case b == ']':
			if depth == 0 {
				if quote != 0 || i == site.start {
					return patternAfterCommaSite{}, false
				}
				site.end = i
				return site, true
			}
			depth--
			mask(i)
			i++
		default:
			dollar = b == '$'
			mask(i)
			i++
		}
	}
	return patternAfterCommaSite{}, false
}

// verify reports whether the retry read the site as the expected shape: a
// `BinaryArithm` `,` at the site's comma whose left operand is the flagged
// pattern and whose right operand is one literal spanning exactly the masked
// expression.
func (site patternAfterCommaSite) verify(tree *syntax.File) bool {
	if site.later {
		return site.verifyLater(tree)
	}
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		binary, ok := node.(*syntax.BinaryArithm)
		if !ok || binary.Op != syntax.Comma || int(binary.OpPos.Offset()) != site.comma {
			return true
		}
		if _, ok := binary.X.(*syntax.FlagsArithm); !ok {
			return true
		}
		word, ok := binary.Y.(*syntax.Word)
		if !ok || len(word.Parts) != 1 {
			return true
		}
		lit, ok := word.Parts[0].(*syntax.Lit)
		if !ok {
			return true
		}
		found = int(lit.ValuePos.Offset()) == site.start && int(lit.ValueEnd.Offset()) == site.end
		return !found
	})
	return found
}

// verifyLater reports whether the retry read a later-subscript site as one
// literal spanning exactly the masked expression, held as the expansion's
// later subscript directly after a flagged one.
func (site patternAfterCommaSite) verifyLater(tree *syntax.File) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		exp, ok := node.(*syntax.ParamExp)
		if !ok {
			return true
		}
		previous := exp.Index
		for _, index := range exp.LaterIndexes {
			if _, flagged := previous.(*syntax.FlagsArithm); flagged && literalSpans(index, site.start, site.end) {
				found = true
				return false
			}
			previous = index
		}
		return true
	})
	return found
}

// literalSpans reports whether expr is a word of one literal whose value
// spans exactly start to end.
func literalSpans(expr syntax.ArithmExpr, start, end int) bool {
	word, ok := expr.(*syntax.Word)
	if !ok || len(word.Parts) != 1 {
		return false
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	return ok && int(lit.ValuePos.Offset()) == start && int(lit.ValueEnd.Offset()) == end
}
