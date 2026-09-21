package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// whileEmptyBodyErrors are the exact parser errors a `while` or `until`
// header with no body produces. The parser requires `do` after the
// condition and reports at the keyword.
var whileEmptyBodyErrors = map[string]bool{
	"`while <cond>` must be followed by `do`": true,
	"`until <cond>` must be followed by `do`": true,
}

// whileEmptyBodyCloserErrors are the errors the parser gives instead when
// the empty body sits before a closer: the parser takes the closer itself
// as the header's body and then rejects it where it stands, so the error
// text names the closer and its position is at or after the body's byte,
// not at the keyword. The parser-front-end contract allows this position
// gate for a construct whose error depends on its body's first token,
// provided the retry's tree is verified to hold the expected node at each
// site, which verifyWhileEmptyBodies does.
var whileEmptyBodyCloserErrors = map[string]bool{
	"`}` can only be used to close a block": true,
	"`fi` can only be used to end an `if`":  true,
	"`done` can only be used to end a loop": true,
	"`esac` can only be used to end a case": true,
	"`then` can only be used in an `if`":    true,
	// A `;` between the empty body and the closer is rejected where it
	// stands rather than at the closer.
	"`;` can only immediately follow a statement": true,
}

// whileEmptyBodySite is one `while list` or `until list` header whose
// sublist body is empty (issue #327).
type whileEmptyBodySite struct {
	// start is the offset of the `while` or `until` word.
	start int
	// until reports an `until` loop.
	until bool
	// condStart is the first byte of the condition.
	condStart int
	// bodyStart is the byte native par_while reads the empty sublist at:
	// a closer, an operator that takes the loop as its left operand, or
	// the end of the file. Both `do` and `done` go there. It is resolved
	// from the probe tree, not from the bytes alone.
	bodyStart int
	// semicolons are the offsets of the `;` separators between the
	// condition and the body, and newline reports a newline among them.
	semicolons []int
	newline    bool
}

// parseWhileEmptyBody adapts a `while` or `until` header whose body is the
// empty sublist native Zsh reads at a closer, before a pipeline or list
// operator, or at the end of the file (issue #327): `while true`,
// `while true print x` (whose condition is the whole command `true print
// x`), `f() { while true }`. par_while in parse.c, when neither `do` nor
// `{` follows and SHORT_LOOPS is set, reads one sublist, and an empty
// sublist there is valid. The parser (mvdan/sh through v3.14.1) instead
// requires `do` and reports either at the keyword or, when a closer follows
// the header, at that closer.
//
// A condition's extent cannot be read from bytes alone, so the adapter
// first parses a byte-preserving probe in which every candidate site's
// keyword is blanked: each condition becomes an ordinary statement and the
// parser's own reading of it gives the byte the header ends on. The retry
// then inserts `do` and `done` at every confirmed site through one source
// map, and every site must come back as a WhileClause at its keyword with
// an empty Do, or the parser error stands.
//
// All sites in a file are resolved in one pass, so a file with many such
// loops costs two parses rather than two per site: re-entering the chain
// per site would make the cost exponential in the number of sites.
//
// This is the `while` and `until` half of the empty loop body; the `for`
// forms are handled in parseForShortForm and the `select` analogue in
// parseSelectShortForm, all three through selectEmptyBodyAt.
func parseWhileEmptyBody(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseWhileEmptyBodyWithParser(src, name, firstErr, parseWithAdapters)
}

func parseWhileEmptyBodyWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	if !whileEmptyBodyErrors[parseErr.Text] && !whileEmptyBodyCloserErrors[parseErr.Text] {
		return nil, firstErr
	}
	errOffset := int(parseErr.Pos.Offset())

	// Every `while` or `until` in command position is a candidate. The
	// error sits at the failing header's keyword, or at the closer the
	// parser took as its body, so a candidate that starts after the error
	// has not been read yet and a candidate at or before it may be the one
	// that failed; the probe decides which are real by whether their body
	// byte ends a list.
	candidates := scanWhileEmptyBodyCandidates(src)
	if len(candidates) == 0 {
		return nil, firstErr
	}

	// The keyword is blanked so each condition parses as an ordinary
	// statement; `while` and `until` are both five bytes, so every offset
	// is preserved. A `!` before one negates the pipeline the keyword
	// begins and would swallow the statement, so it is blanked too.
	probe := bytes.Clone(src)
	for _, site := range candidates {
		if bang, negated := negationBefore(src, site.start); negated {
			probe[bang] = ' '
		}
		copy(probe[site.start:], "     ")
	}
	tree, err := parse(probe, name)
	if err != nil {
		// Blanks raise no error of their own, so an error past the first
		// candidate is a real blocker elsewhere in the file.
		var probeErr syntax.ParseError
		if errors.As(err, &probeErr) && int(probeErr.Pos.Offset()) > errOffset {
			return nil, err
		}
		return nil, firstErr
	}

	condEnds := whileConditionEnds(tree)
	var sites []whileEmptyBodySite
	seedRecognized := false
	for _, site := range candidates {
		condEnd, ok := condEnds[site.condStart]
		if !ok || condEnd > len(src) {
			continue
		}
		// Native Zsh skips the separators between the condition and the
		// body before reading the sublist, so the empty body sits after
		// them.
		_, bodyStart := scanRepeatGap(src, condEnd)
		if !selectEmptyBodyAt(src, bodyStart) {
			// A body the probe read as a real statement: not this gap.
			continue
		}
		site.bodyStart = bodyStart
		site.semicolons, site.newline = whileHeaderSeparators(src, bodyStart)
		sites = append(sites, site)
		if site.start == errOffset || bodyStart <= errOffset {
			seedRecognized = true
		}
	}
	// The failing header must be one of the sites, or this is a different
	// defect and the parser error stands.
	if len(sites) == 0 || !seedRecognized {
		return nil, firstErr
	}

	var edits []repeatEdit
	for _, site := range sites {
		// The parser accepts one `;` and then newlines before `do`, so a
		// header that already carries a separator needs none, and every
		// `;` past the first becomes a blank.
		opener := "do\n"
		redundant := site.semicolons
		switch {
		case site.newline:
		case len(redundant) > 0:
			redundant = redundant[1:]
		default:
			opener = "; do\n"
		}
		for _, at := range redundant {
			edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
		}
		// `done` must not be glued to the closer that follows and must not
		// run into an operator's line.
		doneText := "done\n"
		if selectOperatorAt(src, site.bodyStart) {
			doneText = "done"
		}
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: opener},
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: doneText},
		)
	}

	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err = parse(transformed, name)
	if err != nil {
		err = rebaseForError(err, sm, lineStarts)
		var retryErr syntax.ParseError
		if errors.As(err, &retryErr) && int(retryErr.Pos.Offset()) <= sites[0].start {
			return nil, firstErr
		}
		return nil, err
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing while empty body positions: %w", name, err)
	}
	if !verifyWhileEmptyBodies(tree, sites) {
		return nil, firstErr
	}
	return tree, nil
}

// scanWhileEmptyBodyCandidates returns every `while` or `until` word in
// command position whose header is not already the delimited short form of
// issue #211, in source order. A short form's body is a real sublist on the
// same line, so blanking its keyword for the probe would leave the
// condition and that sublist as two statements with no separator between
// them and fail the probe for a construct this adapter does not own. A
// header whose body the probe reads as a real statement is dropped later;
// the scan itself only establishes that the word is a reserved word rather
// than an argument or quoted text.
func scanWhileEmptyBodyCandidates(src []byte) []whileEmptyBodySite {
	shortForms := make(map[int]bool)
	for _, site := range scanWhileShortFormSites(src) {
		shortForms[site.start] = true
	}
	doForms := scanWhileDoForms(src)
	var sites []whileEmptyBodySite
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		if word != "while" && word != "until" {
			return 0, false
		}
		if shortForms[start] || doForms[start] {
			return 0, false
		}
		condStart := skipInlineSpaces(src, end)
		if condStart >= len(src) {
			return 0, false
		}
		sites = append(sites, whileEmptyBodySite{
			start:     start,
			until:     word == "until",
			condStart: condStart,
		})
		return 0, false
	})
	return sites
}

// scanWhileDoForms reports the offset of every `while` or `until` word whose
// body is a `do ... done` list or a `{ ... }` block, which already parse and
// must keep their keyword: blanking it would orphan the `do` or the block and
// break a construct this adapter does not own.
//
// A keyword owns the next `do` in command position when no other reserved
// word that takes a `do` intervenes, so the scan tracks the innermost
// pending loop keyword and pairs it with the `do` that follows.
func scanWhileDoForms(src []byte) map[int]bool {
	forms := make(map[int]bool)
	var pending []int
	scanCommandWords(src, func(start, end int, word string) (int, bool) {
		switch word {
		case "while", "until", "for", "select", "repeat", "foreach":
			pending = append(pending, start)
		case "do":
			if len(pending) > 0 {
				forms[pending[len(pending)-1]] = true
				pending = pending[:len(pending)-1]
			}
		case "done":
			if len(pending) > 0 {
				pending = pending[:len(pending)-1]
			}
		}
		return 0, false
	})
	// A brace body follows the header directly, so the `{` that opens it is
	// the first syntactically active byte after the keyword's condition.
	// scanWhileShortFormSite already declines a `{`, and a brace-form loop
	// parses, so only the `do` pairing above is needed here.
	return forms
}

// whileHeaderSeparators reads the separators that precede the empty body at
// bodyStart back to the condition's last real byte, in source order. It
// reports the offset of every `;` among them and whether any newline is
// present. Reading backwards is what makes the `;` that terminated the
// condition statement visible: the statement's End() already covers it, so
// a forward gap scan from there would not see it and the retry would insert
// a second `;` that native Zsh rejects.
func whileHeaderSeparators(src []byte, bodyStart int) (semicolons []int, newline bool) {
	start := bodyStart
	for start > 0 {
		switch src[start-1] {
		case ' ', '\t', ';', '\n', '\r':
			start--
			continue
		}
		break
	}
	for i := start; i < bodyStart; i++ {
		switch src[i] {
		case ';':
			semicolons = append(semicolons, i)
		case '\n':
			newline = true
		}
	}
	return semicolons, newline
}

// whileConditionEnds maps each statement's start offset in the probe tree to
// the byte after the condition it would be: the statement widened through
// the `&&`, `||` and `time` chains that take it as an operand, since native
// Zsh reads the whole list as the condition. Building the map once keeps the
// all-sites pass to a single walk.
func whileConditionEnds(tree *syntax.File) map[int]int {
	parents := make(map[syntax.Node]syntax.Node)
	var stmts []*syntax.Stmt
	var stack []syntax.Node
	syntax.Walk(tree, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		if stmt, ok := node.(*syntax.Stmt); ok {
			stmts = append(stmts, stmt)
		}
		return true
	})
	ends := make(map[int]int, len(stmts))
	for _, stmt := range stmts {
		start := int(stmt.Pos().Offset())
		if _, seen := ends[start]; seen {
			continue
		}
		outer := stmt
		for {
			var next *syntax.Stmt
			switch parent := parents[outer].(type) {
			case *syntax.BinaryCmd:
				next, _ = parents[parent].(*syntax.Stmt)
			case *syntax.TimeClause:
				next, _ = parents[parent].(*syntax.Stmt)
			}
			if next == nil {
				break
			}
			outer = next
		}
		ends[start] = int(outer.End().Offset())
	}
	return ends
}

// verifyWhileEmptyBodies reports whether tree holds every site as a loop
// with an empty body: a WhileClause at the keyword, of the right kind, whose
// `do` and `done` are both the body's byte and whose Do holds no statement.
// The separator the retry inserted after each condition is synthetic and is
// cleared so no position points into text the source does not have.
func verifyWhileEmptyBodies(tree *syntax.File, sites []whileEmptyBodySite) bool {
	byStart := make(map[int]whileEmptyBodySite, len(sites))
	for _, site := range sites {
		byStart[site.start] = site
	}
	verified := 0
	ok := true
	syntax.Walk(tree, func(node syntax.Node) bool {
		if !ok {
			return false
		}
		clause, isClause := node.(*syntax.WhileClause)
		if !isClause {
			return true
		}
		site, isSite := byStart[int(clause.WhilePos.Offset())]
		if !isSite {
			return true
		}
		if clause.Until != site.until || len(clause.Cond) == 0 || len(clause.Do) != 0 ||
			int(clause.DoPos.Offset()) != site.bodyStart || int(clause.DonePos.Offset()) != site.bodyStart {
			ok = false
			return false
		}
		clause.Cond[len(clause.Cond)-1].Semicolon = syntax.Pos{}
		verified++
		return true
	})
	return ok && verified == len(sites)
}
