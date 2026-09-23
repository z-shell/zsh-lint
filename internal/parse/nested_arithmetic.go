package parse

import (
	"bytes"
	"errors"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/syntax"
)

// An arithmetic expansion used as a nested parameter, `${$(( expr ))}` or
// `${(l:5:)$(( expr ))}`, is misread by mvdan/sh (LangZsh, v3.14.1).
// zshexpn (Parameter Expansion) allows "a ${...} type parameter expression
// or a $(...) type command substitution" in place of the name; it does not
// name `$((`, but Zsh's lexer reads `$((` as arithmetic before that rule
// applies, and the result is used as the name's value (`${$(( 2*3 ))}`
// prints 6). mvdan/sh's nestedParameterStart peeks one byte past the `$`
// and takes `$(` as a command substitution without checking for `$((`, so
// the body is parsed as a subshell command:
//
//   - `${$(( a[1] ))}` fails, because `a[1]` at the start of a command is an
//     assignment target (`` `a[b]` must be followed by `=` ``);
//   - `${$(( x > 3 ))}` parses, silently, as a subshell redirecting `x` to a
//     file named `3`, and every rule then sees the wrong tree.
//
// Native Zsh decides lexically (Src/lex.c, cmd_or_math): after `$((` it
// reads to the first unbalanced `)`, and the construct is arithmetic exactly
// when the next byte is `)` too. `${$((echo a) )}` and
// `${$((echo a); (echo b))}` are therefore command substitutions and keep
// that reading. cmd_or_math reads with double-quote rules, so it sees
// through a quote this file's scanner declines on; declining keeps the
// parser's reading, the conservative direction.
//
// Two pieces cover the two outcomes. resolveNestedArithmetic runs in Parse
// after the chain has produced a tree: every nested command substitution
// whose source is arithmetic by the rule above is replaced by the
// arithmetic expansion the parser builds for the same bytes. When the
// subshell reading fails instead, parseNestedArithmetic, in the chain, masks
// that one site to a same-length command substitution so the rest of the
// file can parse; the resolver then replaces the placeholder the same way.

// parseNestedArithmetic retries a parse that failed inside the body of a
// nested arithmetic expansion. The parser reads that body as a command, so
// the error text depends on the body, and the adapter gates on the error's
// position instead: it must lie inside a `$(( ... ))` span that stands in a
// nested parameter position and that native Zsh reads as arithmetic. The
// span is masked to `$(:` blanks `)`, keeping newlines, and the retry is
// verified to hold a nested command substitution at exactly that span.
func parseNestedArithmetic(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseNestedArithmeticWithParser(src, name, firstErr, parseWithAdapters)
}

func parseNestedArithmeticWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) {
		return nil, firstErr
	}
	start, end, ok := nestedArithmeticSiteAround(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[start+2] = ':'
	for i := start + 3; i < end-1; i++ {
		if masked[i] != '\n' {
			masked[i] = ' '
		}
	}

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !holdsNestedCommandSubstitution(tree, start, end) {
		return nil, firstErr
	}
	return tree, nil
}

// nestedArithmeticSiteAround finds the innermost `$(( ... ))` span that
// contains offset strictly inside it, stands in a nested parameter position
// and is arithmetic by native Zsh's rule. It returns the span as [start,end).
func nestedArithmeticSiteAround(src []byte, offset int) (int, int, bool) {
	if offset <= 0 || offset >= len(src) {
		return 0, 0, false
	}
	for start := offset - 1; start >= 0; start-- {
		if !bytes.HasPrefix(src[start:], []byte("$((")) || !isNestedParameterStart(src, start) {
			continue
		}
		end, ok := nestedArithmeticEnd(src, start)
		if ok && offset < end {
			return start, end, true
		}
	}
	return 0, 0, false
}

// isNestedParameterStart reports whether the `$` at dollar begins the
// parameter of an enclosing `${...}`: the bytes before it, read backwards,
// are an optional `"`, any of the prefix characters `#`, `=`, `~`, `^`, an
// optional `(flags)` group, and then `${`. The parser reads flags as a
// literal up to the first `)`, so a flags group holds no `)`. `+` is not a
// prefix here: `${+$(...)}` is not valid Zsh, and in `${#+$(( ))}` the `+`
// is the alternate-value operator, whose word the parser already reads as
// arithmetic. The masked retry verifies the result, so this only has to
// avoid masking a span that is plainly not a nested parameter.
func isNestedParameterStart(src []byte, dollar int) bool {
	j := dollar - 1
	if j >= 0 && src[j] == '"' {
		j--
	}
	for j >= 0 && strings.IndexByte("#=~^", src[j]) >= 0 {
		j--
	}
	if j >= 0 && src[j] == ')' {
		k := j - 1
		for k >= 0 && src[k] != '(' && src[k] != '\n' {
			k--
		}
		if k < 0 || src[k] != '(' {
			return false
		}
		j = k - 1
	}
	return j >= 1 && src[j] == '{' && src[j-1] == '$'
}

// nestedArithmeticEnd applies native Zsh's lexical rule to the `$((` at
// start: scan to the first unbalanced `)` and report the span as arithmetic
// only when the byte after it is `)` as well. It returns the offset just
// past that second `)`. A nested `${...}` is skipped whole and a backslash
// escapes the next byte; a quote or backquote makes the extent uncertain,
// so it reports false and the parser's own reading stands.
func nestedArithmeticEnd(src []byte, start int) (int, bool) {
	depth := 0
	for i := start + 3; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'', '"', '`':
			return 0, false
		case '$':
			if i+1 < len(src) && src[i+1] == '{' {
				close, ok := nestedArithmeticBraceEnd(src, i+1)
				if !ok {
					return 0, false
				}
				i = close
			}
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
				continue
			}
			if i+1 < len(src) && src[i+1] == ')' {
				return i + 2, true
			}
			return 0, false
		}
	}
	return 0, false
}

// nestedArithmeticBraceEnd returns the offset of the `}` closing the `{` at
// open, counting nested braces. A quote or backquote makes it give up, like
// nestedArithmeticEnd.
func nestedArithmeticBraceEnd(src []byte, open int) (int, bool) {
	depth := 0
	for i := open; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case '\'', '"', '`':
			return 0, false
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// nestedCommandSubstitution returns the command substitution a parameter
// expansion holds as its nested parameter, directly or as the only part of
// a double-quoted nested parameter, `${"$(...)"}`.
func nestedCommandSubstitution(exp *syntax.ParamExp) (*syntax.CmdSubst, func(syntax.WordPart)) {
	switch nested := exp.NestedParam.(type) {
	case *syntax.CmdSubst:
		return nested, func(part syntax.WordPart) { exp.NestedParam = part }
	case *syntax.DblQuoted:
		if len(nested.Parts) == 1 {
			if sub, ok := nested.Parts[0].(*syntax.CmdSubst); ok {
				return sub, func(part syntax.WordPart) { nested.Parts[0] = part }
			}
		}
	}
	return nil, nil
}

// holdsNestedCommandSubstitution reports whether tree has a nested command
// substitution spanning exactly [start,end), the node the masked retry must
// produce for the resolver to restore.
func holdsNestedCommandSubstitution(tree *syntax.File, start, end int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		if exp, ok := node.(*syntax.ParamExp); ok {
			if sub, _ := nestedCommandSubstitution(exp); sub != nil &&
				int(sub.Left.Offset()) == start && int(sub.Right.Offset()) == end-1 {
				found = true
			}
		}
		return !found
	})
	return found
}

// nestedArithmeticSite is one nested command substitution the resolver
// replaces with an arithmetic expansion.
type nestedArithmeticSite struct {
	start, end int
	replace    func(syntax.WordPart)
}

// resolveNestedArithmetic replaces every nested command substitution in
// node whose source bytes native Zsh reads as arithmetic. Each site is
// reparsed from the original bytes; an arithmetic body the parser rejects
// is reported as that error, the verdict the same body gets as a top-level
// `$(( ... ))`.
func resolveNestedArithmetic(src []byte, name string, node syntax.Node) error {
	if !bytes.Contains(src, []byte("$((")) {
		return nil
	}
	var sites []nestedArithmeticSite
	claimed := map[*syntax.CmdSubst]bool{}
	syntax.Walk(node, func(node syntax.Node) bool {
		switch node := node.(type) {
		case *syntax.CmdSubst:
			// A claimed substitution's subtree is the misread body and
			// is replaced whole; a site inside it is resolved by the
			// reparse of the enclosing span instead. Walking into it
			// would give the same tree, reparsing every inner level once
			// per enclosing level.
			return !claimed[node]
		case *syntax.ParamExp:
			sub, replace := nestedCommandSubstitution(node)
			if sub == nil {
				return true
			}
			start := int(sub.Left.Offset())
			if start+3 > len(src) || string(src[start:start+3]) != "$((" {
				return true
			}
			end, ok := nestedArithmeticEnd(src, start)
			// The two readings end at different bytes only when a `#`
			// starts a comment in the command reading, which native Zsh
			// does not do inside arithmetic. The extent the tree holds
			// then disagrees with the source, so the command reading the
			// base parser gave is left as it was.
			if !ok || end-1 != int(sub.Right.Offset()) {
				return true
			}
			claimed[sub] = true
			sites = append(sites, nestedArithmeticSite{start: start, end: end, replace: replace})
		}
		return true
	})
	for _, site := range sites {
		arith, err := parseArithmeticSpan(src, name, site.start, site.end)
		if err != nil {
			return err
		}
		site.replace(arith)
	}
	return nil
}

// arithmeticSpanReparses counts parseArithmeticSpan calls, so a test can pin
// that each nested site is reparsed once. Parse is a library entry point, so
// the counter is atomic rather than assume a single caller.
var arithmeticSpanReparses atomic.Int64

// parseArithmeticSpan parses src[start:end], a `$(( ... ))` span, as the
// arithmetic expansion the parser builds for it at the top level. Every
// byte outside the span is blanked, keeping newlines, so each position in
// the result is the original one. The retry runs through the adapter chain,
// so a construct inside the expression that needs an adapter still parses,
// and a nested arithmetic site inside it is resolved in turn.
func parseArithmeticSpan(src []byte, name string, start, end int) (*syntax.ArithmExp, error) {
	arithmeticSpanReparses.Add(1)
	blanked := make([]byte, len(src))
	for i, b := range src {
		switch {
		case i >= start && i < end:
			blanked[i] = b
		case b == '\n':
			blanked[i] = '\n'
		default:
			blanked[i] = ' '
		}
	}
	tree, err := parseWithAdapters(blanked, name)
	if err != nil {
		return nil, err
	}
	arith := soleArithmeticExpansion(tree)
	if arith == nil || int(arith.Left.Offset()) != start || int(arith.End().Offset()) != end {
		// The span did not come back as one arithmetic expansion over
		// exactly its own bytes: report it where it starts rather than
		// splice in a node that does not describe the source.
		return nil, syntax.ParseError{
			Filename: name,
			Pos:      positionAt(src, start),
			Text:     "`$((` nested in a parameter expansion must be one arithmetic expansion",
		}
	}
	if err := resolveNestedArithmetic(src, name, arith); err != nil {
		return nil, err
	}
	return arith, nil
}

// soleArithmeticExpansion returns the arithmetic expansion when tree is one
// statement whose command is one word made of just that expansion.
func soleArithmeticExpansion(tree *syntax.File) *syntax.ArithmExp {
	if len(tree.Stmts) != 1 {
		return nil
	}
	call, ok := tree.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) != 1 || len(call.Args[0].Parts) != 1 {
		return nil
	}
	arith, _ := call.Args[0].Parts[0].(*syntax.ArithmExp)
	return arith
}

// positionAt converts a byte offset in src to a syntax position.
func positionAt(src []byte, offset int) syntax.Pos {
	line, col := 1, 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}
		col++
	}
	return syntax.NewPos(uint(offset), uint(line), uint(col))
}
