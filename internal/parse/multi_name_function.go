package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

// multiNameFunctionError is the error the parser raises at the `(` of a
// definition that names more than one function, `name1 name2 () { list }`.
// In the Zsh dialect a `(` inside a command is a glob qualifier unless `)`
// follows it directly, so this text with `()` at the error offset names one
// construct.
const multiNameFunctionError = "a command can only contain words and redirects; encountered `(`"

// sourceSpan is a half-open byte range [start, end) of the original source.
type sourceSpan struct {
	start int
	end   int
}

// parseMultiNameFunction adapts the multi-name function definition
// `name1 name2 () { list }` (issue #251), which defines every listed name with
// the same body. mvdan/sh (through v3.14.1) reads the `()` form with one name
// only; the reserved-word form `function name1 name2` already yields a
// FuncDecl whose Names holds every name, and that is the shape restored here.
// The retry masks every name but the last with same-width spaces so the
// parser sees a single-name definition at unchanged offsets, then the
// declaration gets its full name list and its position at the first name
// back. A name the parser would not accept on its own (a quoted word, an
// expansion, an assignment) is never masked, so the gate does not widen
// what a single-name definition accepts.
func parseMultiNameFunction(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseMultiNameFunctionWithParser(src, name, firstErr, parseWithAdapters)
}

func parseMultiNameFunctionWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != multiNameFunctionError {
		return nil, firstErr
	}

	names, ok := findMultiNameFunction(src, int(parseErr.Pos.Offset()))
	if !ok {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	for _, span := range names[:len(names)-1] {
		for i := span.start; i < span.end; i++ {
			masked[i] = ' '
		}
	}

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !restoreMultiNameFunction(tree, src, names) {
		return nil, firstErr
	}
	return tree, nil
}

// findMultiNameFunction reads the names of the definition whose `()` starts
// at paren: the run of blank-separated plain words before it on the same
// logical line, back to the start of the command. It returns the names in
// source order, at least two of them, or false when the site is not a
// multi-name definition the parser would accept name by name.
func findMultiNameFunction(src []byte, paren int) ([]sourceSpan, bool) {
	if paren < 0 || paren+1 >= len(src) || src[paren] != '(' || src[paren+1] != ')' {
		return nil, false
	}

	var names []sourceSpan
	i := paren
	for {
		i = skipBlanksBackward(src, i)
		if i >= 2 && src[i-1] == '\n' && src[i-2] == '\\' {
			// A `\`-newline continuation is removed by the shell, so the
			// names may span lines that way.
			i -= 2
			continue
		}
		end := i
		for i > 0 && isFunctionNameByte(src[i-1]) {
			i--
		}
		if i == end {
			break
		}
		names = append(names, sourceSpan{start: i, end: end})
	}
	if len(names) == 0 {
		return nil, false
	}

	// names is in reverse source order; the last collected word is the
	// leftmost, at command position.
	for left, right := 0, len(names)-1; left < right; left, right = left+1, right-1 {
		names[left], names[right] = names[right], names[left]
	}

	// The run must start a command. A reserved word the parser reads before
	// a command (`then a b () { }`, `repeat 2 a b () { }`) is the boundary
	// rather than a name, together with the words it takes; a reserved word
	// that opens a construct of its own (`for`, `case`, `select`) never
	// begins a definition; any other word before the run (`x=1 a b () { }`,
	// `! a b () { }`) leaves the site unrecognised.
	first := names[0]
	firstWord := string(src[first.start:first.end])
	if taken, ok := multiNameFunctionPrefixWords[firstWord]; ok {
		if len(names) <= taken+1 {
			return nil, false
		}
		names = names[taken+1:]
	} else if multiNameFunctionNeverNames[firstWord] {
		return nil, false
	} else {
		boundary := skipBlanksBackward(src, first.start)
		if boundary > 0 && !isMultiNameFunctionBoundary(src[boundary-1]) {
			return nil, false
		}
	}
	if len(names) < 2 {
		return nil, false
	}
	return names, true
}

// multiNameFunctionPrefixWords are the reserved words after which a function
// definition may begin on the same line, with the number of words each one
// takes first (`repeat` reads its count). The parser reads them as part of
// the enclosing construct, so they never count as a name.
var multiNameFunctionPrefixWords = map[string]int{
	"coproc":    0,
	"do":        0,
	"elif":      0,
	"else":      0,
	"if":        0,
	"nocorrect": 0,
	"repeat":    1,
	"then":      0,
	"time":      0,
	"until":     0,
	"while":     0,
}

// multiNameFunctionNeverNames are the reserved words that open a construct
// of their own at command position, so a run that starts with one is not a
// function definition whatever follows.
var multiNameFunctionNeverNames = map[string]bool{
	"always":   true,
	"case":     true,
	"done":     true,
	"end":      true,
	"esac":     true,
	"fi":       true,
	"for":      true,
	"foreach":  true,
	"function": true,
	"select":   true,
}

// isFunctionNameByte reports whether b may appear in a name the retry masks:
// the plain literal bytes the parser itself accepts in a single-name `()`
// definition, without quotes, expansions, globs, or an assignment `=`.
func isFunctionNameByte(b byte) bool {
	if isIdentByte(b) {
		return true
	}
	switch b {
	case '-', '.', ':', '/', '+', '@', ',', '%':
		return true
	}
	return false
}

// isMultiNameFunctionBoundary reports whether b, the byte before the first
// name, starts a command: a separator, an opening subshell or brace, or a
// legacy substitution delimiter.
func isMultiNameFunctionBoundary(b byte) bool {
	switch b {
	case '\n', ';', '&', '|', '(', '{', '`':
		return true
	}
	return false
}

// skipBlanksBackward returns the offset after the last non-blank byte before
// i, so src[result-1] is neither a space nor a tab.
func skipBlanksBackward(src []byte, i int) int {
	for i > 0 && (src[i-1] == ' ' || src[i-1] == '\t') {
		i--
	}
	return i
}

// restoreMultiNameFunction finds the single-name declaration the retry
// produced at the last name and gives it every name from the original
// source, with the declaration and its statement positioned at the first
// name, as the reserved-word form `function name1 name2` is. It reports
// false when no declaration matches, so the caller returns the parser error
// rather than an unverified tree.
func restoreMultiNameFunction(tree *syntax.File, src []byte, names []sourceSpan) bool {
	last := names[len(names)-1]
	restored := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if restored {
			return false
		}
		stmt, ok := node.(*syntax.Stmt)
		if !ok {
			return true
		}
		decl, ok := stmt.Cmd.(*syntax.FuncDecl)
		if !ok || decl.RsrvWord || !decl.Parens || decl.Name == nil || len(decl.Names) != 0 {
			return true
		}
		if int(stmt.Position.Offset()) != last.start || int(decl.Position.Offset()) != last.start ||
			int(decl.Name.ValuePos.Offset()) != last.start || int(decl.Name.ValueEnd.Offset()) != last.end ||
			decl.Name.Value != string(src[last.start:last.end]) {
			return true
		}
		lits := make([]*syntax.Lit, 0, len(names))
		for _, span := range names {
			start, ok := sourcePos(src, span.start)
			if !ok {
				return false
			}
			end, ok := sourcePos(src, span.end)
			if !ok {
				return false
			}
			lits = append(lits, &syntax.Lit{
				ValuePos: start,
				ValueEnd: end,
				Value:    string(src[span.start:span.end]),
			})
		}
		decl.Name = nil
		decl.Names = lits
		decl.Position = lits[0].ValuePos
		stmt.Position = decl.Position
		restored = true
		return false
	})
	return restored
}
