package parse

import (
	"mvdan.cc/sh/v3/syntax"
)

// unsupportedLoopWords are Zsh reserved words that the front end does not
// recognise. In command position mvdan/sh reads them as ordinary command
// names, so `foreach x (a b)` becomes a call named foreach. A silent tree of
// the wrong shape is worse than a parse error for every rule that reasons
// about loop bodies, so the front end fails closed until the constructs are
// supported. `repeat` left this list when resolveRepeatLoops (repeat.go)
// started rewriting the loop.
var unsupportedLoopWords = map[string]string{
	"foreach": "`foreach ... end` loops are not supported yet (z-shell/zsh-lint#214)",
}

// rejectUnsupportedLoopWords returns a parse error positioned at the first
// call whose command name is an unsupported reserved word.
func rejectUnsupportedLoopWords(tree *syntax.File, name string) error {
	var found error
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found != nil {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || len(call.Args[0].Parts) != 1 {
			return true
		}
		lit, ok := call.Args[0].Parts[0].(*syntax.Lit)
		if !ok {
			return true
		}
		text, unsupported := unsupportedLoopWords[lit.Value]
		if !unsupported {
			return true
		}
		found = syntax.ParseError{Filename: name, Pos: lit.Pos(), Text: text}
		return false
	})
	if found == nil {
		return nil
	}
	return found
}
