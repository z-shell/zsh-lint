package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// forSite is one `for name [in word ...]` or `for name ( word ... )` header
// in command position whose body is a sublist: native Zsh (par_for in parse.c)
// reads the name, skips newlines, reads either `in` and a word list that the
// first `;` or newline ends, the parenthesized list, or a separator (positional
// parameters form), skips any number of `;` and newlines, and then reads a
// `do` list, a `{ list }` or one sublist.
type forSite struct {
	// start is the offset of the `for` word.
	start int
	// in reports an `in` word list; the term after it is the first
	// separator of the gap.
	in bool
	// paren reports a parenthesized word list: `for name ( word ... )`.
	paren bool
	// parenOpen and parenClose are the offsets of `(` and `)`.
	parenOpen  int
	parenClose int
	// newlines are the newlines inside the parenthesized list.
	newlines []int
	// words are the parenthesized list's words as [start, end) offsets.
	words [][2]int
	// gap holds the separators native Zsh skips before the body: after the
	// list's term in the `in` form, after `)` in the `paren` form, or after
	// the name in the positional form.
	gap repeatGap
	// bodyStart is the offset of the body's first byte, or len(src).
	bodyStart int
}

// scanForSites finds every `for` header in command position whose body is a
// short-form sublist, in source order. A header the scan does not recognise,
// a `do` body or a `{ list }` brace body is not a site.
func scanForSites(src []byte) []forSite {
	var sites []forSite
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		if word != "for" || end >= len(src) || (src[end] != ' ' && src[end] != '\t') {
			return 0, false
		}
		site, ok := scanForSite(src, start)
		if !ok {
			return 0, false
		}
		if !matchSourceWord(src, site.bodyStart, "do") && repeatBodyForm(src, site.bodyStart) != repeatBodyBrace {
			sites = append(sites, site)
		}
		return site.bodyStart, true
	})
	return sites
}

// scanForSite reads the header of the `for` loop at start. The `in` word
// list is read with the parser's own word lexer, so a quoted or expanded
// word never ends it; the list ends at the first `;` or unescaped newline
// after a word, as native Zsh reads it. The parenthesized list is read by
// scanParenWordList. The positional form requires a `;` or newline after the
// name.
func scanForSite(src []byte, start int) (forSite, bool) {
	nameStart := skipInlineSpaces(src, start+len("for"))
	nameEnd := nameStart
	for nameEnd < len(src) && isIdentByte(src[nameEnd]) {
		nameEnd++
	}
	if nameEnd == nameStart || (nameEnd < len(src) && !isRepeatWordEnd(src[nameEnd])) {
		return forSite{}, false
	}
	site := forSite{start: start}
	gap, next := scanRepeatGap(src, nameEnd)
	if next < len(src) && src[next] == '(' {
		if next+1 < len(src) && src[next+1] == '(' {
			return forSite{}, false
		}
		parenClose, newlines, words, ok := scanParenWordList(src, next)
		if !ok {
			return forSite{}, false
		}
		site.paren = true
		site.parenOpen = next
		site.parenClose = parenClose - 1
		site.newlines = newlines
		site.words = words
		site.gap, site.bodyStart = scanRepeatGap(src, parenClose)
		return site, true
	}
	if len(gap.semicolons) == 0 && matchSourceWord(src, next, "in") {
		term, ok := scanForWordList(src, next+len("in"))
		if !ok {
			return forSite{}, false
		}
		site.in = true
		gapStart := term
		if src[term] == ';' {
			gapStart++
		}
		site.gap, site.bodyStart = scanRepeatGap(src, gapStart)
		return site, true
	}
	if len(gap.semicolons) == 0 && !gap.newline {
		return forSite{}, false
	}
	site.gap, site.bodyStart = gap, next
	return site, true
}

// scanForWordList returns the offset of the `;` or newline that ends the
// word list starting at from. The lexer reads across newlines, so the gap
// before each word, and the gap before the token that stops the lexer, is
// checked for a newline that is not a `\`-newline continuation. A newline
// before any words is invalid in Zsh (issue #263) and ends the scan with ok false.
func scanForWordList(src []byte, from int) (int, bool) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangZsh))
	gapStart := from
	var err error
	wordsSeen := 0
	for word, wordErr := range parser.WordsSeq(bytes.NewReader(src[from:])) {
		if wordErr != nil {
			err = wordErr
			break
		}
		wordStart := from + int(word.Pos().Offset())
		if term, ok := selectListNewline(src, gapStart, wordStart); ok {
			if wordsSeen == 0 {
				return -1, false
			}
			return term, true
		}
		if lit, ok := word.Parts[0].(*syntax.Lit); ok && len(word.Parts) == 1 && lit.Value == "}" {
			return -1, false
		}
		wordsSeen++
		gapStart = from + int(word.End().Offset())
	}
	stop := len(src)
	var parseErr syntax.ParseError
	if errors.As(err, &parseErr) {
		stop = from + int(parseErr.Pos.Offset())
	} else if err != nil {
		return -1, false
	}
	if term, ok := selectListNewline(src, gapStart, stop); ok {
		if wordsSeen == 0 {
			return -1, false
		}
		return term, true
	}
	if err != nil && parseErr.Text == "`;` is not a valid word" && stop < len(src) && src[stop] == ';' {
		return stop, true
	}
	return -1, false
}

// parseForShortForm adapts `for name [in word ...] term sublist` and
// `for name ( word ... ) sublist`, the short forms of for (issue #211).
// The parser requires `do` after the header and reports one of two errors
// at the `for` word depending on whether the header carried a separator.
// The body has no tree yet, so the retry is in two steps: a byte-preserving
// probe blanks the header of every unread site and parses, which yields the
// sublist statement at the body's first byte and, through the statement it
// widens to, the byte where the body ends; then a source-mapped retry
// inserts `do` and `done` around the body (and rewrites a parenthesized
// word list to `in`), and the tree is verified to hold a for loop at the
// site whose body starts at the first byte and ends at the closer before
// every position is rebased. The last unread site is rewritten first, so an
// enclosing site's probe sees the loop it contains with a real `done`, and
// each site costs one probe and one retry.
func parseForShortForm(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseForShortFormWithParser(src, name, firstErr, parseWithAdapters)
}

func parseForShortFormWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	if !strings.HasPrefix(parseErr.Text, "`for foo") || !strings.Contains(parseErr.Text, "must be followed by") {
		return nil, firstErr
	}
	errOffset := int(parseErr.Pos.Offset())
	if !matchSourceWord(src, errOffset, "for") {
		return nil, firstErr
	}
	var unread []forSite
	for _, site := range scanForSites(src) {
		if site.start >= errOffset {
			unread = append(unread, site)
		}
	}
	if len(unread) == 0 || unread[0].start != errOffset {
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
		var probeErr syntax.ParseError
		if errors.As(err, &probeErr) && int(probeErr.Pos.Offset()) > errOffset {
			return nil, err
		}
		return nil, firstErr
	}
	inner, outer, parents, comments := sublistStatements(tree, site.bodyStart)
	var closer int
	// Nothing starts at the body's first byte although the rest of the file
	// parsed. That is the empty sublist native par_for reads at a closer,
	// at the end of the file, or before an operator that takes the loop as
	// its left operand (issue #327), the same shape the select adapter maps
	// through selectEmptyBodyAt; `do` and `done` both go on that byte.
	// Anything else there is a body the probe could not isolate and keeps
	// the parser error.
	empty := inner == nil
	if empty {
		if !selectEmptyBodyAt(src, site.bodyStart) {
			return nil, firstErr
		}
		closer = site.bodyStart
	} else {
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

	var edits []repeatEdit
	if site.paren {
		edits = append(edits, repeatEdit{start: site.parenOpen, end: site.parenOpen + 1, text: " in "})
		for _, at := range site.newlines {
			edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
		}
		closerText := ";"
		if next := skipInlineSpaces(src, site.parenClose+1); next < len(src) && (src[next] == ';' || src[next] == '\n') {
			closerText = " "
		} else if next := site.parenClose + 1; next < len(src) && (src[next] == '|' || src[next] == '&') {
			closerText = "; "
		}
		edits = append(edits, repeatEdit{start: site.parenClose, end: site.parenClose + 1, text: closerText})
		redundant := site.gap.semicolons
		if closerText == " " && !site.gap.newline && len(redundant) > 0 {
			redundant = redundant[1:]
		}
		for _, at := range redundant {
			edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
		}
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: "do\n"},
			repeatEdit{start: closer, end: closer, text: forDoneText(src, closer, empty)},
		)
	} else {
		opener := "do\n"
		redundant := site.gap.semicolons
		if !site.in {
			if !site.gap.newline && len(redundant) > 0 {
				redundant = redundant[1:]
			}
		}
		for _, at := range redundant {
			edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
		}
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: opener},
			repeatEdit{start: closer, end: closer, text: forDoneText(src, closer, empty)},
		)
	}

	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err = parse(transformed, name)
	if err != nil {
		err = rebaseForError(err, sm, lineStarts)
		var retryErr syntax.ParseError
		if errors.As(err, &retryErr) && int(retryErr.Pos.Offset()) <= site.start {
			return nil, firstErr
		}
		return nil, err
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing for loop positions: %w", name, err)
	}
	if !verifyForLoop(tree, site, closer, empty) {
		return nil, firstErr
	}
	return tree, nil
}

// forDoneText is the text inserted at the loop's closer. A non-empty body
// ends on its own last byte, so `done` goes on the next line. An empty body
// puts `do` and `done` on the same byte (issue #327): the closer that
// follows, or the end of the file, must not be glued to `done`, and an
// operator that takes the loop as its left operand must stay on its line.
func forDoneText(src []byte, closer int, empty bool) string {
	if !empty {
		return "\ndone"
	}
	if selectOperatorAt(src, closer) {
		return "done"
	}
	return "done\n"
}

// verifyForLoop reports whether tree holds a for loop at the site whose `do`
// is at the body's first byte, whose `done` is at the closer, and whose
// statements consist of exactly the one sublist statement, or none when the
// body is empty.
func verifyForLoop(tree *syntax.File, site forSite, closer int, empty bool) bool {
	verified := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if verified {
			return false
		}
		loop, ok := node.(*syntax.ForClause)
		if !ok || loop.Select || int(loop.ForPos.Offset()) != site.start {
			return true
		}
		if int(loop.DoPos.Offset()) != site.bodyStart || int(loop.DonePos.Offset()) != closer {
			return false
		}
		if empty {
			if len(loop.Do) != 0 {
				return false
			}
		} else {
			if len(loop.Do) != 1 || int(loop.Do[0].Pos().Offset()) != site.bodyStart {
				return false
			}
		}
		if site.paren {
			iter, ok := loop.Loop.(*syntax.WordIter)
			if !ok || int(iter.InPos.Offset()) != site.parenOpen || len(iter.Items) != len(site.words) {
				return false
			}
			for i, item := range iter.Items {
				if int(item.Pos().Offset()) != site.words[i][0] || int(item.End().Offset()) != site.words[i][1] {
					return false
				}
			}
		}
		verified = true
		return false
	})
	return verified
}
