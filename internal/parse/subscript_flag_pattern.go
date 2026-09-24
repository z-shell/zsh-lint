package parse

import (
	"bytes"
	"errors"
	"sort"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/syntax"
)

// flagPatternCutPasses counts resolveFlagPatternCuts's passes, so a test can
// pin that the pass count depends on the cut sites, not on the file's size.
// Parse is a library entry point, so the counter is atomic.
var flagPatternCutPasses atomic.Int64

const (
	// mvdan/sh ends a flagged subscript pattern at the first `]`, so the
	// byte after a bracket expression's own `]` is reported as an operator
	// ("not a valid parameter expansion operator: `]`"), as a stray word
	// ("`line` cannot be followed by a word"), or, in an assignment, as a
	// missing `=`.
	invalidFlagPatternOperator = "not a valid parameter expansion operator:"
	invalidFlagPatternWord     = " cannot be followed by a word"
	invalidFlagPatternAssign   = "`a[b]` must be followed by `=`"
	// A `,` after the cut is bash's case-modification operator, which the
	// Zsh dialect reports as a language error rather than a parse error
	// (issue #283). The feature text is shared with other operators, so the
	// gate is the structural one below: the byte before the error is the
	// premature `]`, a flagged subscript opens before it, and the retry
	// must come back holding that pattern whole.
	bashOnlyExpansionOperator = "this expansion operator"
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
	var langErr syntax.LangError
	isParseErr := errors.As(firstErr, &parseErr)
	isLangErr := errors.As(firstErr, &langErr)
	if !isParseErr && !isLangErr {
		return nil, firstErr
	}

	var seed int
	var patternStart int
	var ok bool
	positional := false
	switch {
	case isLangErr:
		// `,` after the cut is bash's case-modification operator. The
		// feature text names no construct of its own, so the whole gate
		// is structural: the premature `]` before it, a flagged
		// subscript opening before that, and the verified retry below.
		//
		// This text check is defence in depth, not the gate: the only
		// other language error mvdan/sh raises here is `${!foo}`, whose
		// position never sits past a premature `]`, so the structural
		// check below rejects it anyway. Keep it as the cheap early-out
		// that states the intent.
		if langErr.Feature != bashOnlyExpansionOperator {
			return nil, firstErr
		}
		seed = int(langErr.Pos.Offset())
		if seed < 0 || seed >= len(src) {
			return nil, firstErr
		}
		patternStart, ok = flagPatternBeforePrematureClose(src, seed)
	case strings.HasPrefix(parseErr.Text, invalidFlagPatternOperator),
		strings.HasSuffix(parseErr.Text, invalidFlagPatternWord):
		seed = int(parseErr.Pos.Offset())
		if seed < 0 || seed >= len(src) {
			return nil, firstErr
		}
		patternStart, ok = flagPatternBeforePrematureClose(src, seed)
	case parseErr.Text == invalidFlagPatternAssign:
		seed = int(parseErr.Pos.Offset())
		if seed < 0 || seed >= len(src) {
			return nil, firstErr
		}
		patternStart, ok = flagPatternAfterAssignName(src, seed)
		// The assignment error is reported at the name, not at the
		// premature `]`, so there is no cut byte to match below.
		seed = -1
	default:
		// Outside the texts above the cut surfaces as whatever error the
		// bytes after it produce. In arithmetic (issue #368) they are read
		// as arithmetic: `#` is an invalid operator, `*` a missing operand,
		// `.` runs on to a stray `)`. The same cut after a length prefix,
		// `${#m[(i)a[bc]]}`, or before a `:`, `${m[(i)a[bc]:]}`, reports
		// a combined-operator or ternary error. The text depends on the
		// pattern's own bytes, so the gate is positional: the error sits
		// after a `]` the scan masks, in a flagged pattern on the same
		// line, and holdsFlagPattern below verifies the retry.
		seed = int(parseErr.Pos.Offset())
		if seed < 0 || seed > len(src) {
			return nil, firstErr
		}
		patternStart, ok = flagPatternCutBeforeError(src, seed)
		positional = true
		// The error need not sit just past the premature `]`, so there is
		// no single cut byte to match below; the helper already required
		// a masked `]` before the error.
		seed = -1
	}
	if !ok {
		return nil, firstErr
	}

	edits, close, ok := scanFlagPatternBrackets(src, patternStart)
	if !ok {
		return nil, firstErr
	}
	if positional && !positionalPatternDecidable(src[patternStart:close]) {
		return nil, firstErr
	}
	// The error must point just past a `]` this scan masked, which ties the
	// repair to the cut the parser actually reported. Defence in depth: every
	// shape that reaches here does satisfy it, because the same premature `]`
	// is what flagPatternBeforePrematureClose found the pattern from.
	if seed >= 0 && !editsContain(edits, seed-1) {
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
	if !holdsFlagPattern(tree, patternStart, close) {
		return nil, firstErr
	}
	if err := restorePatternEdits(tree, src, edits); err != nil {
		return nil, err
	}
	return tree, nil
}

// holdsFlagPattern reports whether tree has a flagged subscript whose pattern
// is one literal spanning exactly [start,end), the node the masked retry must
// produce. It replaces the assignment's earlier `=`-after-the-subscript check
// with the shape that check stood in for, and covers the range form, where the
// pattern ends at a `,` the check could not see.
func holdsFlagPattern(tree *syntax.File, start, end int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		flagged, ok := node.(*syntax.FlagsArithm)
		if !ok {
			return true
		}
		word, ok := flagged.X.(*syntax.Word)
		if !ok || len(word.Parts) != 1 {
			return true
		}
		lit, ok := word.Parts[0].(*syntax.Lit)
		if !ok {
			return true
		}
		found = int(lit.ValuePos.Offset()) == start && int(lit.ValueEnd.Offset()) == end
		return !found
	})
	return found
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
		if src[i] == '(' && isFlagGroupOpener(src, i) {
			open = i - 1
			break
		}
	}
	if open < 1 {
		return 0, false
	}
	patternStart, ok := flagPatternStart(src, open)
	if !ok || patternStart > prematureClose {
		return 0, false
	}
	return patternStart, true
}

// flagPatternCutBeforeError locates a flagged subscript on seed's line whose
// pattern mvdan/sh cut at a bracket expression's `]` before seed, and returns
// the pattern start (issue #368). It walks the flag-group openers back from
// seed and takes the nearest whose scanned pattern masks a `]` before seed,
// so a later whole pattern between the cut and the error does not hide it.
func flagPatternCutBeforeError(src []byte, seed int) (int, bool) {
	for i := min(seed, len(src)) - 1; i > 0; i-- {
		if src[i] == '\n' {
			return 0, false
		}
		if src[i] != '(' || !isFlagGroupOpener(src, i) {
			continue
		}
		start, ok := flagPatternStart(src, i-1)
		if !ok || start > seed {
			continue
		}
		edits, _, ok := scanFlagPatternBrackets(src, start)
		if !ok {
			continue
		}
		for _, edit := range edits {
			if edit.original == ']' && edit.offset < seed {
				return start, true
			}
		}
	}
	return 0, false
}

// positionalPatternDecidable reports whether a flagged pattern found by the
// positional arm is one the repair can hand to the parser without changing how
// native Zsh reads the enclosing expression (issue #368). The rules below are
// arithmetic's, the strictest reader the arm serves; under `${#` or before a
// `:` they refuse only rows main already rejects.
//
// Parentheses must balance. Zsh finds the end of `$(( ))` and `(( ))` by
// counting parentheses, so `$(( m[(i)a[b)c]] ))` and `$(( m[(i)a[b)(c]] ))`
// are parse errors natively, while `a[b()c]` and `(a|b)[bc]` are valid.
// mvdan/sh reads the flagged pattern as one raw literal and never counts them,
// so once the mask lets that literal reach the real `]` every such row would
// be accepted. A `)` with nothing open refuses on sight, so a later `(` cannot
// rebalance it.
//
// A quote or a backslash refuses outright. Native Zsh does not count a quoted
// parenthesis, so a byte count would read `a[b(c")"]` as balanced when Zsh
// reads an unclosed `(`; a byte count and a quote are not composable (the #371
// lesson). A backslash changes more than the count: with `\]` inside,
// `$(( m[(i)a\]b[x]] ))` is not arithmetic natively at all. Zsh falls back to
// a command substitution holding a subshell and reports
// `no matches found: m[(i)a]b[x]]` when run, and the same pattern in a
// `for (( ))` header is a runtime parse error. Accepting it as arithmetic would
// be a false accept with a wrong tree. Every refused row keeps the verdict it
// has on main.
func positionalPatternDecidable(pattern []byte) bool {
	depth := 0
	for _, b := range pattern {
		switch b {
		case '"', '\'', '\\':
			return false
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return false
			}
			depth--
		}
	}
	return depth == 0
}

// isFlagGroupOpener reports whether the `(` at paren opens a subscript's
// `(flags)` group. A flagged pattern stands in two places: at the start of a
// subscript, `name[(i)pat]`, and after a range's `,`, `${a[1,(i)pat]}`
// (zshparam, Subscript Flags, where a range endpoint is itself a subscript
// expression and may carry flags).
//
// The range form is also the shape the second-subscript adapter's mask
// produces, since it rewrites the `][` boundary as `, `. Accepting it here is
// what lets the two adapters compose on `${a[b][(i)[x]]}` (issue #250), and it
// is not a concession to that mask: the form fails identically without any
// second subscript, so this is the grammar position, not the mask, being
// handled.
//
// flagPatternStart reads the flags from paren+1 either way, so both openers
// are reported through the byte before the `(`.
func isFlagGroupOpener(src []byte, paren int) bool {
	if paren < 1 {
		return false
	}
	// `name[(flags)`: the subscript's own opener. The byte before the `[` is
	// a name character, or the `}` of a nested expansion the subscript
	// applies to, `${${a[b]}[(i)pat]}`.
	if src[paren-1] == '[' {
		return paren >= 2 && (isIdentByte(src[paren-2]) || src[paren-2] == '}')
	}
	// `,(flags)` or, after the second-subscript mask, `, (flags)`.
	i := paren - 1
	for i > 0 && (src[i] == ' ' || src[i] == '	') {
		i--
	}
	return src[i] == ',' && commaInsideSubscript(src, i)
}

// commaInsideSubscript reports whether the `,` at comma stands inside a
// subscript, by walking back to the `[` that opens it on the same line. The
// byte before that `[` is a name character, or the `}` of a nested expansion
// the subscript applies to, matching the opener test above.
//
// Without this an arithmetic comma followed by a parenthesized operand,
// `$(( a, (b) ))`, would answer the opener test on position alone.
func commaInsideSubscript(src []byte, comma int) bool {
	depth := 0
	for i := comma - 1; i > 0; i-- {
		switch src[i] {
		case '\n':
			return false
		case ']':
			depth++
		case '[':
			if depth > 0 {
				depth--
				continue
			}
			return isIdentByte(src[i-1]) || src[i-1] == '}'
		}
	}
	return false
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
// real end, collecting the mask for every `[` and `]` that does not delimit
// the subscript and every `,` inside a bracket expression. Zsh delimits a
// subscript by bracket nesting with backslash escapes, so the scanner counts
// `[` and `]` and skips escaped bytes. The pattern ends at the `]` that closes
// the subscript, or at the `,` that separates it from a range's second
// endpoint (`${a[(r)[^:],3]}`, zshparam Array Subscripts); the returned offset
// is that byte, and the bytes after a `,` belong to the expression the
// after-comma adapter (#277) reads. It reports false when the pattern holds
// anything whose extent it cannot decide, such as a nested expansion or a
// command substitution in either form.
func scanFlagPatternBrackets(src []byte, start int) ([]patternEdit, int, bool) {
	var edits []patternEdit
	depth := 0
	parens := 0
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
		case b == '`', b == '$' && i+1 < len(src) && src[i+1] == '(':
			// A command substitution, in either form, and an arithmetic
			// expansion, which is `$(` with a nested `(` (issue #379).
			// Its `,` is masked like any other byte inside the pattern;
			// its extent is decided by the delimiter, and a shape the
			// helper cannot decide keeps the parser error.
			end, ok := maskPatternSubstitution(src, i, mask)
			if !ok {
				return nil, 0, false
			}
			i = end
			// The parenthesis count is deliberately NOT reset here. A
			// group may open before a substitution and close after it,
			// as in `${m[(i)(a|b$(echo x))]}`, which Zsh accepts, so
			// discarding the count would lose a legitimately open group.
			// The substitution's own parentheses are already balanced by
			// maskPatternSubstitution, so they cannot affect this count.
		case b == '(':
			parens++
			i++
		case b == ')':
			if parens == 0 {
				return nil, 0, false
			}
			parens--
			i++
		case b == '$' && i+1 < len(src) && src[i+1] == '{':
			// A nested expansion's extent is decided by brace nesting, so
			// an unbalanced one is refused like any other shape this
			// scanner cannot bound. Its `[`, `]` and `,` are masked like
			// any other byte inside the pattern (issue #371): mvdan/sh
			// reads the whole flagged pattern as one raw literal and stops
			// it at the first `]` whatever encloses that byte, so leaving
			// the nested brackets alone cut `${m[(r)${Z[a]}]}` at the
			// inner `]`. The bytes are restored from the literal either
			// way, and holdsFlagPattern verifies the retry produced the
			// whole pattern as that one literal.
			end, ok := maskNestedExpansion(src, i+1, mask)
			if !ok {
				return nil, 0, false
			}
			i = end
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
				if b == '}' || len(edits) == 0 {
					return nil, 0, false
				}
				// A range's `,` ends the pattern the same way its `]`
				// does. The expression after it is not this adapter's;
				// masking the pattern is enough for the parser to read
				// the `,` as the range separator it is.
				return edits, i, true
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

// maskNestedExpansion reports where a nested expansion inside a flagged
// pattern ends, and masks the `[`, `]` and `,` inside it so the raw pattern
// literal the parser reads runs past them to the `]` that really ends the
// subscript (issue #371).
//
// A nested expansion is not a node of its own inside a flagged pattern.
// mvdan/sh reads the whole pattern as one `*syntax.Lit`, so `${m[(r)${Z[a]}]}`
// ended at the inner `]` and left `}]}` as a stray word, and `${m[(r)${a[1,2]}]}`
// split the index at the inner `,` into a range the source does not have. The
// masked bytes are restored from that same literal by restorePatternEdits, so
// masking them is byte-for-byte invisible to callers.
//
// The brackets must balance, or the expansion is refused and the parser error
// stands. Masking a byte hides it from the parser until the tree is built, so
// an unbalanced `]` would be masked into invisibility and the outer subscript
// would close over invalid source: `${m[(r)${Z]}]}` and `${m[(r)${Z[a]]}]}`
// are `bad substitution` natively and were rejected before this mask existed.
// Refusing keeps them rejected, which is the adapter contract's rule that an
// extent the scanner cannot decide returns the parser error rather than a
// guess.
//
// Quoting is deliberately not tracked, so a quote inside the nested expansion
// refuses it. That is not fastidiousness: a byte count and a quote are not
// composable. A quoted bracket still moves the count, so a quoted `[` can
// rebalance an unquoted stray `]` and hand the parser
// `${m[(r)${Z[a]]"["}]}` — `bad substitution` natively — as a balanced
// expansion. Tracking quotes properly would mean reproducing native Zsh's own
// rule for a quote inside a subscript, which is position-dependent and
// asymmetric between quote kinds: `x=${Z["a]b"]}` is valid while the same
// expansion in command position is not, and `${Z[${Y['a b']}]}` is valid while
// `${Z[${Y["a b"]}]}` is not. Refusing is the honest answer until something
// implements that rule; every refused row keeps the verdict it has on main
// rather than gaining one.
//
// The front end's wider divergence on that quoted family is separate and
// pre-existing: `${m[${Z["a b"]}]}` has no flagged pattern, never reaches this
// scanner, and is accepted on main although Zsh rejects it (tracked in #374).
func maskNestedExpansion(src []byte, brace int, mask func(int)) (int, bool) {
	var masked []int
	depth := 0
	brackets := 0
	for i := brace; i < len(src); i++ {
		switch src[i] {
		case '\\':
			// The same masking the outer scan does at its own `\[` and
			// `\]`: mvdan/sh cuts a raw pattern at the bracket even when
			// it is escaped. The escape means the byte is not a subscript
			// delimiter, so it must not move the balance count either.
			//
			// The end-of-source arm mirrors the outer scan's refusal and
			// is defensive: a trailing backslash also runs the loop off
			// the end, which refuses anyway, so no input distinguishes
			// the two paths. Kept so the two scanners read alike.
			if i+1 >= len(src) || src[i+1] == '\n' {
				return 0, false
			}
			if src[i+1] == '[' || src[i+1] == ']' {
				masked = append(masked, i+1)
			}
			i++
		case '\n':
			return 0, false
		case '"', '\'':
			// See the quoting note above: the count cannot survive one.
			return 0, false
		case '{':
			depth++
		case '[':
			// Masking the opener is verdict-redundant, measured: with it
			// disabled, no verdict changed across 315 generated
			// flag-pattern shapes (flag x nested shape x context) and all
			// 187 tree fixtures. mvdan/sh cuts a raw pattern at `]` and
			// `,`, never at `[`. It is kept so the masked source stays
			// bracket-balanced for the other adapters the retry runs
			// through, and the helper's own unit test pins the offsets.
			brackets++
			masked = append(masked, i)
		case ']':
			// A `]` with nothing open cannot be masked away: masking hides
			// it from the parser, so the outer subscript would close over
			// source native Zsh rejects as `bad substitution`. Refuse on
			// sight rather than at the closing `}`, so no later `[` can
			// bring the count back to zero and make it look balanced.
			if brackets == 0 {
				return 0, false
			}
			brackets--
			masked = append(masked, i)
		case ',':
			masked = append(masked, i)
		case '}':
			depth--
			if depth == 0 {
				if brackets != 0 {
					return 0, false
				}
				for _, offset := range masked {
					mask(offset)
				}
				return i + 1, true
			}
		}
	}
	return 0, false
}

func editsContain(edits []patternEdit, offset int) bool {
	for _, edit := range edits {
		if edit.offset == offset {
			return true
		}
	}
	return false
}

// maskPatternSubstitution reports where a command substitution, in either the
// `$(...)` or the backquoted form, or an arithmetic expansion `$((...))` inside
// a flagged subscript pattern ends, and masks the `,` inside it so the raw
// pattern literal the parser reads runs past it to the byte that really ends
// the subscript (issue #379).
//
// mvdan/sh reads the whole flagged pattern as one `*syntax.Lit` and already
// keeps a substitution's own bytes inside that literal: `${m[(i)ab$(echo x)]}`
// parses on main. What it does not do is reach past the `]` of a bracket
// expression, so `${m[(i)a[bc]$(echo x)]}` was cut at that `]` and the `$` after
// it read as a parameter-expansion operator. Stepping over the substitution is
// the whole repair: the masked bytes are restored from that same literal by
// restorePatternEdits, and holdsFlagPattern verifies the retry produced the
// pattern as one literal spanning the pattern exactly.
//
// Only the `,` is masked. Zsh reads a `,` inside the substitution as the
// substitution's own byte, while mvdan/sh splits a subscript index at any `,`
// it sees in the literal, so `${m[(i)a[bc]$(echo a,b)]}` would otherwise become
// a range the source does not have.
//
// Every other punctuation byte refuses the pattern, because native Zsh's
// reading of it inside a substitution inside a subscript is not what the
// containing constructs suggest, and reproducing it here is not decidable.
// Measured with `zsh -f -n` on `${m[(i)a[bc]$(echo X)]}` and its backquoted
// twin:
//
//   - A bracket is counted as a subscript delimiter regardless of the
//     substitution and regardless of quoting: `$(echo [)` and `$(echo "]")`
//     are both `bad substitution`, while `$(echo "[]")` is valid because the
//     pair balances and `$(echo a]b[c)` is valid although its first `]` closes
//     a bracket opened after it. Masking a bracket would hide it from our own
//     count as well as the parser's, so a row Zsh rejects would be accepted.
//   - A parenthesis is counted the same way and is likewise not quote-exempt:
//     `$(echo ")")` and `$(echo "(")` are `bad substitution`, so the quoted
//     `)` really does close the substitution. A byte count and a quote are not
//     composable (the #371 lesson), so any quote refuses rather than being
//     counted through; that in turn is why an unquoted parenthesis may be
//     counted for the extent below.
//   - A brace is quote-sensitive, unlike the two above: `$(echo })` is a parse
//     error where `$(echo "}")` is valid, since the unquoted `}` closes the
//     enclosing `${`. Deciding that needs the quote tracking this helper
//     declines to do, so either brace refuses.
//   - A `#` would comment out the rest of the line including the delimiter
//     (`$(echo x # )` is a native parse error), and the adapter contract
//     forbids masking a byte a `*syntax.Comment` would hold.
//   - A newline refuses, matching the outer scan, which bounds a pattern to one
//     line.
//
// Every refused row keeps the verdict it has on main rather than gaining one,
// which is the bar: valid Zsh that stays rejected is a documented remaining
// gap, an acceptance Zsh does not give is a defect.
func maskPatternSubstitution(src []byte, start int, mask func(int)) (int, bool) {
	if src[start] == '`' {
		return maskBackquotedSubstitution(src, start, mask)
	}
	// `$(`, which is also the arithmetic expansion `$((`. Both end at the
	// `)` that balances the nesting opened here, so one counter serves.
	depth := 0
	var masked []int
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '[', ']', '{', '}', '"', '\'', '#', '\n', '\\', '`':
			// See the refusals above. A backslash refuses with them:
			// an escaped delimiter is not a delimiter, so counting it
			// would be wrong and skipping it would hide a `]` the
			// outer scan needs. A nested backquoted substitution
			// refuses because its body is read by backquote rules,
			// under which the `)` that ends this one cannot be found.
			return 0, false
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				// The bytes between the opener and here stay inside
				// the raw pattern literal, so nothing downstream
				// ever reads them as the commands they are. Parse
				// the body on its own or the repair accepts a
				// substitution Zsh rejects: `$(done)`, `$(| echo a)`
				// and `$(echo ())` are each a native error, and
				// stepping over them silently turned all three into
				// accepted source.
				if !substitutionBodyParses(src[start+2 : i]) {
					return 0, false
				}
				for _, offset := range masked {
					mask(offset)
				}
				return i + 1, true
			}
		case ',':
			masked = append(masked, i)
		}
	}
	return 0, false
}

// substitutionBodyParses reports whether a command substitution's body is a
// command list the front end can read on its own.
//
// scanFlagPatternBrackets keeps a substitution's bytes inside the flagged
// pattern's raw literal, which is what lets the pattern parse at all, so the
// body is never offered to the parser as the code it is. Without this check the
// repair accepts any bytes that merely balance their parentheses, including a
// bare `done`, a leading `|`, and an empty `( )` group that Zsh reports as
// `closing brace expected`.
//
// A body that fails here refuses the whole repair rather than reporting the
// body's own error, because the pattern is the construct under repair and the
// user's source is invalid either way: refusing keeps main's error, which names
// the pattern, instead of inventing a position inside a substitution the
// upstream parser never entered.
func substitutionBodyParses(body []byte) bool {
	if isZshIncompleteWord(bytes.TrimSpace(body)) {
		return false
	}
	// An empty body needs no special case: `$()` is valid Zsh, and the parser
	// reads empty input as an empty command list without error.
	parser := syntax.NewParser(syntax.Variant(syntax.LangZsh))
	_, err := parser.Parse(bytes.NewReader(body), "substitution")
	return err == nil
}

// zshIncompleteWords are words upstream parses as an ordinary command where Zsh
// reports a parse error, so a body consisting of one of them alone must be
// refused by name rather than by asking the parser.
//
// Measured with `zsh -f -n` on a file holding only the word, against
// `syntax.LangZsh` on the same bytes. `else` is a reserved word that cannot open
// a list; `nocorrect` and `repeat` are prefixes that require a following
// command; `foreach` and `end` belong to the `foreach` loop the front end does
// not support yet (issue #214). Words that merely look reserved are NOT here
// because Zsh accepts them alone as ordinary commands: `in`, `fo`, `time` and
// `coproc` all exit 0, so refusing them would reject valid source.
var zshIncompleteWords = map[string]struct{}{
	"else":      {},
	"nocorrect": {},
	"repeat":    {},
	"foreach":   {},
	"end":       {},
}

// isZshIncompleteWord reports whether the body is exactly one of the words Zsh
// rejects standalone but upstream accepts.
func isZshIncompleteWord(body []byte) bool {
	_, ok := zshIncompleteWords[string(body)]
	return ok
}

// maskBackquotedSubstitution is maskPatternSubstitution's arm for the
// backquoted form, whose extent is the next unescaped backquote rather than a
// balance count.
//
// Its refusals are the `$(` arm's plus the parenthesis, which the backquoted
// form does not get to treat as ordinary text either: a body of `echo )` or
// `echo (` is `bad substitution` natively, so Zsh counts a parenthesis here as
// well, and a body of `echo a]b[c` is rejected where the `$(` spelling of the
// same body is valid. Rather than model that asymmetry, this arm accepts a
// body of ordinary bytes and refuses every delimiter, which is enough for the
// reported shape (a backquoted `echo x` after a bracket expression) and keeps
// everything else at main's verdict.
func maskBackquotedSubstitution(src []byte, start int, mask func(int)) (int, bool) {
	var masked []int
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '[', ']', '{', '}', '(', ')', '"', '\'', '#', '\n', '\\', '$':
			// `$` refuses with them: a nested `$(` or `${` inside the
			// backquotes reintroduces a delimiter this arm does not
			// count, and a bare `$name` is refused with it rather
			// than distinguishing the spellings.
			return 0, false
		case ',':
			masked = append(masked, i)
		case '`':
			for _, offset := range masked {
				mask(offset)
			}
			return i + 1, true
		}
	}
	return 0, false
}

// resolveFlagPatternCuts repairs a flagged subscript pattern the parser cut at
// a bracket expression's `]` without raising an error at all (issue #283).
//
// When the byte after that premature `]` is `#` or `##`, it is a valid Zsh
// expansion operator, so `${m[(r)a[^:]##]}` parses with the pattern `a[^:` and
// an `Expansion` operator whose word is the rest of the subscript. No error
// exists for an adapter to gate on, and every rule then reads a pattern and an
// operator the source does not have.
//
// The recognizer is the cut itself, found on the tree the parse produced: a
// flagged subscript whose pattern literal ends before the `]` or `,` that
// scanFlagPatternBrackets, applying Zsh's own bracket nesting, says ends it.
// Each cut site is masked and the file reparsed through the chain, exactly as
// the error-gated adapter does; the retry must come back with each pattern
// whole, or the parser's original tree stands.
//
// reparse is the reading path the retry uses. It must be the same path that
// produced tree, or the retry fails on a construct the original parse had
// already read and the repair silently does not happen: a file holding an
// anonymous function invocation parses only through parseAnonymousFunctionArgs,
// so with the bare chain as the retry every cut in such a file went unrepaired
// (found while fixing #382). The island path inside that fallback passes the
// bare chain deliberately, which is what bounds the recursion.
func resolveFlagPatternCuts(
	src []byte,
	name string,
	tree *syntax.File,
	reparse func([]byte, string) (*syntax.File, error),
) *syntax.File {
	// A flagged pattern opens at `[(` (a subscript's own flags) or at `,(`
	// (a range endpoint's). Neither is common, so this keeps the walk off
	// files that cannot hold one.
	if !bytes.Contains(src, []byte("[(")) && !bytes.Contains(src, []byte(",(")) {
		return tree
	}
	// Each pass repairs every cut the current tree shows. A repaired
	// pattern can reveal a further cut that the misread tree hid, so the
	// passes run to a fixpoint.
	//
	// The mask is cumulative: a pass keeps every byte earlier passes
	// masked and adds the new sites. Masking each pass's sites against the
	// original bytes alone would unmask the earlier repairs, bring their
	// cuts back, and oscillate. The loop stops as soon as a pass adds no
	// new byte, so it runs at most once per distinct masked byte, never
	// per `]` in the file; a shape that never converges costs a few
	// reparses, not a count that grows with the file.
	var mask []patternEdit
	for {
		flagPatternCutPasses.Add(1)
		grown := mergePatternEdits(mask, flagPatternCutEdits(src, tree))
		if len(grown) == len(mask) {
			return tree
		}
		mask = grown
		masked := bytes.Clone(src)
		for _, edit := range mask {
			masked[edit.offset] = edit.replacement
		}
		retried, err := reparse(masked, name)
		if err != nil {
			return tree
		}
		if err := restorePatternEdits(retried, src, mask); err != nil {
			return tree
		}
		tree = retried
	}
}

// mergePatternEdits returns the edits in base plus every edit in add whose
// offset base does not already hold, sorted by offset. The result never holds
// two edits at one offset, which restorePatternEdits refuses.
func mergePatternEdits(base, add []patternEdit) []patternEdit {
	merged := append([]patternEdit(nil), base...)
	for _, edit := range add {
		if !editsContain(merged, edit.offset) {
			merged = append(merged, edit)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].offset < merged[j].offset })
	return merged
}

// flagPatternCutEdits returns the mask for every flagged subscript in tree
// whose pattern literal stops before the byte that really ends it. The edits
// are ordered by offset and hold no duplicate, since the sites are disjoint
// spans walked in source order.
func flagPatternCutEdits(src []byte, tree *syntax.File) []patternEdit {
	var edits []patternEdit
	covered := 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		flagged, ok := node.(*syntax.FlagsArithm)
		if !ok {
			return true
		}
		word, ok := flagged.X.(*syntax.Word)
		if !ok || len(word.Parts) != 1 {
			return true
		}
		lit, ok := word.Parts[0].(*syntax.Lit)
		if !ok {
			return true
		}
		start := int(lit.ValuePos.Offset())
		if start < covered {
			return true
		}
		siteEdits, close, ok := scanFlagPatternBrackets(src, start)
		// A pattern the scanner can bound past this literal's end is one
		// the parser cut; anything else is already whole and must be left
		// alone. `!ok` is redundant with the bound test, since a refusal
		// reports close 0, but stating both keeps the two conditions
		// independent of that.
		if !ok || close <= int(lit.ValueEnd.Offset()) {
			return true
		}
		edits = append(edits, siteEdits...)
		covered = close
		return true
	})
	return edits
}
