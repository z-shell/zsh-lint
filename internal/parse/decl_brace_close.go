package parse

import (
	"bytes"
	"errors"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// unmatchedBraceClose is the tail of every error the parser reports for a
// `{` it never closes: `reached EOF without matching ...`, and the `)`, `;;`
// and backtick variants when the block sits inside a subshell, a case arm,
// or a legacy substitution. The error lands on the open `{`, so the block
// to close is the first candidate after it.
const unmatchedBraceClose = "without matching `{` with `}`"

// keywordInsideBlockErrors are the errors the parser reports when the block
// sits inside a loop, an `if` or a `case` and a separator follows the brace
// (issue #298): the clause stops at the `;` with the `}` swallowed, the
// block stays open, and the parser meets the enclosing construct's keyword
// inside it. The error lands on that keyword, after the block to close, so
// for this shape the site is the first candidate before the error.
var keywordInsideBlockErrors = map[string]bool{
	"`then` can only be used in an `if`":      true,
	"`elif` can only be used in an `if`":      true,
	"`fi` can only be used to end an `if`":    true,
	"`do` can only be used in a loop":         true,
	"`done` can only be used to end a loop":   true,
	"`esac` can only be used to end a `case`": true,
}

// parseDeclarationBraceClose adapts a brace block whose last command is a
// declaration builtin or `let` with no separator before the closing `}`
// (issues #231, #259 and #298). The parser reads those clauses up to a stop
// token, and a `}` word is not one, so `{ local x=1 }` makes the brace a
// naked argument and leaves the block open. The retry replaces the whitespace
// byte before that `}` with a `;`, which keeps every offset, and the synthetic
// separator is cleared from the statement afterwards so the tree matches the
// original source. An unquoted `}` glued to the preceding word or followed
// by a word byte is not the closing brace in Zsh either and keeps the error.
func parseDeclarationBraceClose(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseDeclarationBraceCloseWithParser(src, name, firstErr, parseWithAdapters)
}

func parseDeclarationBraceCloseWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	// The unmatched error lands on the innermost `{` still open at the stop
	// token, so the candidate is the first `}` after it; the keyword error
	// lands past the open block, so the candidate is the first `}` before
	// it. A `}` the clause consumed leaves its own block open, and a second
	// block is reached by re-entering the chain after this retry.
	after, before := -1, len(src)
	switch {
	case strings.HasSuffix(parseErr.Text, unmatchedBraceClose):
		after = int(parseErr.Pos.Offset())
	case keywordInsideBlockErrors[parseErr.Text]:
		before = int(parseErr.Pos.Offset())
	default:
		return nil, firstErr
	}

	space, brace, ok := findDeclarationBraceClose(src, after, before)
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[space] = ';'

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !restoreDeclarationBraceClose(tree, space, brace) {
		return nil, firstErr
	}
	return tree, nil
}

// declarationClauseWords are the command words the parser turns into a
// DeclClause or LetClause, which read arguments until a stop token.
// `integer` and `float` are ordinary calls and already stop at `}`.
var declarationClauseWords = map[string]bool{
	"declare":  true,
	"export":   true,
	"let":      true,
	"local":    true,
	"nameref":  true,
	"readonly": true,
	"typeset":  true,
}

// findDeclarationBraceClose scans syntactically active source for the first
// declaration clause whose argument list ends at a whole-word `}` on the
// same logical line, with that brace after and before the given offsets. It
// returns the offsets of the whitespace byte before the brace and of the
// brace itself.
func findDeclarationBraceClose(src []byte, after, before int) (int, int, bool) {
	inSingleQuote := false
	inDoubleQuote := false
	inANSICQuote := false
	escaped := false
	inComment := false
	var heredocs []pendingHeredoc
	arithmeticDepth := 0

	// atCommandStart is true where the next word is a command name. A
	// declaration word elsewhere (`print local`) is an argument.
	atCommandStart := true
	// inClause is true between a declaration word and its terminator;
	// parenDepth, braceDepth, and inBacktick track `(...)`, `{...}`, and
	// legacy substitutions inside its arguments, such as `local x=(1 2)`,
	// `${y}`, `$(a | b)`, or `` `a; b` ``, whose separators do not end it.
	inClause := false
	parenDepth := 0
	braceDepth := 0
	inBacktick := false

	endClause := func(commandStart bool) {
		inClause = false
		parenDepth = 0
		braceDepth = 0
		inBacktick = false
		atCommandStart = commandStart
	}
	// nestedInClause reports whether a separator sits inside a substitution
	// of the clause and so continues it.
	nestedInClause := func() bool {
		return inClause && (parenDepth > 0 || inBacktick)
	}

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
				if !nestedInClause() {
					endClause(true)
				}
			}
			continue
		}
		if b == '\n' && len(heredocs) > 0 {
			next, ok := consumeHeredocBodies(src, i+1, heredocs)
			if !ok {
				return 0, 0, false
			}
			heredocs = nil
			i = next - 1
			endClause(true)
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
			if inClause && !nestedInClause() && i > 0 && isDeclarationSpace(src[i-1]) && i+2 < len(src) &&
				src[i+1] == '\n' && src[i+2] == '}' && isDeclarationBraceCandidate(src, i+2, after, before) {
				// A `\`-newline before the brace is removed by the shell, so
				// the backslash is the byte that becomes the separator.
				return i, i + 2, true
			}
			escaped = true
			atCommandStart = false
			continue
		case '\'':
			inSingleQuote = true
			atCommandStart = false
			continue
		case '"':
			atCommandStart = false
			if end, ok := skipDoubleQuotedString(src, i); ok {
				i = end
				continue
			}
			inDoubleQuote = true
			continue
		case '#':
			if i == 0 || isDeclarationWordBoundary(src[i-1]) {
				inComment = true
				continue
			}
			atCommandStart = false
			continue
		case '$':
			if i+1 < len(src) && src[i+1] == '\'' {
				inANSICQuote = true
				i++
				atCommandStart = false
				continue
			}
		case ' ', '\t':
			continue
		case '\n', ';', '&', '|':
			if !nestedInClause() {
				endClause(true)
			}
			continue
		case '`':
			if inClause {
				inBacktick = !inBacktick
			} else {
				endClause(true)
			}
			continue
		}
		if b == '<' && i+1 < len(src) && src[i+1] == '<' {
			heredoc, end, isHeredoc, ok := heredocAt(src, i)
			if !ok {
				return 0, 0, false
			}
			if !isHeredoc {
				// A here-string `<<<`: its word is ordinary text.
				i += 2
				atCommandStart = false
				continue
			}
			heredocs = append(heredocs, heredoc)
			i = end - 1
			atCommandStart = false
			continue
		}

		if inClause {
			switch b {
			case '(':
				parenDepth++
			case ')':
				if parenDepth > 0 {
					parenDepth--
				} else {
					// The `)` closes an enclosing subshell or case arm.
					endClause(true)
				}
			case '{':
				braceDepth++
			case '}':
				if braceDepth > 0 {
					braceDepth--
					continue
				}
				if nestedInClause() {
					// A brace inside a substitution belongs to that command
					// list, not to the block around the clause.
					continue
				}
				if i > 0 && isDeclarationSpace(src[i-1]) && isDeclarationBraceCandidate(src, i, after, before) {
					return i - 1, i, true
				}
				// A glued `}` is part of the word (`x=1}`) or a stray brace
				// (`}else`, `}}`), so this clause is not the one to close.
				endClause(false)
			}
			continue
		}

		switch b {
		case '(', '{':
			atCommandStart = true
			continue
		case ')', '}':
			atCommandStart = false
			continue
		case '!':
			// `! local x=1` negates the clause and keeps it in command
			// position.
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
		wordEnd := i + 1
		gluedToWord := start > 0 && !isDeclarationWordBoundary(src[start-1]) && src[start-1] != '(' && src[start-1] != '`'
		followedByWord := wordEnd < len(src) && !isDeclarationWordBoundary(src[wordEnd]) &&
			src[wordEnd] != ')' && src[wordEnd] != '&' && src[wordEnd] != '|' && src[wordEnd] != '`'
		switch {
		case gluedToWord || followedByWord:
			atCommandStart = false
		case atCommandStart && declarationClauseWords[word]:
			inClause = true
		case word == "do", word == "then", word == "else", word == "elif", word == "time":
			atCommandStart = true
		default:
			atCommandStart = false
		}
	}
	return 0, 0, false
}

// isDeclarationBraceCandidate reports whether the `}` at brace lies strictly
// between after and before and is followed by a separator, a redirection, a
// closing token, or the end of the source. The caller checks the byte before
// the brace.
func isDeclarationBraceCandidate(src []byte, brace, after, before int) bool {
	if brace <= after || brace >= before {
		return false
	}
	return brace+1 == len(src) || isDeclarationBraceFollower(src[brace+1])
}

// isDeclarationSpace reports whether b is the blank that must precede the
// closing brace for it to be a word of its own.
func isDeclarationSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

// isDeclarationWordBoundary reports whether b ends a word before a `#`
// comment or a declaration command word.
func isDeclarationWordBoundary(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == ';' || b == '{' || b == '}'
}

// isDeclarationBraceFollower reports whether b may follow a closing `}` in
// Zsh: a separator, a redirection, or the end of a subshell or legacy
// substitution. A word byte or a `#` glues onto the brace instead.
func isDeclarationBraceFollower(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '&', '|', ')', '<', '>', '`':
		return true
	}
	return false
}

// restoreDeclarationBraceClose finds the block the mask closed, whose `}` is
// at brace and whose last statement is a declaration or let clause ended by
// the synthetic separator at space, and clears that separator so the
// statement ends at its last argument as in the original source. It reports
// false when no block matches, so the caller returns the parser error
// instead of an unverified tree.
func restoreDeclarationBraceClose(tree *syntax.File, space, brace int) bool {
	restored := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if restored {
			return false
		}
		block, ok := node.(*syntax.Block)
		if !ok || int(block.Rbrace.Offset()) != brace || len(block.Stmts) == 0 {
			return true
		}
		last := block.Stmts[len(block.Stmts)-1]
		if int(last.Semicolon.Offset()) != space || last.Background || last.Coprocess {
			return true
		}
		if !isDeclarationCommand(last.Cmd) {
			return true
		}
		last.Semicolon = syntax.Pos{}
		restored = true
		return false
	})
	return restored
}

// isDeclarationCommand reports whether cmd is a declaration or let clause,
// directly or under a `time` prefix.
func isDeclarationCommand(cmd syntax.Command) bool {
	switch cmd := cmd.(type) {
	case *syntax.DeclClause, *syntax.LetClause:
		return true
	case *syntax.TimeClause:
		return cmd.Stmt != nil && isDeclarationCommand(cmd.Stmt.Cmd)
	}
	return false
}
