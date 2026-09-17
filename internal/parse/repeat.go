package parse

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"mvdan.cc/sh/v3/syntax"
)

// RepeatLoop is a native `repeat count sublist` loop (issue #208). mvdan/sh
// (through v3.14.1) has no node for it and reads `repeat` as an ordinary
// command name, so the compatibility front end rewrites every loop into the
// closest typed shape, a WhileClause positioned at the `repeat` word whose
// single condition statement is the count word, and records it here. The
// loop body is Loop.Do. Consumers that distinguish a repeat loop from a
// while loop must inspect this metadata rather than the tree alone.
type RepeatLoop struct {
	Loop  *syntax.WhileClause
	Count *syntax.Word
}

// repeatShapeError is reported for a `repeat` word in command position that
// native Zsh rejects (a missing count, `repeat 3 do ...` at end of file, a
// count followed by `&`) or that the front end declines to rewrite, such as
// a sublist whose heredoc body extends past the end of the loop.
const repeatShapeError = "`repeat` loop has a shape the front end does not recognise (z-shell/zsh-lint#208)"

// repeatSite is one `repeat` reserved word in command position in the
// source, with the extent of its count word and the gap up to the body.
type repeatSite struct {
	start      int
	countStart int
	countEnd   int
	gap        repeatGap
	bodyStart  int
}

// repeatGap describes what native Zsh skips between the count word and the
// loop body: any number of `;` and newlines, plus the blanks, comments and
// `\`-newline continuations the lexer drops. The rewritten `while` accepts
// one `;` or a newline before `do`, not both and not two `;`, so a retry
// blanks the `;` bytes the parser would reject and a `do` on the same line
// as the count needs a `;` written over the last blank before it.
type repeatGap struct {
	newline    bool
	semicolons []int
	lastBlank  int
}

// scanRepeatGap classifies the bytes after a count word at from and returns
// the offset of the first body byte, or len(src) when nothing follows.
func scanRepeatGap(src []byte, from int) (repeatGap, int) {
	gap := repeatGap{lastBlank: -1}
	for i := from; i < len(src); i++ {
		switch src[i] {
		case ' ', '\t':
			gap.lastBlank = i
		case '\n':
			gap.newline = true
		case '\\':
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
				continue
			}
			return gap, i
		case '#':
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case ';':
			if i+1 < len(src) && src[i+1] == ';' {
				return gap, i
			}
			gap.semicolons = append(gap.semicolons, i)
		default:
			return gap, i
		}
	}
	return gap, len(src)
}

// repeatBody is the body form the front end must be helped to read.
type repeatBody int

const (
	// repeatBodyOther is a sublist body, or an empty body, which the parser
	// reads as ordinary words and statements without help.
	repeatBodyOther repeatBody = iota
	// repeatBodyDo is `do list done`, which the parser rejects after a
	// command name.
	repeatBodyDo
	// repeatBodyBrace is `{ list }` as a word of its own, which the parser
	// reads as a block only after a separator.
	repeatBodyBrace
)

// repeatBodyForm mirrors the parser's own token rule: `do` is the reserved
// word only as a whole word, and `{` opens a block only when the byte after
// it ends a literal word.
func repeatBodyForm(src []byte, at int) repeatBody {
	if matchSourceWord(src, at, "do") {
		return repeatBodyDo
	}
	if at < len(src) && src[at] == '{' && (at+1 == len(src) || isRepeatLitEnd(src[at+1])) {
		return repeatBodyBrace
	}
	return repeatBodyOther
}

// isRepeatLitEnd reports whether b ends a literal word in the parser's
// lexer, so that a `{` before it is a block opener rather than part of a
// word such as `{print`.
func isRepeatLitEnd(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '&', '|', ';', ')', '(':
		return true
	}
	return false
}

// repeatCommandPrefixWords are the reserved words after which the next word
// is still in command position, so `repeat` there is the reserved word.
var repeatCommandPrefixWords = map[string]bool{
	"then":  true,
	"else":  true,
	"do":    true,
	"if":    true,
	"elif":  true,
	"while": true,
	"until": true,
	"time":  true,
}

// isRepeatWordBoundary reports whether b separates words and leaves the
// next one in a position where a reserved word is recognised.
func isRepeatWordBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', '(', '`':
		return true
	}
	return false
}

// isRepeatWordEnd reports whether b ends a command word.
func isRepeatWordEnd(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', ')', '(', '`', '<', '>':
		return true
	}
	return false
}

// scanRepeatSites finds every `repeat` reserved word in command position in
// syntactically active source. Native Zsh recognises the word at the start
// of a command: after a separator, an opening `(` or `{`, a case pattern's
// `)`, a `!`, or a reserved word that itself precedes a command. Quoted
// text, comments, heredoc bodies and arithmetic never hold a site. The
// count word's extent comes from the parser's own word lexer, so a count
// such as `$(cat n)` or `"$n"` is one word however it is written.
func scanRepeatSites(src []byte) []repeatSite {
	var sites []repeatSite
	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false
	inBacktick := false
	var heredocs []pendingHeredoc
	arithmeticDepth := 0
	// parens records, for each open `(`, whether it opened a subshell or a
	// command substitution, whose `)` ends a command, rather than a case
	// pattern, an array value or a glob group, whose `)` may precede one.
	var parens []bool
	atCommandStart := true

	for i := 0; i < len(src); i++ {
		b := src[i]
		if escaped {
			escaped = false
			continue
		}
		if inSingleQuote {
			if b == '\'' {
				inSingleQuote = false
			}
			continue
		}
		if inDoubleQuote {
			switch b {
			case '\\':
				escaped = true
			case '"':
				inDoubleQuote = false
			}
			continue
		}
		if inANSICQuote {
			switch b {
			case '\\':
				escaped = true
			case '\'':
				inANSICQuote = false
			}
			continue
		}
		if inComment {
			if b == '\n' {
				inComment = false
				atCommandStart = true
			}
			continue
		}
		if b == '\n' && len(heredocs) > 0 {
			next, ok := consumeHeredocBodies(src, i+1, heredocs)
			if !ok {
				return sites
			}
			heredocs = nil
			i = next - 1
			atCommandStart = true
			continue
		}
		if b == '(' && i+1 < len(src) && src[i+1] == '(' {
			arithmeticDepth++
			i++
			continue
		}
		if arithmeticDepth > 0 {
			if b == ')' && i+1 < len(src) && src[i+1] == ')' {
				arithmeticDepth--
				i++
			}
			continue
		}

		switch b {
		case '\\':
			escaped = true
			if i+1 >= len(src) || src[i+1] != '\n' {
				atCommandStart = false
			}
			continue
		case '\'':
			inSingleQuote = true
			atCommandStart = false
			continue
		case '"':
			inDoubleQuote = true
			atCommandStart = false
			continue
		case '#':
			if i == 0 || isRepeatWordBoundary(src[i-1]) {
				inComment = true
			} else {
				atCommandStart = false
			}
			continue
		case '$':
			if i+1 < len(src) {
				switch src[i+1] {
				case '\'':
					inANSICQuote = true
					i++
					atCommandStart = false
					continue
				case '{':
					i++
					atCommandStart = false
					continue
				case '(':
					if i+2 < len(src) && src[i+2] == '(' {
						arithmeticDepth++
						i += 2
						continue
					}
					parens = append(parens, true)
					i++
					atCommandStart = true
					continue
				}
			}
			atCommandStart = false
			continue
		case ' ', '\t':
			continue
		case '\n', ';', '&', '|', '{':
			atCommandStart = true
			continue
		case '}':
			atCommandStart = false
			continue
		case '(':
			// An array value `name=(...)` holds words, not commands; any
			// other `(` may open a subshell, so its first word is a site.
			if i > 0 && src[i-1] == '=' {
				parens = append(parens, false)
				atCommandStart = false
				continue
			}
			parens = append(parens, atCommandStart)
			atCommandStart = true
			continue
		case ')':
			endsCommand := false
			if len(parens) > 0 {
				endsCommand = parens[len(parens)-1]
				parens = parens[:len(parens)-1]
			}
			atCommandStart = !endsCommand
			continue
		case '`':
			inBacktick = !inBacktick
			atCommandStart = inBacktick
			continue
		case '!':
			if i+1 >= len(src) || (src[i+1] != ' ' && src[i+1] != '\t') {
				atCommandStart = false
			}
			continue
		case '<', '>':
			if b == '<' && i+1 < len(src) && src[i+1] == '<' {
				if i+2 < len(src) && src[i+2] == '<' {
					i += 2
					atCommandStart = false
					continue
				}
				stripTabs := i+2 < len(src) && src[i+2] == '-'
				delimiterStart := i + 2
				if stripTabs {
					delimiterStart++
				}
				delimiter, end, ok := parseHeredocDelimiter(src, delimiterStart)
				if !ok {
					return sites
				}
				heredocs = append(heredocs, pendingHeredoc{
					delimiter: delimiter,
					stripTabs: stripTabs,
				})
				i = end - 1
				atCommandStart = false
				continue
			}
			if i+1 < len(src) && src[i+1] == '&' {
				i++
			}
			atCommandStart = false
			continue
		}
		if !isIdentByte(b) {
			atCommandStart = false
			continue
		}
		start := i
		for i+1 < len(src) && isIdentByte(src[i+1]) {
			i++
		}
		word := string(src[start : i+1])
		glued := start > 0 && !isRepeatWordBoundary(src[start-1])
		followed := i+1 < len(src) && !isRepeatWordEnd(src[i+1])
		switch {
		case glued || followed:
			atCommandStart = false
		case atCommandStart && word == "repeat" && i+1 < len(src) && (src[i+1] == ' ' || src[i+1] == '\t'):
			site, ok := scanRepeatSite(src, start)
			if !ok {
				atCommandStart = false
				continue
			}
			sites = append(sites, site)
			i = site.countEnd - 1
			atCommandStart = true
		case atCommandStart && repeatCommandPrefixWords[word]:
		default:
			atCommandStart = false
		}
	}
	return sites
}

// scanRepeatSite reads the count word after the `repeat` at start with the
// parser's word lexer and classifies the gap after it.
func scanRepeatSite(src []byte, start int) (repeatSite, bool) {
	countStart := skipInlineSpaces(src, start+len("repeat"))
	if countStart >= len(src) {
		return repeatSite{}, false
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangZsh))
	countEnd := -1
	for word, err := range parser.WordsSeq(bytes.NewReader(src[countStart:])) {
		if err == nil && word.Pos().Offset() == 0 {
			countEnd = countStart + int(word.End().Offset())
		}
		break
	}
	if countEnd <= countStart {
		return repeatSite{}, false
	}
	site := repeatSite{start: start, countStart: countStart, countEnd: countEnd}
	site.gap, site.bodyStart = scanRepeatGap(src, countEnd)
	return site, true
}

// repeatSiteEdits are the byte-preserving overwrites that let the parser
// read one site: `repeat` becomes `while `, a `do` on the count's line gets
// the `;` the parser needs, a `{` on the count's line gets the `;` that
// makes it a block, and `;` bytes a newline makes redundant become blanks.
type repeatSiteEdits struct {
	while       bool
	semicolonAt int
	blanks      []int
}

// repeatEditsForSite returns the overwrites for site, or false when the
// parser already reads the site's shape on its own, so the error must have
// another cause, or when no blank byte can carry the separator.
//
// A sublist on the count's line gets the `;` only when the error is inside
// it: the parser reads a simple command there as arguments of `repeat`, but
// a compound command (`(( ... ))`, `if`, `for`, `case`, a function
// definition) fails at its first reserved token, and the `;` lets the
// parser read it as the statement after the count.
func repeatEditsForSite(src []byte, site repeatSite, errOffset int) (repeatSiteEdits, bool) {
	edits := repeatSiteEdits{semicolonAt: -1}
	gap := site.gap
	form := repeatBodyForm(src, site.bodyStart)
	edits.while = form == repeatBodyDo
	switch {
	case gap.newline:
		edits.blanks = gap.semicolons
	case len(gap.semicolons) > 1:
		edits.blanks = gap.semicolons[1:]
	case len(gap.semicolons) == 0 && (form != repeatBodyOther || errOffset >= site.bodyStart):
		if gap.lastBlank < 0 {
			return edits, false
		}
		edits.semicolonAt = gap.lastBlank
	}
	if !edits.while && edits.semicolonAt < 0 && len(edits.blanks) == 0 {
		return edits, false
	}
	return edits, true
}

// parseRepeat adapts the `repeat count sublist` loop (issue #208). The
// parser reads `repeat` as a command name, so a `do` or `{` body fails with
// an error whose text depends on the body's first token: `do` can only be
// used in a loop, `then` can only be used in an if, `}` can only be used to
// close a block, and so on. The gate is therefore positional: the error
// must sit at or after a `repeat` site in command position. The retry
// overwrites bytes in place and is verified afterwards: the site must have
// become a `while` loop whose only condition is the count word, or, when
// only separators were blanked, must still be the `repeat` call. Any other
// outcome returns the parser error.
//
// The retry handles the do form, the separator shapes the parser rejects,
// and a sublist on the count's line that starts with a compound command;
// a sublist of simple commands, the brace form after a separator, and an
// empty body parse as ordinary statements. Every form is rewritten into the
// loop by resolveRepeatLoops once the whole file parses.
func parseRepeat(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseRepeatWithParser(src, name, firstErr, parseWithAdapters)
}

func parseRepeatWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	errOffset := int(parseErr.Pos.Offset())
	if !bytes.Contains(src[:min(len(src), errOffset+len("repeat"))], []byte("repeat")) {
		return nil, firstErr
	}
	// The chain hands an error it could not place to this adapter at every
	// level of its recursion, so the error may lie far past where the
	// parser stops on this level's source: an inner level, whose source
	// carries one more mask, already retried the site and carried the
	// error past it. A site the plain parse never reaches cannot be what
	// blocks this level, and its retry would repeat the inner level's whole
	// work. The plain parse fails at that earlier point quickly.
	if _, plainErr := parseTree(src, name); plainErr != nil {
		var plainParseErr syntax.ParseError
		if errors.As(plainErr, &plainParseErr) {
			errOffset = min(errOffset, int(plainParseErr.Pos.Offset()))
		}
	}
	if !bytes.Contains(src[:min(len(src), errOffset+len("repeat"))], []byte("repeat")) {
		return nil, firstErr
	}
	sites := scanRepeatSites(src)
	// The innermost site before the error is the one whose body the
	// error is in. A site the parser already reads on its own is skipped
	// so that an outer do form enclosing a sublist form is still found.
	for index := len(sites) - 1; index >= 0; index-- {
		site := sites[index]
		if site.start > errOffset {
			continue
		}
		edits, ok := repeatEditsForSite(src, site, errOffset)
		if !ok {
			continue
		}
		masked := bytes.Clone(src)
		if edits.while {
			copy(masked[site.start:], "while ")
		}
		if edits.semicolonAt >= 0 {
			masked[edits.semicolonAt] = ';'
		}
		for _, at := range edits.blanks {
			masked[at] = ' '
		}
		tree, err := parse(masked, name)
		if err != nil {
			// The overwrite is byte-preserving, so a retry error that did
			// not move past the first one is not this site's: either the
			// word was not the reserved word (a `repeat` inside a word
			// list in a subshell) or the error has another cause.
			var retryErr syntax.ParseError
			if errors.As(err, &retryErr) && int(retryErr.Pos.Offset()) <= errOffset {
				continue
			}
			return nil, err
		}
		if !restoreRepeatSite(tree, site, edits.while, src) {
			// The site is not a statement of its own in the retry, as
			// for `repeat 2 repeat 3 { ... }`, where the inner word is an
			// argument of the outer call until the outer site is
			// separated too; the outer site's retry re-enters here.
			continue
		}
		return tree, nil
	}
	return nil, firstErr
}

// restoreRepeatSite verifies that the retry read the site as intended and
// restores the condition statement's separator to the source: the `;` the
// retry wrote is cleared, and a `;` it blanked is put back.
func restoreRepeatSite(tree *syntax.File, site repeatSite, while bool, src []byte) bool {
	var cond *syntax.Stmt
	syntax.Walk(tree, func(node syntax.Node) bool {
		if cond != nil {
			return false
		}
		switch node := node.(type) {
		case *syntax.WhileClause:
			if while && int(node.WhilePos.Offset()) == site.start &&
				isRepeatCondition(node.Cond, site.countStart, site.countEnd) {
				cond = node.Cond[0]
				return false
			}
		case *syntax.Stmt:
			if call := repeatCall(node.Cmd); !while && call != nil &&
				int(call.Args[0].Pos().Offset()) == site.start {
				cond = node
				return false
			}
		}
		return true
	})
	if cond == nil {
		return false
	}
	setRepeatSeparator(cond, site.gap, src)
	return true
}

// isRepeatCondition reports whether cond is the single count-word statement
// a rewritten repeat loop must have. countEnd is -1 when only the start is
// known.
func isRepeatCondition(cond []*syntax.Stmt, countStart, countEnd int) bool {
	if len(cond) != 1 {
		return false
	}
	stmt := cond[0]
	if stmt.Negated || stmt.Background || stmt.Coprocess || len(stmt.Redirs) > 0 {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) != 1 {
		return false
	}
	count := call.Args[0]
	if int(count.Pos().Offset()) != countStart {
		return false
	}
	return countEnd < 0 || int(count.End().Offset()) == countEnd
}

// setRepeatSeparator points the statement's Semicolon at the first `;` the
// source has between the count word and the body, or clears it.
func setRepeatSeparator(stmt *syntax.Stmt, gap repeatGap, src []byte) {
	if len(gap.semicolons) == 0 {
		stmt.Semicolon = syntax.Pos{}
		return
	}
	stmt.Semicolon, _ = sourcePos(src, gap.semicolons[0])
}

// repeatCall returns cmd when it is a call named by the bare word `repeat`.
// The word is the reserved word even after an assignment prefix, where
// native Zsh reports a parse error.
func repeatCall(cmd syntax.Command) *syntax.CallExpr {
	call, ok := cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return nil
	}
	if value, ok := singleLiteral(call.Args[0]); !ok || value != "repeat" {
		return nil
	}
	return call
}

// singleLiteral returns the text of a word made of one unquoted literal.
func singleLiteral(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) != 1 {
		return "", false
	}
	lit, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return "", false
	}
	return lit.Value, true
}

// repeatEdit replaces src[start:end] with text; an insertion has start equal
// to end. Every synthetic byte maps back to start.
type repeatEdit struct {
	start int
	end   int
	text  string
}

// applyRepeatEdits rewrites src with edits, sorted by offset with earlier
// edits first at equal offsets, so an inner loop's closer lands before the
// closer of the loop that encloses it.
func applyRepeatEdits(src []byte, edits []repeatEdit) ([]byte, forSourceMap) {
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		return edits[i].end < edits[j].end
	})
	var transformed bytes.Buffer
	sm := forSourceMap{origByTransformed: make([]int, 0, len(src)+len(edits)*8)}
	last := 0
	for _, edit := range edits {
		for i := last; i < edit.start; i++ {
			transformed.WriteByte(src[i])
			sm.origByTransformed = append(sm.origByTransformed, i)
		}
		for i := 0; i < len(edit.text); i++ {
			transformed.WriteByte(edit.text[i])
			sm.origByTransformed = append(sm.origByTransformed, edit.start)
		}
		last = edit.end
	}
	for i := last; i < len(src); i++ {
		transformed.WriteByte(src[i])
		sm.origByTransformed = append(sm.origByTransformed, i)
	}
	sm.origByTransformed = append(sm.origByTransformed, len(src))
	return transformed.Bytes(), sm
}

// resolveRepeatLoops rewrites every `repeat` call left in a parsed tree into
// the loop it denotes. The parser reads `repeat 3 print hi` as one call and
// `repeat 3; print hi` as two statements; native Zsh reads the count and
// then one sublist, which extends over the whole `&&`, `||` and `|` chain
// to its right, or is empty when a `then`, `}`, `|`, `&&` or the end of the
// input follows the count. Each pass rewrites the last `repeat` call in the
// file, so an inner loop is closed before the loop around it, and reparses
// the original source with every edit so far through a source map; the
// final tree is rebased to original positions. A shape native Zsh rejects
// is reported as a parse error at the `repeat` word.
func resolveRepeatLoops(
	src []byte,
	name string,
	tree *syntax.File,
	invocations []AnonymousInvocation,
) (*syntax.File, []AnonymousInvocation, error) {
	if !bytes.Contains(src, []byte("repeat")) {
		return tree, invocations, nil
	}
	lineStarts := originalLineStarts(src)
	var edits []repeatEdit
	transformed, sm := applyRepeatEdits(src, nil)
	limit := bytes.Count(src, []byte("repeat"))
	for pass := 0; ; pass++ {
		call, parents := lastRepeatCall(tree)
		if call == nil {
			break
		}
		if pass >= limit {
			return nil, nil, fmt.Errorf("%s: repeat loop rewrite did not converge", name)
		}
		passEdits, err := repeatCallEdits(transformed, name, call, parents, invocations)
		if err != nil {
			return nil, nil, rebaseForError(err, sm, lineStarts)
		}
		for _, edit := range passEdits {
			// Every edited byte is original text, so a range keeps its
			// length; only the offset moves.
			start := sm.origByTransformed[edit.start]
			edits = append(edits, repeatEdit{start: start, end: start + edit.end - edit.start, text: edit.text})
		}
		transformed, sm = applyRepeatEdits(src, edits)
		tree, invocations, err = parseFull(transformed, name)
		if err != nil {
			return nil, nil, rebaseForError(err, sm, lineStarts)
		}
	}
	if len(edits) == 0 {
		return tree, invocations, nil
	}
	if err := rebaseForPositions(reflect.ValueOf(tree), sm, lineStarts); err != nil {
		return nil, nil, fmt.Errorf("%s: rebasing repeat loop positions: %w", name, err)
	}
	for _, invocation := range invocations {
		if err := rebaseForPositions(reflect.ValueOf(invocation.Words), sm, lineStarts); err != nil {
			return nil, nil, fmt.Errorf("%s: rebasing repeat loop positions: %w", name, err)
		}
	}
	return tree, invocations, nil
}

// lastRepeatCall returns the `repeat` call with the greatest offset and the
// parent of every node in the tree.
func lastRepeatCall(tree *syntax.File) (*syntax.CallExpr, map[syntax.Node]syntax.Node) {
	parents := make(map[syntax.Node]syntax.Node)
	var stack []syntax.Node
	var last *syntax.CallExpr
	syntax.Walk(tree, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) > 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		if cmd, ok := node.(syntax.Command); ok {
			if call := repeatCall(cmd); call != nil && (last == nil || call.Pos().After(last.Pos())) {
				last = call
			}
		}
		return true
	})
	return last, parents
}

// repeatCallEdits returns the edits, in the coordinates of src, that turn
// the loop starting at call into a `while` loop.
func repeatCallEdits(
	src []byte,
	name string,
	call *syntax.CallExpr,
	parents map[syntax.Node]syntax.Node,
	invocations []AnonymousInvocation,
) ([]repeatEdit, error) {
	shapeErr := syntax.ParseError{Filename: name, Pos: call.Args[0].Pos(), Text: repeatShapeError}
	stmt, ok := parents[call].(*syntax.Stmt)
	if !ok || len(call.Assigns) > 0 || len(call.Args) < 2 || stmt.Coprocess {
		return nil, shapeErr
	}
	start := int(call.Args[0].Pos().Offset())
	countEnd := int(call.Args[1].End().Offset())
	edits := []repeatEdit{{start: start, end: start + len("repeat"), text: "while "}}

	// A word or redirect after the count starts the sublist, which runs to
	// the end of the outermost statement the call is part of.
	bodyStart := -1
	if len(call.Args) > 2 {
		if value, ok := singleLiteral(call.Args[2]); ok && (value == "do" || value == "{") {
			return nil, shapeErr
		}
		bodyStart = int(call.Args[2].Pos().Offset())
	}
	for _, redirect := range stmt.Redirs {
		at := int(redirect.Pos().Offset())
		if at < countEnd {
			return nil, shapeErr
		}
		if bodyStart < 0 || at < bodyStart {
			bodyStart = at
		}
	}
	if bodyStart >= 0 {
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
		closer, ok := repeatSublistCloser(src, outer, repeatCloserOffset(outer, invocations))
		if !ok {
			return nil, shapeErr
		}
		return append(edits,
			repeatEdit{start: bodyStart, end: bodyStart, text: "; do\n"},
			repeatEdit{start: closer, end: closer, text: "\ndone"},
		), nil
	}

	if stmt.Background {
		return nil, shapeErr
	}
	// After a separator the body is the next statement in the list that
	// holds the outermost statement the call ends; on the left of `&&`,
	// `||` or `|`, or at the end of a list, the body is empty.
	var next *syntax.Stmt
	outer := stmt
	empty := false
walk:
	for {
		switch parent := parents[outer].(type) {
		case *syntax.BinaryCmd:
			if parent.X == outer {
				empty = true
				break walk
			}
			outer = parents[parent].(*syntax.Stmt)
		case *syntax.TimeClause:
			outer = parents[parent].(*syntax.Stmt)
		default:
			next = followingStmt(parent, outer)
			break walk
		}
	}
	if empty || next == nil {
		at := int(stmt.End().Offset())
		if stmt.Semicolon.IsValid() {
			at = int(stmt.Semicolon.Offset())
		}
		return append(edits, repeatEdit{start: at, end: at, text: "; do\ndone"}), nil
	}

	gap, bodyStart := scanRepeatGap(src, countEnd)
	if bodyStart != int(next.Pos().Offset()) {
		return nil, shapeErr
	}
	opener := "; do\n"
	if gap.newline || len(gap.semicolons) > 0 {
		opener = "do\n"
	}
	redundant := gap.semicolons
	if !gap.newline && len(redundant) > 0 {
		redundant = redundant[1:]
	}
	for _, at := range redundant {
		edits = append(edits, repeatEdit{start: at, end: at + 1, text: " "})
	}
	if block, ok := next.Cmd.(*syntax.Block); ok && !next.Negated {
		// `{ list }` is the loop's own body syntax, so the braces become
		// the keywords and the list is the body directly.
		lbrace := int(block.Lbrace.Offset())
		rbrace := int(block.Rbrace.Offset())
		if closer, ok := repeatSublistCloser(src, next, rbrace); !ok || closer != rbrace {
			return nil, shapeErr
		}
		return append(edits,
			repeatEdit{start: lbrace, end: lbrace + 1, text: opener},
			repeatEdit{start: rbrace, end: rbrace + 1, text: "\ndone"},
		), nil
	}
	closer, ok := repeatSublistCloser(src, next, repeatCloserOffset(next, invocations))
	if !ok {
		return nil, shapeErr
	}
	return append(edits,
		repeatEdit{start: bodyStart, end: bodyStart, text: opener},
		repeatEdit{start: closer, end: closer, text: "\ndone"},
	), nil
}

// repeatCloserOffset is where `done` goes after a sublist statement: before
// its `;` or `&` when it has one, else after its last word or redirect. The
// words of an anonymous function invocation in the statement are metadata
// the tree does not span, so a statement without a separator ends after
// the last of them; a separator always follows those words.
func repeatCloserOffset(stmt *syntax.Stmt, invocations []AnonymousInvocation) int {
	if stmt.Semicolon.IsValid() {
		return int(stmt.Semicolon.Offset())
	}
	closer := int(stmt.End().Offset())
	for _, invocation := range invocations {
		if len(invocation.Words) == 0 || invocation.Function.Pos().Offset() < stmt.Pos().Offset() ||
			invocation.Function.End().Offset() > stmt.End().Offset() {
			continue
		}
		closer = max(closer, int(invocation.Words[len(invocation.Words)-1].End().Offset()))
	}
	return closer
}

// repeatSublistCloser returns where `done` goes after a sublist statement
// whose last token ends at closer. The parser reads a heredoc body from the
// line after its redirect, so a `done` inserted on that line would become
// body text; the closer then moves past the heredoc's delimiter, provided
// only blanks, `;` and a comment separate the two. A heredoc with an empty
// body has no node to locate its delimiter by, and one that follows any
// other text on the closer's line cannot be closed by a rewrite; both are
// reported as false.
func repeatSublistCloser(src []byte, stmt *syntax.Stmt, closer int) (int, bool) {
	ok := true
	syntax.Walk(stmt, func(node syntax.Node) bool {
		if !ok {
			return false
		}
		redirect, isRedirect := node.(*syntax.Redirect)
		if !isRedirect || (redirect.Op != syntax.Hdoc && redirect.Op != syntax.DashHdoc) {
			return true
		}
		if redirect.Hdoc == nil {
			ok = false
			return false
		}
		bodyStart := int(redirect.Hdoc.Pos().Offset())
		if bodyStart <= closer {
			return true
		}
		if !repeatLineTail(src, closer, bodyStart) {
			ok = false
			return false
		}
		closer = int(redirect.Hdoc.End().Offset())
		return true
	})
	return closer, ok
}

// repeatLineTail reports whether src[from:to] is the end of one line: blanks,
// `;` and a comment, then the newline that to follows.
func repeatLineTail(src []byte, from, to int) bool {
	if to <= from || src[to-1] != '\n' {
		return false
	}
	for i := from; i < to-1; i++ {
		switch src[i] {
		case ' ', '\t', ';':
		case '#':
			return !bytes.ContainsRune(src[i:to-1], '\n')
		default:
			return false
		}
	}
	return true
}

// followingStmt returns the statement after stmt in the list of parent that
// holds it, or nil at the end of the list or when parent holds no list.
func followingStmt(parent syntax.Node, stmt *syntax.Stmt) *syntax.Stmt {
	var lists [][]*syntax.Stmt
	switch parent := parent.(type) {
	case *syntax.File:
		lists = [][]*syntax.Stmt{parent.Stmts}
	case *syntax.Block:
		lists = [][]*syntax.Stmt{parent.Stmts}
	case *syntax.Subshell:
		lists = [][]*syntax.Stmt{parent.Stmts}
	case *syntax.IfClause:
		lists = [][]*syntax.Stmt{parent.Cond, parent.Then}
	case *syntax.WhileClause:
		lists = [][]*syntax.Stmt{parent.Cond, parent.Do}
	case *syntax.ForClause:
		lists = [][]*syntax.Stmt{parent.Do}
	case *syntax.CaseItem:
		lists = [][]*syntax.Stmt{parent.Stmts}
	case *syntax.CmdSubst:
		lists = [][]*syntax.Stmt{parent.Stmts}
	case *syntax.ProcSubst:
		lists = [][]*syntax.Stmt{parent.Stmts}
	}
	for _, list := range lists {
		for index, candidate := range list {
			if candidate == stmt && index+1 < len(list) {
				return list[index+1]
			}
		}
	}
	return nil
}

// bindRepeatLoops collects every loop the front end rewrote from `repeat`,
// verifies its shape, and points its condition statement's separator at
// the `;` the source has after the count word, or clears it, so the tree
// carries no synthetic byte. A `while` loop positioned on a `repeat` word
// with any other shape is a rewrite the front end cannot vouch for and is
// reported as a parse error.
func bindRepeatLoops(tree *syntax.File, src []byte, name string) ([]RepeatLoop, error) {
	var loops []RepeatLoop
	var bindErr error
	syntax.Walk(tree, func(node syntax.Node) bool {
		if bindErr != nil {
			return false
		}
		loop, ok := node.(*syntax.WhileClause)
		if !ok {
			return true
		}
		start := int(loop.WhilePos.Offset())
		if !matchSourceWord(src, start, "repeat") {
			return true
		}
		countStart := skipInlineSpaces(src, start+len("repeat"))
		if loop.Until || !isRepeatCondition(loop.Cond, countStart, -1) {
			bindErr = syntax.ParseError{Filename: name, Pos: loop.WhilePos, Text: repeatShapeError}
			return false
		}
		count := loop.Cond[0].Cmd.(*syntax.CallExpr).Args[0]
		gap, _ := scanRepeatGap(src, int(count.End().Offset()))
		setRepeatSeparator(loop.Cond[0], gap, src)
		loops = append(loops, RepeatLoop{Loop: loop, Count: count})
		return true
	})
	return loops, bindErr
}
