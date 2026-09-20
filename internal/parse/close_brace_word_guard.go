package parse

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// closeBraceWordError is the parser's own text for a `}` in a command's
// arguments, reused where mvdan/sh reads the byte as part of a word instead.
const closeBraceWordError = "`}` can only be used to close a block"

// rejectCloseBraceWords returns a parse error at the first `}` that Zsh's
// lexer would read as the reserved word rather than as part of a word: an
// unquoted, unescaped `}` that ends a word while no `{` earlier in the
// word's unquoted literal text is still open (issues #314 and #316). Zsh
// ends the word there and rejects the token, so `for x in a }`, `print a}`,
// `x=( a} )`, `case a} in` and `[[ a} == a} ]]` are parse errors natively;
// mvdan/sh (v3.14.1) rejects the bare byte only in a command's arguments
// and otherwise lexes it into the word. A `}` that closes a `{` in the same
// word (`{a}`, `a{}`), one that is not the word's last byte (`a}b`), an
// escaped or quoted one, and a `}` inside an expansion stay words, as do the
// three positions Zsh lexes differently: the value of a `name=value`
// assignment, a case pattern and a here-document body.
func rejectCloseBraceWords(tree *syntax.File, name string) error {
	var found error
	var stack []syntax.Node
	syntax.Walk(tree, func(node syntax.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if found != nil {
			return false
		}
		word, ok := node.(*syntax.Word)
		if ok && !closeBraceExemptWord(word, stack) {
			if pos, ok := trailingCloseBrace(word); ok {
				found = syntax.ParseError{Filename: name, Pos: pos, Text: closeBraceWordError}
				return false
			}
		}
		stack = append(stack, node)
		return true
	})
	return found
}

// closeBraceExemptWord reports whether word sits where Zsh reads a
// trailing `}` as ordinary text: a `name=value` assignment's value (not a
// naked declaration argument, whose word Zsh lexes as a command argument),
// a case pattern, or a here-document body.
func closeBraceExemptWord(word *syntax.Word, stack []syntax.Node) bool {
	if len(stack) == 0 {
		return false
	}
	switch parent := stack[len(stack)-1].(type) {
	case *syntax.Assign:
		return !parent.Naked && parent.Value == word
	case *syntax.CaseItem:
		return true
	case *syntax.Redirect:
		return parent.Hdoc == word
	}
	return false
}

// trailingCloseBrace returns the position of word's last byte when that
// byte is an unquoted, unescaped `}` at brace depth zero, and false
// otherwise. Depth counts unescaped `{` and `}` in the word's unquoted
// literal parts; quoted and expansion parts contribute nothing, as they do
// not in Zsh's lexer.
func trailingCloseBrace(word *syntax.Word) (syntax.Pos, bool) {
	if len(word.Parts) == 0 {
		return syntax.Pos{}, false
	}
	last, ok := word.Parts[len(word.Parts)-1].(*syntax.Lit)
	if !ok || !strings.HasSuffix(last.Value, "}") {
		return syntax.Pos{}, false
	}
	depth := 0
	escaped := false
	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			continue
		}
		text := lit.Value
		if lit == last {
			text = text[:len(text)-1]
		}
		for i := 0; i < len(text); i++ {
			switch {
			case escaped:
				escaped = false
			case text[i] == '\\':
				escaped = true
			case text[i] == '{':
				depth++
			case text[i] == '}' && depth > 0:
				depth--
			}
		}
	}
	if escaped || depth > 0 {
		return syntax.Pos{}, false
	}
	offset := int(last.Pos().Offset()) + len(last.Value) - 1
	return syntax.NewPos(uint(offset), last.Pos().Line(), last.Pos().Col()+uint(len(last.Value))-1), true
}
