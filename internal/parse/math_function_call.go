package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

// invalidMathFunctionCall is the exact error mvdan/sh (LangZsh, v3.14.1)
// reports for a math function call in an arithmetic expression,
// `$(( sqrt(4) ))`: it reads the identifier as a complete operand and then
// finds `(` where an operator would be.
const invalidMathFunctionCall = "not a valid arithmetic operator: `(`"

// MathFunctionCall pairs a math function call's name with the arguments
// written inside its parentheses (zshmisc, Arithmetic Evaluation: "It is also
// possible to use the function call syntax `func(args)`"). mvdan/sh (through
// v3.14.1) has no call node in its arithmetic grammar, so the compatibility
// front end retains the call here as typed arithmetic nodes with their
// original source positions, in source order.
//
// Args is empty for a call written with no arguments, `rand48()`.
type MathFunctionCall struct {
	Name *syntax.Lit
	Args []syntax.ArithmExpr
}

// parseMathFunctionCall retries only a native-Zsh math function call in an
// arithmetic expression. The retry blanks each call's name and keeps its
// parentheses, so `sqrt(4)` is read as the parenthesized group `(4)`: a
// self-contained operand that binds exactly where the call did, leaving the
// surrounding expression's precedence and associativity untouched. A call's
// arguments stay inside those parentheses, so a multi-argument call becomes
// the comma expression the parser already understands.
//
// Masking the parentheses instead would re-associate the expression. Turning
// `sqrt(4)` into the comma expression `sqrt,4` parses, but because a comma
// binds loosest of all arithmetic operators it captures the neighbours too:
// `1 + sqrt(4)` becomes `(1 + sqrt) , 4`, which reads the name as a variable
// and moves the argument out of the call.
//
// A call written with no arguments has nothing to keep between its
// parentheses, and an empty group is not an expression, so the name and its
// `()` are replaced by a literal `0` padded to the original width.
//
// Every call site is resolved in one pass: masking one site per retry would be
// quadratic in the number of calls per file.
//
// The masked bytes are not literal text, so nothing is restored here: Parse
// rebuilds the calls from the original source bytes once the chain has
// succeeded (see bindMathFunctionCalls).
func parseMathFunctionCall(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidMathFunctionCall {
		return nil, firstErr
	}

	sites := findMathFunctionCalls(src)
	if len(sites) == 0 {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	for _, site := range sites {
		maskMathCall(masked, site)
	}

	return parseWithAdapters(masked, name)
}

// maskMathCall blanks one call in place, keeping every byte offset.
func maskMathCall(masked []byte, site mathCallSite) {
	for i := site.nameStart; i < site.open; i++ {
		masked[i] = ' '
	}
	if !site.emptyArgs {
		return
	}
	// `name()` has no operand to keep, so the whole call becomes `0`.
	for i := site.open; i <= site.close; i++ {
		masked[i] = ' '
	}
	masked[site.close] = '0'
}

// bindMathFunctionCalls pairs each masked call site with the parenthesized
// group the retry left in its place, recovering the function name from the
// original source bytes.
//
// The adapter blanks a call's name and keeps its parentheses, so the group's
// own position identifies the call: the name is the identifier immediately
// before the group's `(` in the original source. A call written with no
// arguments has no group, so its name is read back the same way from the
// literal `0` that replaced it.
func bindMathFunctionCalls(tree *syntax.File, src []byte) []MathFunctionCall {
	sites := findMathFunctionCalls(src)
	if len(sites) == 0 {
		return nil
	}
	byOpen := make(map[int]mathCallSite, len(sites))
	for _, site := range sites {
		byOpen[site.open] = site
	}

	found := make([]MathFunctionCall, 0, len(sites))
	syntax.Walk(tree, func(node syntax.Node) bool {
		switch expr := node.(type) {
		case *syntax.ParenArithm:
			site, ok := byOpen[int(expr.Lparen.Offset())]
			if !ok || site.emptyArgs {
				return true
			}
			found = append(found, MathFunctionCall{
				Name: mathCallName(src, site),
				Args: flattenCommaArgs(expr.X),
			})
		case *syntax.Word:
			// An empty-argument call was replaced by `0` at the `)`.
			site, ok := byOpen[int(expr.Pos().Offset())-1]
			if !ok || !site.emptyArgs || expr.Lit() != "0" {
				return true
			}
			found = append(found, MathFunctionCall{Name: mathCallName(src, site)})
		}
		return true
	})
	return found
}

// mathCallName rebuilds the call's name literal from the original source.
func mathCallName(src []byte, site mathCallSite) *syntax.Lit {
	return &syntax.Lit{
		ValuePos: mathCallPos(site.nameStart),
		ValueEnd: mathCallPos(site.open),
		Value:    string(src[site.nameStart:site.open]),
	}
}

// mathCallPos builds a position carrying the byte offset, which is what
// consumers of this metadata read.
func mathCallPos(offset int) syntax.Pos {
	return syntax.NewPos(uint(offset), 0, 0)
}

// flattenCommaArgs splits a call's argument group on its top-level commas.
// A single argument has no comma and is returned as the only element.
func flattenCommaArgs(expr syntax.ArithmExpr) []syntax.ArithmExpr {
	binary, ok := expr.(*syntax.BinaryArithm)
	if !ok || binary.Op != syntax.Comma {
		return []syntax.ArithmExpr{expr}
	}
	return append(flattenCommaArgs(binary.X), flattenCommaArgs(binary.Y)...)
}

// mathCallSite is one `name(args)` call, as offsets into the original source.
type mathCallSite struct {
	nameStart int
	open      int
	close     int
	emptyArgs bool
}

// findMathFunctionCalls reports every math function call inside an arithmetic
// context, in source order.
//
// Only `$(( ... ))` and `(( ... ))` are scanned. A `(` elsewhere opens a
// subshell, an array literal or a glob qualifier, and masking one would change
// what the source means rather than let it parse.
func findMathFunctionCalls(src []byte) []mathCallSite {
	var sites []mathCallSite
	for _, span := range arithmeticSpans(src) {
		sites = append(sites, mathCallsInSpan(src, span)...)
	}
	return sites
}

// arithmeticSpan is the [start, end) byte range inside one arithmetic
// expression, excluding its own delimiters.
type arithmeticSpan struct {
	start int
	end   int
}

// arithmeticSpans reports each arithmetic expression's interior, in source
// order, for both the `$(( ))` expansion and the `(( ))` command.
func arithmeticSpans(src []byte) []arithmeticSpan {
	var spans []arithmeticSpan
	for i := 0; i+1 < len(src); i++ {
		if src[i] != '(' || src[i+1] != '(' {
			continue
		}
		// `$((` is the expansion; a bare `((` is the command. Both open an
		// arithmetic context, and `$(` followed by a subshell `(` does not.
		if i > 0 && src[i-1] == '$' {
			if end, ok := arithmeticEnd(src, i+2); ok {
				spans = append(spans, arithmeticSpan{start: i + 2, end: end})
				i = end
			}
			continue
		}
		if i > 0 && src[i-1] == '(' {
			continue
		}
		if end, ok := arithmeticEnd(src, i+2); ok {
			spans = append(spans, arithmeticSpan{start: i + 2, end: end})
			i = end
		}
	}
	return spans
}

// arithmeticEnd reports the offset of the `))` that closes an arithmetic
// expression opened just before from, tracking nested parentheses so a call's
// own parentheses do not end the scan.
func arithmeticEnd(src []byte, from int) (int, bool) {
	depth := 0
	for i := from; i < len(src); i++ {
		switch src[i] {
		case '\'', '"':
			if end, ok := skipQuoted(src, i); ok {
				i = end
				continue
			}
			return 0, false
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
				continue
			}
			if i+1 < len(src) && src[i+1] == ')' {
				return i, true
			}
			return 0, false
		}
	}
	return 0, false
}

// skipQuoted reports the offset of the quote closing the one at start.
func skipQuoted(src []byte, start int) (int, bool) {
	quote := src[start]
	for i := start + 1; i < len(src); i++ {
		if src[i] == '\\' && quote == '"' {
			i++
			continue
		}
		if src[i] == quote {
			return i, true
		}
	}
	return 0, false
}

// mathCallsInSpan reports the calls written directly inside one arithmetic
// expression, in source order.
func mathCallsInSpan(src []byte, span arithmeticSpan) []mathCallSite {
	var sites []mathCallSite
	for i := span.start; i < span.end; i++ {
		if src[i] != '(' {
			continue
		}
		nameStart, ok := mathFunctionNameStart(src, span.start, i)
		if !ok {
			continue
		}
		close, ok := matchingParen(src, i, span.end)
		if !ok {
			continue
		}
		sites = append(sites, mathCallSite{
			nameStart: nameStart,
			open:      i,
			close:     close,
			emptyArgs: blankRange(src, i+1, close),
		})
	}
	return sites
}

// mathFunctionNameStart reports where the identifier immediately before open
// begins, or false when the `(` does not follow one.
//
// A call's name is a plain identifier written with no blank before the `(`;
// anything else is a grouping parenthesis, which must keep its meaning.
func mathFunctionNameStart(src []byte, from, open int) (int, bool) {
	if open <= from {
		return 0, false
	}
	end := open
	start := end
	for start > from && isMathNameByte(src[start-1]) {
		start--
	}
	if start == end || isMathDigit(src[start]) {
		return 0, false
	}
	return start, true
}

// matchingParen reports the offset of the `)` closing the `(` at open.
func matchingParen(src []byte, open, limit int) (int, bool) {
	depth := 0
	for i := open; i < limit; i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// blankRange reports whether [start, end) holds only blanks.
func blankRange(src []byte, start, end int) bool {
	for i := start; i < end; i++ {
		if src[i] != ' ' && src[i] != '\t' {
			return false
		}
	}
	return true
}

func isMathNameByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		isMathDigit(b)
}

func isMathDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
