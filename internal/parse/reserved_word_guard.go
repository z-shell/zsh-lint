package parse

import (
	"mvdan.cc/sh/v3/syntax"
)

// unsupportedLoopWords are Zsh reserved words that the front end does not
// recognise. In command position mvdan/sh reads them as ordinary command
// names. A silent tree of the wrong shape is worse than a parse error for
// every rule that reasons about loop bodies, so the front end fails closed
// until the constructs are supported. `repeat` left this list when the front end
// started reading the loop (#208; the parser fork reads it itself since #281);
// `foreach` left this list when the parser fork started reading it (#214).
var unsupportedLoopWords = map[string]string{}

// assignedRepeatError is reported for an assignment prefix before `repeat`.
const assignedRepeatError = "`repeat` cannot follow an assignment"

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
		// The parser fork reads `repeat` in command position as the loop
		// (#281); after an assignment prefix it is still the reserved word,
		// which native Zsh rejects there, so the call it became is an error.
		if lit.Value == "repeat" && len(call.Assigns) > 0 {
			text, unsupported = assignedRepeatError, true
		}
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
