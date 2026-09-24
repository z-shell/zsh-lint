package parse

import (
	"bytes"

	"mvdan.cc/sh/v3/syntax"
)

// flagPatternCutError is the parser's own text for the byte that ends a flagged
// subscript's pattern too early. The parser raises it itself whenever the cut is
// visible to it, so reusing it here is what makes the quoted spelling of a shape
// report what its unquoted twin already reports.
const flagPatternCutError = "not a valid parameter expansion operator: `]`"

// rejectFlagPatternCuts returns a parse error at the `]` that ended a flagged
// subscript's pattern before the `]` which really closes the subscript, for
// every such pattern the front end could not repair (issue #382).
//
// mvdan/sh reads a flagged subscript's pattern as one raw literal and ends it at
// the first `]`, wherever that byte sits. Inside a nested expansion that byte
// belongs to the inner subscript, so `${m[(i)${Z[a]}]]}` is read with the
// pattern `${Z[a` and the rest left over. resolveFlagPatternCuts repairs that
// reading by masking the nested brackets and reparsing; when the retry fails,
// the misread tree stands, and whether anything then reports an error depends on
// what the leftover bytes happen to mean in their context.
//
// That context dependence is the defect. Outside quotes the leftover `]]}`
// becomes a word ending in `}`, which rejectCloseBraceWords refuses, so the
// source is rejected; inside a double quote the same bytes are ordinary text, no
// guard looks at them, and the file is accepted although native Zsh reports
// `bad substitution`. An outer double quote silently selecting the verdict is a
// rule no user can infer.
//
// The gate is the repair's own failure, not the quoting: a pattern the bracket
// scan can bound past the literal's end is one the parser cut, and by the time
// this runs resolveFlagPatternCuts has already tried and failed to repair it.
// That is what makes the guard decidable — it never judges the pattern's bytes,
// only whether the front end managed to read them as Zsh does. A pattern the
// scanner cannot bound is left alone with the verdict it already has.
//
// A second arm catches the cuts the bracket scan cannot reach at all. That scan
// refuses any pattern holding a quote or an unbalanceable bracket, by design:
// masking a byte hides it from the parser, so a shape whose extent the scanner
// cannot decide must keep the verdict it has rather than gain one. But the
// refusal is decided on the source, while the cut is visible on the tree, and a
// literal that ends while still inside a `${` or `$(` it opened is a cut on its
// face — the parser stopped the pattern at a `]` belonging to that inner
// construct, since a pattern Zsh read whole would close what it opened.
//
// This arm is what covers `${m[(i)${Z[a]}$(echo ])]}`,
// `${m[(r)${Z[a]}$(echo [)]}` and `${m[(i)${Z["a b"]}]}`, whose brackets do not
// balance and which the scanner therefore refuses outright.
//
// A literal holding a single quote is excluded. That is not symmetry with the
// scanner's blanket quote refusal but a measured distinction: Zsh's rule for a
// quote inside a nested subscript is asymmetric between quote kinds, and the
// single-quoted spelling is the valid one. Verified against both oracles across
// contexts and flags, `${m[(i)${Y['a b']}]}` is valid while `${m[(i)${Y["a b"]}]}`
// is `bad substitution`, and the same holds nested two deep. So an ends-open
// literal such as `${Y['a b'` really can be the whole pattern of a valid row,
// where `${Y["a b"` cannot. Measured, dropping the exclusion turns 108 valid
// rows into rejections; keeping it is cheap, because the bracket-scan arm still
// reaches the single-quoted rows whose brackets it can bound, and rejects 36 of
// them that this arm declines.
//
// The double-quoted spelling that looks similar but is valid — `${m[(i)"${Z[a]}"]}`,
// quoting the nested expansion rather than a key inside its subscript — cannot
// be regressed by this arm: mvdan/sh does not reach a tree for it at all, but
// fails with `reached EOF without closing quote`, so it is a pre-existing gap
// this guard never sees.
//
// The two arms overlap without subsuming each other, measured by disabling each
// in turn and re-running both probes against the oracle. Arm 1 alone rejects 36
// rows of the quote probe — patterns holding a single quote, which arm 2
// declines by design but whose brackets the scan can still bound. Arm 2 alone
// rejects 1,278 rows of the main probe and 108 of the quote probe, the shapes
// whose brackets do not balance so the scan refuses them outright. Neither arm
// uniquely rejects any row native Zsh accepts.
//
// Measured over 16,464 generated rows (context x subscript position x flag x
// pattern) plus a 1,728-row quote-placement probe, judged against `zsh -f -n`
// plus the runtime verdict: 1,854 rows of the main probe and 108 of the quote
// probe that native Zsh rejects stop being accepted, and no row that native Zsh
// accepts starts being rejected. The runtime oracle is load-bearing here, since
// `zsh -n` is a parse check: in an assignment, a `case` word or a `[[ ]]` test,
// Zsh parses these rows and reports `bad substitution` only when the expansion
// is reached.
func rejectFlagPatternCuts(src []byte, tree *syntax.File, name string) error {
	// Same precheck resolveFlagPatternCuts uses: a flagged pattern opens at
	// `[(` (a subscript's own flags) or `,(` (a range endpoint's), so a file
	// holding neither cannot have one and neither arm can fire. Both arms
	// walk the tree, and without this they walked it on every parse: a file
	// with no flagged pattern at all paid about 15% of Parse for two walks
	// that could not find anything (measured over a 300-function file, 200
	// iterations, median of 5). Verified behavior-neutral across all 18,192
	// probe rows of both sets: zero verdict changes.
	//
	// Both markers are load-bearing. Dropping the `,(` arm skips 1,284 probe
	// rows whose files hold no `[(` at all, and is caught only by
	// invalid-382-range-endpoint-stray-close.txt.
	if !bytes.Contains(src, []byte("[(")) && !bytes.Contains(src, []byte(",(")) {
		return nil
	}
	if cut := firstPrematureClose(flagPatternCutEdits(src, tree)); cut >= 0 {
		return flagPatternCutParseError(src, cut, name)
	}
	if cut := firstUnclosedFlagPattern(tree); cut >= 0 {
		return flagPatternCutParseError(src, cut, name)
	}
	return nil
}

// flagPatternCutParseError reports the parser's own cut error at offset.
func flagPatternCutParseError(src []byte, offset int, name string) error {
	line, col := lineCol(src, offset)
	return syntax.ParseError{
		Filename: name,
		Pos:      syntax.NewPos(uint(offset), line, col),
		Text:     flagPatternCutError,
	}
}

// firstUnclosedFlagPattern returns the offset just past the first flagged
// pattern literal that ends while still inside a `${` or `$(` opened within it,
// or -1 when every such literal closes what it opened. That offset is the `]`
// the parser treated as ending the subscript, which is the byte to report.
func firstUnclosedFlagPattern(tree *syntax.File) int {
	cut := -1
	syntax.Walk(tree, func(node syntax.Node) bool {
		if cut >= 0 {
			return false
		}
		flags, ok := node.(*syntax.FlagsArithm)
		if !ok {
			return true
		}
		lit, ok := soleLiteral(flags.X)
		if ok && litEndsInsideExpansion(lit.Value) {
			cut = int(lit.ValueEnd.Offset())
			return false
		}
		return true
	})
	return cut
}

// soleLiteral returns the one *syntax.Lit a word is made of, which is how
// mvdan/sh represents a flagged subscript's pattern: the whole pattern is read
// raw, as a single literal. A word of any other shape was not read that way and
// is not this guard's to judge.
//
// The part count is a contract check, not a filter: probed over all 18,192
// distinct rows of both probe sets plus an adversarial set aimed at splitting
// the word (`$b`, `${b}c`, `"b"`, backquotes, `$(...)`, `$((...))`, `$~b`, a
// glob, a space), every flagged pattern came back as exactly one Lit and no
// FlagsArithm carried a non-Word X. Relaxing the count therefore survives
// mutation with no input distinguishing it; it is kept because the guard reads
// lit.Value as the whole pattern, which is only true at one part.
func soleLiteral(expr syntax.ArithmExpr) (*syntax.Lit, bool) {
	word, ok := expr.(*syntax.Word)
	if !ok || len(word.Parts) != 1 {
		return nil, false
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	return lit, ok
}

// litEndsInsideExpansion reports whether a flagged pattern literal ends while a
// `${` or `$(` opened inside it is still unclosed, and holds no single quote.
//
// A single quote returns false rather than being counted through: see the note
// on the second arm above. Its bytes are not skipped either, since a `'` that
// reached this literal has already been read raw by the parser and the row is
// declined whole.
func litEndsInsideExpansion(value string) bool {
	depth := 0
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\\':
			i++
		case '\'':
			return false
		case '$':
			if i+1 < len(value) && (value[i+1] == '{' || value[i+1] == '(') {
				depth++
				i++
			}
		case '}', ')':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

// firstPrematureClose returns the offset of the first `]` among the edits, the
// byte at which the parser ended the pattern, or -1 when the scan masked no `]`.
// A pattern whose mask holds only openers was not cut at a `]`, so it has no
// position to report and is left to whatever verdict it already has.
func firstPrematureClose(edits []patternEdit) int {
	for _, edit := range edits {
		if edit.original == ']' {
			return edit.offset
		}
	}
	return -1
}

// lineCol converts a byte offset into the 1-based line and column syntax.Pos
// wants. Positions elsewhere in the front end come from the parser; this guard
// reports a byte the parser never made a node for, so it counts them itself.
func lineCol(src []byte, offset int) (uint, uint) {
	line, col := uint(1), uint(1)
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return line, col
}
