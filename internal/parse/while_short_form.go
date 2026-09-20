package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// whileShortFormSite is one `while list sublist` or `until list sublist`
// short form (issue #211): the `while` or `until` word, the first byte of
// its delimited condition, the blank between condition and sublist that
// the probe writes a `;` over, and the first byte of the sublist. closer
// is where the sublist ends, resolved from the probe tree.
type whileShortFormSite struct {
	start     int
	until     bool
	condStart int
	separator int
	bodyStart int
	closer    int
	doneText  string
}

// parseWhileShortForm adapts the `while list sublist` and `until list sublist`
// short forms of loops (issue #211): a condition delimited by `[[ ... ]]`,
// `(( ... ))`, `{ ... }` or `( ... )`, possibly negated or chained with
// `&&` and `||`, followed on the same line by one sublist instead of `do`.
// The parser reads the condition and then fails at the sublist's first byte
// because a separator is missing, which is the gate: the error text is
// exact and its position must be the sublist start of a site found by
// scanning the source.
//
// Native Zsh runs the sublist up to its terminator, over any `&&`, `||` and
// `|` chain and over a heredoc body, so the sublist's extent cannot be read
// from bytes alone. The adapter first parses a byte-preserving probe in
// which every site's keyword is blanked and a `;` is written over the blank
// before its sublist, so the condition and the sublist become two
// consecutive statements; the parser's own reading of the statement after
// the condition gives the sublist. Its last byte is found by scanning the
// source back from whatever follows the statement. The retry then inserts
// `; do\n` before each sublist and `done` after each sublist through a
// source map, and every site must come back as a WhileClause at the keyword
// whose Do holds exactly the sublist, or the parser error is returned. All
// sites in a file are handled in one pass so nested and repeated short forms
// cost two parses, not two per site.
func parseWhileShortForm(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseWhileShortFormWithParser(src, name, firstErr, parseWithAdapters)
}

func parseWhileShortFormWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != statementSeparatorRequired {
		return nil, firstErr
	}
	seed := int(parseErr.Pos.Offset())
	sites := scanWhileShortFormSites(src)
	seedRecognized := false
	for _, site := range sites {
		if site.bodyStart == seed {
			seedRecognized = true
			break
		}
	}
	if !seedRecognized {
		return nil, firstErr
	}

	probe := bytes.Clone(src)
	for _, site := range sites {
		if bang, ok := negationBefore(src, site.start); ok {
			probe[bang] = ' '
		}
		copy(probe[site.start:], "     ")
		probe[site.separator] = ';'
	}
	tree, err := parse(probe, name)
	if err != nil {
		if at, ok := errorOffset(err); ok && at > seed {
			return nil, err
		}
		return nil, firstErr
	}
	if !resolveWhileShortFormClosers(src, tree, sites) {
		return nil, firstErr
	}

	edits := make([]repeatEdit, 0, 2*len(sites))
	for index := len(sites) - 1; index >= 0; index-- {
		site := sites[index]
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: "; do\n"},
			repeatEdit{start: site.closer, end: site.closer, text: site.doneText},
		)
	}
	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err = parse(transformed, name)
	if err != nil {
		return nil, rebaseForError(err, sm, lineStarts)
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing while short form positions: %w", name, err)
	}
	if !bindWhileShortForms(tree, sites) {
		return nil, firstErr
	}
	return tree, nil
}

// scanWhileShortFormSites finds every `while` or `until` reserved word in
// command position whose delimited condition is followed on the same line
// by a sublist, in source order.
func scanWhileShortFormSites(src []byte) []whileShortFormSite {
	var sites []whileShortFormSite
	scanCommandWords(src, func(start, _ int, word string) (int, bool) {
		if word == "while" || word == "until" {
			if site, ok := scanWhileShortFormSite(src, start, word == "until"); ok {
				sites = append(sites, site)
			}
		}
		return 0, false
	})
	return sites
}

// scanWhileShortFormSite reads the condition after `while` or `until` at
// start: one or more delimited tests joined by `&&` or `||`, each
// optionally negated. The site is a short form only when the same line
// continues with a word that is neither `do`, a `{` (brace form), a
// separator, a comment nor a redirection, and only when a blank precedes
// that word for the probe to write its `;` over.
func scanWhileShortFormSite(src []byte, start int, until bool) (whileShortFormSite, bool) {
	site := whileShortFormSite{start: start, until: until}
	site.condStart = skipInlineSpaces(src, start+5)
	i := site.condStart
	for {
		for i+1 < len(src) && src[i] == '!' && (src[i+1] == ' ' || src[i+1] == '\t') {
			i = skipInlineSpaces(src, i+1)
		}
		end := scanDelimitedCondition(src, i)
		if end < 0 {
			return site, false
		}
		i = skipInlineSpaces(src, end)
		if i+1 < len(src) && ((src[i] == '&' && src[i+1] == '&') || (src[i] == '|' && src[i+1] == '|')) {
			i = skipAlternateConditionSpaces(src, i+2)
			continue
		}
		break
	}
	site.bodyStart = i
	if i >= len(src) {
		return site, false
	}
	switch src[i] {
	case '\n', '\r', ';', '&', '|', '#', ')', '}', '{', '<', '>':
		return site, false
	}
	if matchSourceWord(src, i, "do") {
		return site, false
	}
	switch {
	case src[i-1] == ' ' || src[i-1] == '\t':
		site.separator = i - 1
	case src[i-1] == '\n' && i >= 2 && src[i-2] == '\\':
		site.separator = i - 2
	default:
		return site, false
	}
	return site, true
}

// resolveWhileShortFormClosers sets each site's closer from the probe tree:
// the statement the parser read after the site's condition is the sublist,
// and the sublist ends after that statement's last byte, before the
// separators, blank lines and comment lines that lead to whatever follows it
// in the list.
func resolveWhileShortFormClosers(src []byte, tree *syntax.File, sites []whileShortFormSite) bool {
	parents := make(map[syntax.Node]syntax.Node)
	stmtAt := make(map[int]*syntax.Stmt)
	var comments []*syntax.Comment
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
		switch node := node.(type) {
		case *syntax.Stmt:
			if _, seen := stmtAt[int(node.Pos().Offset())]; !seen {
				stmtAt[int(node.Pos().Offset())] = node
			}
		case *syntax.Comment:
			comments = append(comments, node)
		}
		return true
	})
	for index := len(sites) - 1; index >= 0; index-- {
		site := &sites[index]
		cond := stmtAt[site.condStart]
		if cond == nil {
			return false
		}
		outer := cond
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
		body := followingStmt(parents[outer], outer)
		if body == nil {
			return false
		}
		if int(body.Pos().Offset()) != site.bodyStart && !nestedWhileShortFormAt(sites[index+1:], site.bodyStart, int(body.Pos().Offset())) {
			return false
		}
		anchor := ifShortFormAnchor(src, parents, body)
		if anchor < 0 {
			return false
		}
		for _, later := range sites[index+1:] {
			if later.condStart == anchor {
				anchor = later.start
			}
		}
		closer := ifShortFormCloser(src, int(body.Pos().Offset()), anchor, comments)
		for _, nested := range sites[index+1:] {
			if nested.start >= site.bodyStart && nested.start < closer {
				closer = max(closer, nested.closer)
			}
		}
		doneText := "; done"
		if closer == len(src) || src[closer] == '\n' {
			doneText = "\ndone"
		}
		if !heredocBodiesAllow(src, body, closer) {
			return false
		}
		site.closer = closer
		site.doneText = doneText
	}
	return true
}

// nestedWhileShortFormAt reports whether a site is at bodyStart with its
// condition at condStart.
func nestedWhileShortFormAt(sites []whileShortFormSite, bodyStart, condStart int) bool {
	for _, site := range sites {
		if site.start == bodyStart && site.condStart == condStart {
			return true
		}
	}
	return false
}

// bindWhileShortForms verifies that the rebased retry tree holds every site
// as a WhileClause at the keyword whose `do` and `done` map to the sublist's
// bounds and whose Do is the one sublist statement, and clears the synthetic
// separator after the condition.
func bindWhileShortForms(tree *syntax.File, sites []whileShortFormSite) bool {
	bound := 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		clause, ok := node.(*syntax.WhileClause)
		if !ok {
			return true
		}
		for _, site := range sites {
			if int(clause.WhilePos.Offset()) != site.start {
				continue
			}
			if clause.Until != site.until || int(clause.DoPos.Offset()) != site.bodyStart ||
				int(clause.DonePos.Offset()) != site.closer || len(clause.Cond) == 0 ||
				len(clause.Do) != 1 || int(clause.Do[0].Pos().Offset()) != site.bodyStart {
				return false
			}
			clause.Cond[len(clause.Cond)-1].Semicolon = syntax.Pos{}
			bound++
		}
		return true
	})
	return bound == len(sites)
}
