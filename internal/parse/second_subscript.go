package parse

import (
	"github.com/z-shell/zsh-lint/third_party/mvdan-sh/syntax"
)

// SecondSubscript pairs a parameter expansion with the subscripts written
// after its first one, `${name[a][b]}` (zshparam, Array Subscripts: a
// subscript may be applied to the result of a previous subscript). The
// parser fork keeps those subscripts in the tree as the expansion's typed
// LaterIndexes children with their original source positions (#215, #252),
// so syntax.Walk, and every analyzer pass that walks the tree, reaches each
// of them once. This is a read-only view of those children.
type SecondSubscript struct {
	Expansion  *syntax.ParamExp
	Subscripts []syntax.ArithmExpr
}

// collectSecondSubscripts lists every expansion with later subscripts in
// walk order, so an outer expansion comes before one nested in its
// subscripts.
func collectSecondSubscripts(tree *syntax.File) []SecondSubscript {
	var found []SecondSubscript
	syntax.Walk(tree, func(node syntax.Node) bool {
		if exp, ok := node.(*syntax.ParamExp); ok && len(exp.LaterIndexes) > 0 {
			found = append(found, SecondSubscript{Expansion: exp, Subscripts: exp.LaterIndexes})
		}
		return true
	})
	return found
}
