package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// arithForSite is one `for (( expr1 ; expr2 ; expr3 ))` header in command
// position whose body is not a `do` list: native Zsh (zshmisc) reads the
// arithmetic header, skips separators (semicolons, newlines, blanks and
// comments), and then reads a `do` list, a `{ list }` or one sublist.
type arithForSite struct {
	// start is the offset of the `for` word.
	start int
	// headerEnd is the offset just past `))`.
	headerEnd int
	// gap holds the separators native Zsh skips before the body.
	gap repeatGap
	// bodyStart is the offset of the body's first byte, or len(src).
	bodyStart int
}

// scanArithForSites finds every `for (( ... ))` header in command position
// whose body the parser cannot read on its own, in source order. A header
// that is already followed by `do` parses without help and is not a site.
func scanArithForSites(src []byte) []arithForSite {
	var sites []arithForSite
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		if word != "for" || end >= len(src) || (src[end] != ' ' && src[end] != '\t') {
			return 0, false
		}
		site, ok := scanArithForSite(src, start)
		if !ok {
			return 0, false
		}
		if !matchSourceWord(src, site.bodyStart, "do") {
			sites = append(sites, site)
		}
		return site.bodyStart, true
	})
	return sites
}

// scanArithForSite reads the header of the arithmetic `for` loop at start.
// The header must start with `((` after inline spaces, and its closing `))`
// is located by reading the header via the parser (probeArithForHeader)
// rather than a naive paren count.
func scanArithForSite(src []byte, start int) (arithForSite, bool) {
	hdrStart := skipInlineSpaces(src, start+len("for"))
	if hdrStart+1 >= len(src) || src[hdrStart] != '(' || src[hdrStart+1] != '(' {
		return arithForSite{}, false
	}
	hdrEnd, ok := probeArithForHeader(src, start)
	if !ok {
		return arithForSite{}, false
	}
	site := arithForSite{start: start, headerEnd: hdrEnd}
	site.gap, site.bodyStart = scanRepeatGap(src, hdrEnd)
	return site, true
}

// probeArithForHeader locates the closing `))` of the arithmetic header starting
// at forStart by using the parser's own reading. For each candidate `))` in the
// source, it appends a synthetic `; do :; done\n` and tests whether the parser
// reads a single `ForClause` whose `CStyleLoop` ends at that candidate offset.
func probeArithForHeader(src []byte, forStart int) (int, bool) {
	for i := forStart + len("for"); i+1 < len(src); i++ {
		if src[i] == ')' && src[i+1] == ')' {
			candidate := append([]byte{}, src[forStart:i+2]...)
			candidate = append(candidate, []byte("; do :; done\n")...)
			p := syntax.NewParser(syntax.Variant(syntax.LangZsh))
			f, err := p.Parse(bytes.NewReader(candidate), "probe")
			if err == nil && len(f.Stmts) == 1 {
				if fc, ok := f.Stmts[0].Cmd.(*syntax.ForClause); ok && !fc.Select {
					if cl, ok := fc.Loop.(*syntax.CStyleLoop); ok {
						if int(cl.Rparen.Offset()) == i-forStart {
							return i + 2, true
						}
					}
				}
			}
		}
	}
	return -1, false
}

// parseArithForSublist adapts `for (( expr1 ; expr2 ; expr3 )) sublist`, the
// alternate form of the arithmetic for loop (issue #241). Under LangZsh,
// mvdan/sh rejects the brace body `{ list }` with a LangError at `{`, and
// rejects the single-command body with a ParseError at `for`. The body has
// no tree yet, so the retry is in two steps: a byte-preserving probe blanks
// the header of every unread site and parses, which yields the sublist
// statement at the body's first byte and, through the statement it widens
// to, the byte where the body ends; then a source-mapped retry inserts `do`
// and `done` (over `{` and `}` for the brace body, around the sublist
// otherwise, with a `;` when the header has no separator), and the tree
// is verified to hold a `ForClause` at the `for` word whose Loop is a
// `*syntax.CStyleLoop` and whose Do statements sit at their original offsets.
// The last unread site is rewritten first, so an enclosing site sees the
// inner loop with a real `done`.
func parseArithForSublist(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseArithForSublistWithParser(src, name, firstErr, parseWithAdapters)
}

func parseArithForSublistWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	var langErr syntax.LangError
	isParseErr := errors.As(firstErr, &parseErr)
	isLangErr := errors.As(firstErr, &langErr)

	if !isParseErr && !isLangErr {
		return nil, firstErr
	}
	// The error text is checked before the source is scanned: every site
	// costs a parser probe per candidate `))`, which an unrelated error must
	// not pay for.
	if isLangErr && langErr.Feature != "for loops with braces" {
		return nil, firstErr
	}
	if isParseErr && parseErr.Text != "`for foo [in words]` must be followed by `do`" {
		return nil, firstErr
	}

	sites := scanArithForSites(src)
	if len(sites) == 0 {
		return nil, firstErr
	}

	var errOffset int
	matched := false
	if isLangErr {
		errOffset = int(langErr.Pos.Offset())
		for _, s := range sites {
			if s.bodyStart == errOffset && errOffset < len(src) && src[errOffset] == '{' {
				matched = true
				break
			}
		}
	} else if isParseErr {
		errOffset = int(parseErr.Pos.Offset())
		for _, s := range sites {
			if s.start == errOffset {
				matched = true
				break
			}
		}
	}
	if !matched {
		return nil, firstErr
	}

	var unread []arithForSite
	for _, site := range sites {
		if site.start >= errOffset || site.bodyStart == errOffset {
			unread = append(unread, site)
		}
	}
	if len(unread) == 0 {
		return nil, firstErr
	}
	site := unread[len(unread)-1]

	probe := bytes.Clone(src)
	for _, blanked := range unread {
		if bang, ok := negationBefore(src, blanked.start); ok {
			probe[bang] = ' '
		}
		for i := blanked.start; i < blanked.bodyStart; i++ {
			if probe[i] != '\n' {
				probe[i] = ' '
			}
		}
		for i := blanked.bodyStart; i < len(src) && selectOperatorByte(src, blanked.bodyStart, i); i++ {
			probe[i] = ' '
		}
	}
	tree, err := parse(probe, name)
	if err != nil {
		if at, ok := errorOffset(err); ok && at > errOffset {
			return nil, err
		}
		return nil, firstErr
	}

	inner, outer, parents, comments := sublistStatements(tree, site.bodyStart)
	var closer int
	var body []int
	brace := repeatBodyForm(src, site.bodyStart) == repeatBodyBrace
	if brace {
		block := blockAt(tree, site.bodyStart)
		if block == nil {
			return nil, firstErr
		}
		closer = int(block.Rbrace.Offset())
		for _, stmt := range block.Stmts {
			body = append(body, int(stmt.Pos().Offset()))
		}
	} else if inner == nil {
		if !selectEmptyBodyAt(src, site.bodyStart) {
			return nil, firstErr
		}
		closer = site.bodyStart
	} else {
		body = []int{site.bodyStart}
		var ok bool
		closer, ok = repeatSublistEnd(src, parents, outer, comments, nil)
		if !ok {
			return nil, firstErr
		}
		if end := skipSpaces(src, closer); end >= len(src) || src[end] == '\n' {
			closer = end
		}
		if closer, ok = repeatSublistCloser(src, outer, closer); !ok {
			return nil, firstErr
		}
	}

	opener := "do\n"
	redundant := site.gap.semicolons
	switch {
	case site.gap.newline:
	case len(redundant) > 0:
		redundant = redundant[1:]
	default:
		opener = "; do\n"
	}
	var edits []repeatEdit
	for _, at := range redundant {
		edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
	}
	switch {
	case brace:
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart + 1, text: opener},
			repeatEdit{start: closer, end: closer + 1, text: "\ndone"},
		)
	case inner == nil:
		text := "done\n"
		if selectOperatorAt(src, closer) {
			text = "done"
		}
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: opener},
			repeatEdit{start: closer, end: closer, text: text},
		)
	default:
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: opener},
			repeatEdit{start: closer, end: closer, text: "\ndone"},
		)
	}

	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err = parse(transformed, name)
	if err != nil {
		err = rebaseForError(err, sm, lineStarts)
		if at, ok := errorOffset(err); ok && at <= site.start {
			return nil, firstErr
		}
		return nil, err
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing arith for loop positions: %w", name, err)
	}
	if !verifyArithForLoop(tree, site, closer, body) {
		return nil, firstErr
	}
	return tree, nil
}

// verifyArithForLoop reports whether tree holds an arithmetic for loop at
// site.start whose Loop is a *syntax.CStyleLoop, whose `do` is at site.bodyStart,
// whose `done` is at closer, and whose Do statements start exactly at the
// offsets in body.
func verifyArithForLoop(tree *syntax.File, site arithForSite, closer int, body []int) bool {
	verified := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if verified {
			return false
		}
		loop, ok := node.(*syntax.ForClause)
		if !ok || loop.Select || int(loop.ForPos.Offset()) != site.start {
			return true
		}
		if _, ok := loop.Loop.(*syntax.CStyleLoop); !ok {
			return false
		}
		if int(loop.DoPos.Offset()) != site.bodyStart || int(loop.DonePos.Offset()) != closer || len(loop.Do) != len(body) {
			return false
		}
		for i, offset := range body {
			if int(loop.Do[i].Pos().Offset()) != offset {
				return false
			}
		}
		verified = true
		return false
	})
	return verified
}
