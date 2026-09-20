package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"

	"mvdan.cc/sh/v3/syntax"
)

// ifShortFormSite is one `if list sublist` short form (issue #210): the `if`
// word, the first byte of its delimited condition, the blank between the
// condition and the sublist that the probe writes a `;` over, and the first
// byte of the sublist. closer is where the sublist ends, resolved from the
// probe tree.
type ifShortFormSite struct {
	start     int
	condStart int
	separator int
	bodyStart int
	closer    int
	// fiText is the text the retry inserts at closer: `fi` on its own line
	// when the sublist ends its line, so that a heredoc delimiter or a
	// trailing comment there stays intact, else `; fi` on the same line,
	// which keeps a `fi` before the body of a heredoc the sublist opens.
	fiText string
}

// parseIfShortForm adapts the `if list sublist` short form of the alternate
// if (issue #210): a condition delimited by `[[ ... ]]`, `(( ... ))`,
// `{ ... }` or `( ... )`, possibly negated or chained with `&&` and `||`,
// followed on the same line by one sublist instead of `then`. The parser
// reads the condition and then fails at the sublist's first byte because a
// separator is missing, which is the gate: the error text is exact and its
// position must be the sublist start of a site found by scanning the source.
//
// Native Zsh runs the sublist up to its terminator, over any `&&`, `||` and
// `|` chain and over a heredoc body, so the sublist's extent cannot be read
// from bytes alone. The adapter first parses a byte-preserving probe in which
// every site's `if` is blanked and a `;` is written over the blank before
// its sublist, so the condition and the sublist become two consecutive
// statements of whatever list holds the `if`; the parser's own reading of
// the statement after the condition gives the sublist. Its last byte is
// found by scanning the source back from whatever follows the statement,
// not from the statement's End(): a synthetic closing keyword another
// adapter mapped into the file (a `fi` at a `}`, a `done` at a sublist end)
// makes End() overshoot the source by the keyword's length. The retry
// then inserts `; then` after each condition and `fi` after each sublist
// through a source map, and every site must come back as an IfClause at the
// `if` word whose Then holds exactly the sublist, or the parser error is
// returned. All sites in a file are handled in one pass so nested and
// repeated short forms cost two parses, not two per site.
func parseIfShortForm(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseIfShortFormWithParser(src, name, firstErr, parseWithAdapters)
}

func parseIfShortFormWithParser(
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
	sites := scanIfShortFormSites(src)
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
		copy(probe[site.start:], "  ")
		probe[site.separator] = ';'
	}
	tree, err := parse(probe, name)
	if err != nil {
		// The probe keeps every byte in place, so an error past the seed
		// names the gap that remains once the short forms are read as
		// statements; an error at or before it means a site was misread.
		if at, ok := errorOffset(err); ok && at > seed {
			return nil, err
		}
		return nil, firstErr
	}
	if !resolveIfShortFormClosers(src, tree, sites) {
		return nil, firstErr
	}

	// Later sites first, so the `fi` of a nested short form lands before
	// the `fi` of the one enclosing it when both close at the same byte.
	edits := make([]repeatEdit, 0, 2*len(sites))
	for index := len(sites) - 1; index >= 0; index-- {
		site := sites[index]
		edits = append(edits,
			repeatEdit{start: site.bodyStart, end: site.bodyStart, text: "; then\n"},
			repeatEdit{start: site.closer, end: site.closer, text: site.fiText},
		)
	}
	transformed, sm := applyRepeatEdits(src, edits)
	lineStarts := originalLineStarts(src)
	tree, err = parse(transformed, name)
	if err != nil {
		return nil, rebaseForError(err, sm, lineStarts)
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, fmt.Errorf("%s: rebasing if short form positions: %w", name, err)
	}
	if !bindIfShortForms(tree, sites) {
		return nil, firstErr
	}
	return tree, nil
}

// errorOffset returns the source offset a parser error points at.
func errorOffset(err error) (int, bool) {
	var parseErr syntax.ParseError
	if errors.As(err, &parseErr) && parseErr.Pos.IsValid() {
		return int(parseErr.Pos.Offset()), true
	}
	var langErr syntax.LangError
	if errors.As(err, &langErr) && langErr.Pos.IsValid() {
		return int(langErr.Pos.Offset()), true
	}
	return 0, false
}

// scanIfShortFormSites finds every `if` reserved word in command position
// whose delimited condition is followed on the same line by a sublist, in
// source order.
func scanIfShortFormSites(src []byte) []ifShortFormSite {
	var sites []ifShortFormSite
	scanCommandWords(src, func(start, _ int, word string) (int, bool) {
		if word == "if" {
			if site, ok := scanIfShortFormSite(src, start); ok {
				sites = append(sites, site)
			}
		}
		// The condition and the sublist are commands too, so scanning
		// carries on through them; `if` keeps command position.
		return 0, false
	})
	return sites
}

// scanIfShortFormSite reads the condition after the `if` at start: one or
// more delimited tests joined by `&&` or `||`, each optionally negated. The
// site is a short form only when the same line continues with a word that
// is neither `then`, a `{` (the brace form, handled by parseAlternateIfBrace),
// a separator, a comment nor a redirection, and only when a blank precedes
// that word for the probe to write its `;` over.
func scanIfShortFormSite(src []byte, start int) (ifShortFormSite, bool) {
	site := ifShortFormSite{start: start}
	site.condStart = skipInlineSpaces(src, start+len("if"))
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
	if matchSourceWord(src, i, "then") {
		return site, false
	}
	switch {
	case src[i-1] == ' ' || src[i-1] == '\t':
		site.separator = i - 1
	case src[i-1] == '\n' && i >= 2 && src[i-2] == '\\':
		// A `\`-newline before the sublist: the `;` goes over the
		// backslash, and the newline it escaped then ends the condition.
		site.separator = i - 2
	default:
		return site, false
	}
	return site, true
}

// scanDelimitedCondition returns the offset just past the delimited test
// that starts at i, or -1 when no suitably delimited test starts there.
func scanDelimitedCondition(src []byte, i int) int {
	if i >= len(src) {
		return -1
	}
	switch {
	case src[i] == '[' && i+1 < len(src) && src[i+1] == '[':
		return scanClosingDoubleBracket(src, i)
	case src[i] == '(' && i+1 < len(src) && src[i+1] == '(':
		return scanClosingDoubleParen(src, i)
	case src[i] == '{' && i+1 < len(src) && isRepeatLitEnd(src[i+1]):
		return scanClosingBrace(src, i)
	case src[i] == '(':
		return scanClosingParen(src, i)
	}
	return -1
}

// scanClosingParen returns the offset just past the `)` that balances the
// `(` at start, or -1.
func scanClosingParen(src []byte, start int) int {
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// resolveIfShortFormClosers sets each site's closer from the probe tree:
// the statement the parser read after the site's condition is the sublist,
// and the sublist ends after that statement's last byte, before the
// separators, blank lines and comment lines that lead to whatever follows it
// in the list. A comment on the closer's own line stays with the sublist. A
// short form nested in the sublist extends it to the nested closer. Any site
// whose condition or sublist the probe did not read as expected makes the
// whole resolution fail.
func resolveIfShortFormClosers(src []byte, tree *syntax.File, sites []ifShortFormSite) bool {
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
			// The first statement seen at an offset is the outermost: the
			// whole `&&`, `||` or `|` chain rather than its first command.
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
		if int(body.Pos().Offset()) != site.bodyStart && !nestedIfShortFormAt(sites[index+1:], site.bodyStart, int(body.Pos().Offset())) {
			return false
		}
		// mvdan/sh reads a bare `else` in command position as a command
		// name, so `if (( 0 )) print a; else print b` and `if (( 0 )) print
		// a || else print b`, which native Zsh rejects, would otherwise parse
		// as an if followed by, or containing, a call named else.
		if following := followingStmt(parents[body], body); following != nil && callsBareElse(following) {
			return false
		}
		if callsBareElse(body) {
			return false
		}
		anchor := ifShortFormAnchor(src, parents, body)
		if anchor < 0 {
			return false
		}
		// A following statement that is the condition of a later site sits
		// after that site's `if`, which the probe blanked and src still has.
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
		fiText := "; fi"
		if closer == len(src) || src[closer] == '\n' {
			fiText = "\nfi"
		}
		if !heredocBodiesAllow(src, body, closer) {
			return false
		}
		site.closer = closer
		site.fiText = fiText
	}
	return true
}

// callsBareElse reports whether stmt is, or contains in its `&&`, `||` and
// `|` chain, a call whose command name is the bare word `else`.
func callsBareElse(stmt *syntax.Stmt) bool {
	found := false
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if call, ok := node.(*syntax.CallExpr); ok && len(call.Args) > 0 {
			if word, ok := singleLiteral(call.Args[0]); ok && word == "else" {
				found = true
			}
		}
		return !found
	})
	return found
}

// nestedIfShortFormAt reports whether a site's `if` is at bodyStart with its
// condition at condStart: the probe blanked that `if`, so the statement read
// after an enclosing condition is the nested condition rather than the
// nested `if`.
func nestedIfShortFormAt(sites []ifShortFormSite, bodyStart, condStart int) bool {
	for _, site := range sites {
		if site.start == bodyStart && site.condStart == condStart {
			return true
		}
	}
	return false
}

// ifShortFormAnchor returns the offset of the first byte after the sublist
// statement that belongs to something else: the next statement of the list
// that holds it, or the token that closes that list. The parent's positions
// may themselves be the source-mapped result of another adapter, which is
// fine: the anchor only bounds the backward scan for the sublist's last
// byte.
func ifShortFormAnchor(src []byte, parents map[syntax.Node]syntax.Node, stmt *syntax.Stmt) int {
	parent := parents[stmt]
	if next := followingStmt(parent, stmt); next != nil {
		return int(next.Pos().Offset())
	}
	inList := func(list []*syntax.Stmt) bool {
		for _, candidate := range list {
			if candidate == stmt {
				return true
			}
		}
		return false
	}
	switch parent := parent.(type) {
	case *syntax.File:
		return len(src)
	case *syntax.Block:
		return int(parent.Rbrace.Offset())
	case *syntax.Subshell:
		return int(parent.Rparen.Offset())
	case *syntax.CmdSubst:
		return int(parent.Right.Offset())
	case *syntax.ProcSubst:
		return int(parent.Rparen.Offset())
	case *syntax.CaseItem:
		if parent.OpPos.IsValid() {
			return int(parent.OpPos.Offset())
		}
		if clause, ok := parents[parent].(*syntax.CaseClause); ok {
			return int(clause.Esac.Offset())
		}
	case *syntax.IfClause:
		if inList(parent.Cond) {
			return int(parent.ThenPos.Offset())
		}
		if parent.Else != nil {
			return int(parent.Else.Position.Offset())
		}
		return int(parent.FiPos.Offset())
	case *syntax.WhileClause:
		if inList(parent.Cond) {
			return int(parent.DoPos.Offset())
		}
		return int(parent.DonePos.Offset())
	case *syntax.ForClause:
		return int(parent.DonePos.Offset())
	}
	return -1
}

// ifShortFormCloser returns the offset after the last byte of the sublist
// statement starting at start and bounded by anchor: blanks, newlines,
// separators and whole comments before the anchor are not part of it, but a
// comment on the same line as the last byte is kept, so it stays attached
// to the statement it trails.
func ifShortFormCloser(src []byte, start, anchor int, comments []*syntax.Comment) int {
	end := anchor
scan:
	for end > start {
		switch src[end-1] {
		case ' ', '\t', '\n', '\r', ';', '&':
			end--
			continue
		}
		for _, comment := range comments {
			if at := int(comment.Pos().Offset()); at < end && end <= int(comment.End().Offset()) && at >= start {
				end = at
				continue scan
			}
		}
		break
	}
	trailing := end
	for trailing < anchor && (src[trailing] == ' ' || src[trailing] == '\t') {
		trailing++
	}
	if trailing < anchor && src[trailing] == '#' {
		for _, comment := range comments {
			if int(comment.Pos().Offset()) == trailing {
				return int(comment.End().Offset())
			}
		}
	}
	return end
}

// heredocBodiesAllow reports whether a `fi` inserted at closer stays out of
// every heredoc body in stmt: either the body starts before closer, so the
// closer follows its delimiter line, or the closer is still on the line of
// the redirection that opens it, so `; fi` precedes the body. A heredoc with
// no body has no node to place it by.
func heredocBodiesAllow(src []byte, stmt *syntax.Stmt, closer int) bool {
	ok := true
	syntax.Walk(stmt, func(node syntax.Node) bool {
		redirect, isRedirect := node.(*syntax.Redirect)
		if !isRedirect || (redirect.Op != syntax.Hdoc && redirect.Op != syntax.DashHdoc) {
			return ok
		}
		switch {
		case redirect.Hdoc == nil:
			ok = false
		case int(redirect.Hdoc.Pos().Offset()) <= closer:
		case bytes.IndexByte(src[redirect.Word.End().Offset():closer], '\n') >= 0:
			ok = false
		}
		return ok
	})
	return ok
}

// bindIfShortForms verifies that the rebased retry tree holds every site as
// an IfClause at the `if` word whose `then` and `fi` map to the sublist's
// bounds and whose Then is the one sublist statement, and clears the
// separator the retry wrote after each condition, so the tree carries no
// synthetic byte.
func bindIfShortForms(tree *syntax.File, sites []ifShortFormSite) bool {
	bound := 0
	syntax.Walk(tree, func(node syntax.Node) bool {
		clause, ok := node.(*syntax.IfClause)
		if !ok {
			return true
		}
		for _, site := range sites {
			if int(clause.Position.Offset()) != site.start {
				continue
			}
			if int(clause.ThenPos.Offset()) != site.bodyStart || int(clause.FiPos.Offset()) != site.closer ||
				clause.Else != nil || len(clause.Cond) == 0 || len(clause.Then) != 1 ||
				int(clause.Then[0].Pos().Offset()) != site.bodyStart {
				return false
			}
			clause.Cond[len(clause.Cond)-1].Semicolon = syntax.Pos{}
			bound++
		}
		return true
	})
	return bound == len(sites)
}
