package parse

import (
	"errors"
	"fmt"
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// selectParenListError is the parser's error for a select header whose name
// is followed by something other than `in`, `do` or a separator; a `(` is
// that something.
const selectParenListError = "`select foo` must be followed by `in`, `do`, `;`, or a newline"

// selectParenSite is one `select name ( word ... )` header in command
// position whose list the parser cannot read.
type selectParenSite struct {
	// start is the offset of the `select` word.
	start int
	// parenOpen and parenClose are the offsets of the `(` and the `)`.
	parenOpen, parenClose int
	// newlines are the newlines inside the list that native Zsh reads as
	// word separators.
	newlines []int
	// words are the list's words as [start, end) offsets.
	words [][2]int
}

// parseSelectParenList adapts `select name ( word ... ) body`, the
// parenthesized list form of select (issue #303), which native par_for
// reads through the same branch as `for name ( word ... )`. The parser has
// no such form, so the list is rewritten into the `in` form it does have:
// `(` becomes ` in `, a newline inside the list becomes a blank (an `in`
// list ends at a newline, the parenthesized one does not), and `)` becomes
// `;`, or a blank when a separator already follows it. The rewritten
// header is then a select the chain already reads: a `do` body natively, a
// sublist (#212), a `{ list }` (#301) or an empty body (#302), so the retry
// re-enters the chain and every position is rebased through the source map
// afterwards. Every site in the file is rewritten in the one pass, as the
// for adapter does: a pass per site would re-enter the chain, and the body
// adapters behind it, once per remaining site, which grows exponentially
// with the sites in a file. The tree is verified to hold, at every site, a
// select loop whose `in` sits on the `(` and whose items are the list's
// words on their own bytes; anything else keeps the parser error. A list
// carrying a comment is not rewritten (scanParenWordList), as for the for
// form, and a file holding one keeps the parser error for that loop.
func parseSelectParenList(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseSelectParenListWithParser(src, name, firstErr, parseWithAdapters)
}

func parseSelectParenListWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != selectParenListError {
		return nil, firstErr
	}
	errOffset := int(parseErr.Pos.Offset())
	if !matchSourceWord(src, errOffset, "select") {
		return nil, firstErr
	}
	sites := scanSelectParenSites(src)
	if len(sites) == 0 || !hasSiteAt(sites, errOffset) {
		return nil, firstErr
	}

	var edits []repeatEdit
	for _, site := range sites {
		edits = append(edits, repeatEdit{start: site.parenOpen, end: site.parenOpen + 1, text: " in "})
		for _, at := range site.newlines {
			edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
		}
		// The parser wants one `;` or a newline between the list and the
		// body, and the select adapters tolerate more; a separator the
		// source already has after the `)` is kept as the term. A `|` or
		// `&` glued to the `)` gets a blank after the `;`, as `;|` and `;&`
		// are case terminators (issue #319).
		closer := ";"
		if next := skipInlineSpaces(src, site.parenClose+1); next < len(src) && (src[next] == ';' || src[next] == '\n') {
			closer = " "
		} else if next := site.parenClose + 1; next < len(src) && (src[next] == '|' || src[next] == '&') {
			closer = "; "
		}
		edits = append(edits, repeatEdit{start: site.parenClose, end: site.parenClose + 1, text: closer})
	}

	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err := parse(transformed, name)
	if err != nil {
		err = rebaseForError(err, sm, lineStarts)
		var retryErr syntax.ParseError
		if errors.As(err, &retryErr) && int(retryErr.Pos.Offset()) <= errOffset {
			return nil, firstErr
		}
		return nil, err
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing select list positions: %w", name, err)
	}
	for _, site := range sites {
		if !verifySelectParenList(tree, site) {
			return nil, firstErr
		}
	}
	return tree, nil
}

// scanSelectParenSites finds every `select name ( word ... )` header in
// command position, in source order; the walk resumes after each list so
// its words are never sites themselves.
func scanSelectParenSites(src []byte) []selectParenSite {
	var sites []selectParenSite
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		if word != "select" || end >= len(src) || (src[end] != ' ' && src[end] != '\t') {
			return 0, false
		}
		site, ok := scanSelectParenSite(src, start)
		if !ok {
			return 0, false
		}
		sites = append(sites, site)
		return site.parenClose + 1, true
	})
	return sites
}

// hasSiteAt reports whether a site starts at offset: the parser's error sits
// on the first header it could not read, so that header must be one of the
// sites for the rewrite to be the fix.
func hasSiteAt(sites []selectParenSite, offset int) bool {
	for _, site := range sites {
		if site.start == offset {
			return true
		}
	}
	return false
}

// scanSelectParenSite reads the `select name ( word ... )` header whose
// `select` word is at start: an identifier name, blanks, and a `(` that is
// not `((`, with a list scanParenWordList accepts.
func scanSelectParenSite(src []byte, start int) (selectParenSite, bool) {
	nameStart := skipInlineSpaces(src, start+len("select"))
	nameEnd := nameStart
	for nameEnd < len(src) && isIdentByte(src[nameEnd]) {
		nameEnd++
	}
	if nameEnd == nameStart {
		return selectParenSite{}, false
	}
	parenOpen := skipInlineSpaces(src, nameEnd)
	if parenOpen >= len(src) || src[parenOpen] != '(' || (parenOpen+1 < len(src) && src[parenOpen+1] == '(') {
		return selectParenSite{}, false
	}
	parenClose, newlines, words, ok := scanParenWordList(src, parenOpen)
	if !ok {
		return selectParenSite{}, false
	}
	return selectParenSite{start: start, parenOpen: parenOpen, parenClose: parenClose - 1, newlines: newlines, words: words}, true
}

// verifySelectParenList reports whether tree holds a select loop at the
// site whose `in` is on the `(` byte and whose items are the list's words
// at their own offsets.
func verifySelectParenList(tree *syntax.File, site selectParenSite) bool {
	verified := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if verified {
			return false
		}
		loop, ok := node.(*syntax.ForClause)
		if !ok || !loop.Select || int(loop.ForPos.Offset()) != site.start {
			return true
		}
		iter, ok := loop.Loop.(*syntax.WordIter)
		if !ok || int(iter.InPos.Offset()) != site.parenOpen || len(iter.Items) != len(site.words) {
			return false
		}
		for i, item := range iter.Items {
			if int(item.Pos().Offset()) != site.words[i][0] || int(item.End().Offset()) != site.words[i][1] {
				return false
			}
		}
		verified = true
		return false
	})
	return verified
}
