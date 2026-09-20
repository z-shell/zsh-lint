package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// doEmptyBodyErrors are the parser errors a loop body beginning with a
// separator produces. `do;` on one line is read as an empty body, so the
// parser then demands `done` and reports it at the loop keyword; a `;` after
// a newline or a comment is instead rejected where it stands.
var doEmptyBodyErrors = []string{
	"`while` statement must end with `done`",
	"`until` statement must end with `done`",
	"`for` statement must end with `done`",
	"`select` statement must end with `done`",
	"`;` can only immediately follow a statement",
}

// doSeparatorKeywords are the reserved words the loop adapter masks after.
var doSeparatorKeywords = map[string]bool{"do": true}

// leadingSeparatorSite is a reserved word whose following list begins with
// a `;`.
type leadingSeparatorSite struct {
	// keyword is the offset of the reserved word.
	keyword int
	// separator is the offset of the `;` that opens the list.
	separator int
}

// leadingSeparatorSpec describes one family of reserved words whose
// following list Zsh reads as zero or more sublists, so that a leading `;`
// is an empty sublist the parser (mvdan/sh through v3.14.1) instead takes
// as the whole list. The `do` adapter (#238) and the `then`/`else` adapter
// (#297) share the gate, mask and verify steps through it.
type leadingSeparatorSpec struct {
	// errors are the exact parser error texts the gate accepts.
	errors []string
	// keywords are the reserved words a site may start with.
	keywords map[string]bool
	// verify reports whether the retry's tree holds the expected node whose
	// keyword is at the given offset, so a genuinely broken construct keeps
	// the parser error.
	verify func(tree *syntax.File, keyword int) bool
}

// parseDoLeadingSeparator adapts a loop body that begins with a separator
// directly after `do` (issue #238): `while (( $# )); do; shift; done`. Zsh
// reads `do list done` with a list of zero or more sublists, so a leading
// `;` is an empty sublist. The parser (mvdan/sh through v3.14.1) takes the
// `;` as the whole body and demands `done` next. The shared
// parseLeadingSeparatorWithParser masks that `;` and verifies the tree holds
// a loop whose `do` is the site, so a genuinely unterminated loop keeps the
// parser error.
func parseDoLeadingSeparator(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseDoLeadingSeparatorWithParser(src, name, firstErr, parseWithAdapters)
}

func parseDoLeadingSeparatorWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	spec := leadingSeparatorSpec{errors: doEmptyBodyErrors, keywords: doSeparatorKeywords, verify: hasLoopDo}
	return parseLeadingSeparatorWithParser(src, name, firstErr, spec, parse)
}

// parseLeadingSeparatorWithParser is the shared adapter body: gate on one
// of spec's exact error texts, find the first site at or after the error,
// overwrite its one `;` with a blank (every offset and comment byte is
// kept; an empty sublist owns no node, so nothing is restored afterwards),
// re-parse through parse, and hand the tree back only when spec.verify
// finds the construct at the site. Each pass masks one separator; `; ;`
// re-enters the chain for the next.
func parseLeadingSeparatorWithParser(
	src []byte,
	name string,
	firstErr error,
	spec leadingSeparatorSpec,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || !isLeadingSeparatorError(parseErr.Text, spec.errors) {
		return nil, firstErr
	}

	// The closer error sits on the failing construct's keyword and the `;`
	// error on the separator itself, so in both shapes the site's separator
	// is at or after the error. A construct before the error has parsed.
	site, ok := findLeadingSeparatorSite(src, int(parseErr.Pos.Offset()), spec.keywords)
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[site.separator] = ' '

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !spec.verify(tree, site.keyword) {
		return nil, firstErr
	}
	return tree, nil
}

// isDoEmptyBodyError reports whether text is one of the errors the parser
// gives a loop body that begins with a separator.
func isDoEmptyBodyError(text string) bool {
	return isLeadingSeparatorError(text, doEmptyBodyErrors)
}

// isLeadingSeparatorError reports whether text is one of the gate's errors.
func isLeadingSeparatorError(text string, errors []string) bool {
	for _, want := range errors {
		if text == want {
			return true
		}
	}
	return false
}

// findLeadingSeparatorSite returns the first reserved word in keywords, in
// command position, whose next syntactically active byte is a single `;` at
// or after seed. Blanks, newlines and comments may separate the two. A
// `;;`, `;&` or `;|` is a case terminator, which Zsh rejects there too, and
// is left to the parser.
func findLeadingSeparatorSite(src []byte, seed int, keywords map[string]bool) (leadingSeparatorSite, bool) {
	var site leadingSeparatorSite
	found := false
	scanDoSeparatorWords(src, func(start, end int, word string) (int, bool) {
		if found || !keywords[word] {
			return 0, false
		}
		separator := skipSpacesAndComments(src, end)
		if separator < seed || separator >= len(src) || src[separator] != ';' {
			return 0, false
		}
		if separator+1 < len(src) && strings.IndexByte(";&|", src[separator+1]) >= 0 {
			return 0, false
		}
		site = leadingSeparatorSite{keyword: start, separator: separator}
		found = true
		return 0, false
	})
	return site, found
}

// hasLoopDo reports whether the tree holds a `do ... done` loop whose `do`
// is at offset do. `select` and the C-style `for` are ForClauses too; the
// brace-form for loop has no `do` and never matches.
func hasLoopDo(tree *syntax.File, do int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		switch loop := node.(type) {
		case *syntax.WhileClause:
			found = int(loop.DoPos.Offset()) == do
		case *syntax.ForClause:
			found = !loop.Braces && int(loop.DoPos.Offset()) == do
		}
		return !found
	})
	return found
}

// scanDoSeparatorWords calls visit for every identifier word in command
// position in syntactically active source. Native Zsh recognises a reserved
// word at the start of a command: after a separator, an opening `(` or `{`,
// a case pattern's `)`, a `!`, or a reserved word that itself precedes a
// command. Quoted text, comments, heredoc bodies and arithmetic never hold
// one. A visit that consumes the word's operands returns the offset to
// resume at and true; the next word is then in command position again.
// Otherwise the word is treated by its own meaning: a reserved word that
// precedes a command keeps command position, any other word ends it.
//
// This is the command-position walk of scanRepeatSites (repeat.go) with the
// `repeat` case handed to the caller. The if short form adapter (#210)
// extracts the same walk from scanRepeatSites as scanCommandWords; once
// both are on main this copy should be dropped in favour of that one.
func scanDoSeparatorWords(src []byte, visit func(start, end int, word string) (int, bool)) {
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
				return
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
					return
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
		case atCommandStart:
			if resume, ok := visit(start, i+1, word); ok {
				i = resume - 1
				continue
			}
			if !repeatCommandPrefixWords[word] {
				atCommandStart = false
			}
		default:
			atCommandStart = false
		}
	}
}
