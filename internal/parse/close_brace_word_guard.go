package parse

import (
	"mvdan.cc/sh/v3/syntax"
)

// closeBraceWordError is the parser's own text for a `}` in a command's
// arguments, reused where mvdan/sh reads the byte as a word instead.
const closeBraceWordError = "`}` can only be used to close a block"

// rejectBareCloseBraceWords returns a parse error at the first word that is
// a bare `}` (issue #314). Zsh's lexer never reads an unquoted `}` as a
// word, so `for x in a }`, `x=( a } )` and `case } in` are parse errors
// natively; mvdan/sh (v3.14.1) rejects the byte only in a command's
// arguments and lexes it as an ordinary word in a loop's word list, an
// array value and a case word, and the select adapters inherit that
// through the parser's word lexer. A quoted or escaped `}`, a here-document
// line and a `}` glued to an expansion are other parts or longer literals
// and stay words, as they are for Zsh.
func rejectBareCloseBraceWords(tree *syntax.File, name string) error {
	var found error
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found != nil {
			return false
		}
		word, ok := node.(*syntax.Word)
		if !ok || len(word.Parts) != 1 {
			return true
		}
		lit, ok := word.Parts[0].(*syntax.Lit)
		if !ok || lit.Value != "}" {
			return true
		}
		found = syntax.ParseError{Filename: name, Pos: lit.Pos(), Text: closeBraceWordError}
		return false
	})
	return found
}
