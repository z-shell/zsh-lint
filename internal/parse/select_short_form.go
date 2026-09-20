package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// selectSite is one `select name [in word ...]` header in command position
// whose body is not a `do` list: native Zsh (par_for in parse.c) reads the
// name, skips newlines, reads either `in` and a word list that the first
// `;` or newline ends or the parenthesized list, skips any number of `;`
// and newlines, and then reads a `do` list, a `{ list }` or one sublist.
type selectSite struct {
	// start is the offset of the `select` word.
	start int
	// in reports an `in` word list; the term after it is the first
	// separator of the gap.
	in bool
	// gap holds the separators native Zsh skips before the body: after the
	// list's term in the `in` form, after the name otherwise.
	gap repeatGap
	// bodyStart is the offset of the body's first byte, or len(src).
	bodyStart int
}

// scanSelectSites finds every `select` header in command position whose
// body the parser cannot read on its own, in source order. A header the
// scan does not recognise, such as the parenthesized list form, is not a
// site.
func scanSelectSites(src []byte) []selectSite {
	var sites []selectSite
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		if word != "select" || end >= len(src) || (src[end] != ' ' && src[end] != '\t') {
			return 0, false
		}
		site, ok := scanSelectSite(src, start)
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

// scanSelectSite reads the header of the `select` loop at start. The word
// list is read with the parser's own word lexer, so a quoted or expanded
// word never ends it; the list ends at the first `;` or unescaped newline
// after a word, as native Zsh reads it. The parenthesized form, a name that
// is not an identifier, and a list the lexer rejects or that `&`, `|`, `)`
// or `;;` ends report ok false.
func scanSelectSite(src []byte, start int) (selectSite, bool) {
	nameStart := skipInlineSpaces(src, start+len("select"))
	nameEnd := nameStart
	for nameEnd < len(src) && isIdentByte(src[nameEnd]) {
		nameEnd++
	}
	if nameEnd == nameStart || (nameEnd < len(src) && !isRepeatWordEnd(src[nameEnd])) {
		return selectSite{}, false
	}
	site := selectSite{start: start}
	gap, next := scanRepeatGap(src, nameEnd)
	if next < len(src) && src[next] == '(' {
		return selectSite{}, false
	}
	if len(gap.semicolons) == 0 && matchSourceWord(src, next, "in") {
		term, ok := scanSelectWordList(src, next+len("in"))
		if !ok {
			return selectSite{}, false
		}
		site.in = true
		gapStart := term
		if src[term] == ';' {
			gapStart++
		}
		site.gap, site.bodyStart = scanRepeatGap(src, gapStart)
		return site, true
	}
	site.gap, site.bodyStart = gap, next
	return site, true
}

// scanSelectWordList returns the offset of the `;` or newline that ends the
// word list starting at from. The lexer reads across newlines, so the gap
// before each word, and the gap before the token that stops the lexer, is
// checked for a newline that is not a `\`-newline continuation; a comment
// in a gap runs to the newline that ends it.
func scanSelectWordList(src []byte, from int) (int, bool) {
	parser := syntax.NewParser(syntax.Variant(syntax.LangZsh))
	gapStart := from
	var err error
	for word, wordErr := range parser.WordsSeq(bytes.NewReader(src[from:])) {
		if wordErr != nil {
			err = wordErr
			break
		}
		wordStart := from + int(word.Pos().Offset())
		if term, ok := selectListNewline(src, gapStart, wordStart); ok {
			return term, true
		}
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
		return term, true
	}
	if err != nil && parseErr.Text == "`;` is not a valid word" && stop < len(src) && src[stop] == ';' {
		return stop, true
	}
	return -1, false
}

// selectListNewline returns the offset of the first newline in src[from:to]
// that is not a `\`-newline continuation. Blanks and a comment running to
// the newline are the only other bytes the lexer skips there.
func selectListNewline(src []byte, from, to int) (int, bool) {
	for i := from; i < to; i++ {
		switch src[i] {
		case ' ', '\t', '\r':
		case '\\':
			if i+1 < to && src[i+1] == '\n' {
				i++
			}
		case '\n':
			return i, true
		case '#':
			for i+1 < to && src[i+1] != '\n' {
				i++
			}
		}
	}
	return -1, false
}

// sublistStatements returns the statement that starts at offset in tree and
// the outermost statement of the sublist it begins: native Zsh reads a
// sublist as the whole `&&`, `||` and `|` chain, so a statement on the left
// of such an operator, or under `time`, widens to the statement that holds
// the chain. Both are nil when no statement starts at offset.
func sublistStatements(tree *syntax.File, offset int) (inner, outer *syntax.Stmt) {
	var stack []syntax.Node
	syntax.Walk(tree, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if inner != nil {
			return false
		}
		if stmt, ok := node.(*syntax.Stmt); ok && int(stmt.Pos().Offset()) == offset {
			inner, outer = stmt, stmt
			for index := len(stack) - 1; index > 0; index -= 2 {
				switch stack[index].(type) {
				case *syntax.BinaryCmd, *syntax.TimeClause:
				default:
					return false
				}
				parent, ok := stack[index-1].(*syntax.Stmt)
				if !ok {
					return false
				}
				outer = parent
			}
			return false
		}
		stack = append(stack, node)
		return true
	})
	return inner, outer
}

// sublistEndsAt reports whether a sublist may end at offset: the rest of
// the line is blank, a comment or a separator, or a closer follows. A
// closing keyword another adapter synthesized in the probe carries the
// keyword's length past the byte it maps to, so a `done` inserted at that
// end would split a word; this check declines such a body.
func sublistEndsAt(src []byte, offset int) bool {
	offset = skipInlineSpaces(src, offset)
	if offset >= len(src) {
		return true
	}
	switch src[offset] {
	case '\n', ';', '&', '|', '#', '}', ')':
		return true
	}
	return false
}

// parseSelectShortForm adapts `select name [in word ...] term sublist`, the
// short form of select (issue #212), where term is `;` or a newline. The
// parser requires `do` after the header and reports one of two errors at
// the `select` word depending on whether the header carried a separator.
// The body has no tree yet, so the retry is in two steps: a byte-preserving
// probe blanks the header of every unread site and parses, which yields the
// sublist statement at the body's first byte and, through the statement it
// widens to, the byte where the body ends; then a source-mapped retry
// inserts `do` and `done` around the body, and the tree is verified to
// hold a select loop at the site whose body starts at the first byte and
// ends at the closer before every position is rebased. The last unread
// site is rewritten first, so an enclosing site's probe sees the loop it
// contains with a real `done`, and each site costs one probe and one retry.
//
// A `{ list }` body, an empty body, and the parenthesized list form keep
// the parser error; they are other productions of the loop.
func parseSelectShortForm(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseSelectShortFormWithParser(src, name, firstErr, parseWithAdapters)
}

func parseSelectShortFormWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	if !strings.HasPrefix(parseErr.Text, "`select foo") || !strings.Contains(parseErr.Text, "must be followed by") {
		return nil, firstErr
	}
	errOffset := int(parseErr.Pos.Offset())
	if !matchSourceWord(src, errOffset, "select") {
		return nil, firstErr
	}
	// The parser stops at the first header it cannot read, so the sites
	// from the error on are the unread ones; a site before it parsed.
	var unread []selectSite
	for _, site := range scanSelectSites(src) {
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
		for i := blanked.start; i < blanked.bodyStart; i++ {
			if probe[i] != '\n' {
				probe[i] = ' '
			}
		}
	}
	tree, err := parse(probe, name)
	if err != nil {
		// Blanks raise no error of their own, so an error past the site is
		// the next real blocker, and the retry that runs outside the chain
		// needs its position.
		var probeErr syntax.ParseError
		if errors.As(err, &probeErr) && int(probeErr.Pos.Offset()) > errOffset {
			return nil, err
		}
		return nil, firstErr
	}
	inner, outer := sublistStatements(tree, site.bodyStart)
	if inner == nil {
		return nil, firstErr
	}
	if _, ok := inner.Cmd.(*syntax.Block); ok && !inner.Negated {
		return nil, firstErr
	}
	closer := repeatCloserOffset(outer, nil)
	if !sublistEndsAt(src, closer) {
		return nil, firstErr
	}
	// The words of an anonymous function invocation are blanks in the
	// source the retry outside the chain hands back, so blanks that run to
	// the end of the line go inside the loop and `done` follows the words.
	if end := skipSpaces(src, closer); end >= len(src) || src[end] == '\n' {
		closer = end
	}
	closer, ok := repeatSublistCloser(src, outer, closer)
	if !ok {
		return nil, firstErr
	}

	// The parser accepts one `;` and then newlines before `do`: every other
	// `;` becomes a blank, and a header with no separator at all gets one.
	opener := "do\n"
	redundant := site.gap.semicolons
	if !site.in {
		switch {
		case site.gap.newline:
		case len(redundant) > 0:
			redundant = redundant[1:]
		default:
			opener = "; do\n"
		}
	}
	var edits []repeatEdit
	for _, at := range redundant {
		edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
	}
	edits = append(edits,
		repeatEdit{start: site.bodyStart, end: site.bodyStart, text: opener},
		repeatEdit{start: closer, end: closer, text: "\ndone"},
	)
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
		return nil, fmt.Errorf("%s: rebasing select loop positions: %w", name, err)
	}
	if !verifySelectLoop(tree, site, closer) {
		return nil, firstErr
	}
	return tree, nil
}

// verifySelectLoop reports whether tree holds a select loop at the site
// whose `do` and first body statement are at the body's first byte and
// whose `done` is at the closer.
func verifySelectLoop(tree *syntax.File, site selectSite, closer int) bool {
	verified := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if verified {
			return false
		}
		loop, ok := node.(*syntax.ForClause)
		if !ok || !loop.Select || int(loop.ForPos.Offset()) != site.start {
			return true
		}
		verified = int(loop.DoPos.Offset()) == site.bodyStart &&
			len(loop.Do) > 0 &&
			int(loop.Do[0].Pos().Offset()) == site.bodyStart &&
			int(loop.DonePos.Offset()) == closer
		return false
	})
	return verified
}
