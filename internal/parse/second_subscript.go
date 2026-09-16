package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

// invalidSecondSubscript is the exact error mvdan/sh (LangZsh, v3.14.1)
// reports for a second subscript in a parameter expansion, `${name[a][b]}`:
// it closes the expansion's single index at the first `]` and then reads the
// next `[` where an expansion operator would be. Under the length and
// existence prefixes (`${#name[a][b]}`, `${+name[a][b]}`) the same `[` is
// reported as invalidSecondSubscriptPrefixed instead; the adapter tells the
// construct apart from other operators by the bytes at the reported position.
const (
	invalidSecondSubscript         = "not a valid parameter expansion operator: `[`"
	invalidSecondSubscriptPrefixed = "cannot combine multiple parameter expansion operators"
)

// SecondSubscript pairs a parameter expansion with the subscripts written
// after its first one, `${name[a][b]}` (zshparam, Array Subscripts: a
// subscript may be applied to the result of a previous subscript). mvdan/sh
// (through v3.14.1) has one Index field per expansion, so the compatibility
// front end keeps the first subscript there and retains the others here as
// typed arithmetic nodes with their original source positions, in source
// order.
type SecondSubscript struct {
	Expansion  *syntax.ParamExp
	Subscripts []syntax.ArithmExpr
}

// parseSecondSubscript retries only a native-Zsh second subscript in a
// parameter expansion. The retry masks the `][` boundary as `, ` so the
// parser reads both subscripts as one comma expression in the single index it
// has; a comma binds loosest of all arithmetic operators, so the operands on
// each side of the masked comma are exactly the two subscripts, and an
// expansion operator after the second `]` still parses in its own field.
// The masked bytes are not literal text, so nothing is restored here: Parse
// splits the comma expression back into its subscripts once the chain has
// succeeded (see bindSecondSubscripts), keyed on the original source bytes at
// the comma.
func parseSecondSubscript(src []byte, name string, firstErr error) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) ||
		(parseErr.Text != invalidSecondSubscript && parseErr.Text != invalidSecondSubscriptPrefixed) {
		return nil, firstErr
	}
	open := int(parseErr.Pos.Offset())
	if open < 1 || open >= len(src) || src[open] != '[' || src[open-1] != ']' {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[open-1] = ','
	masked[open] = ' '

	tree, err := parseWithAdapters(masked, name)
	if err != nil {
		return nil, err
	}
	if !hasSubscriptComma(tree, open-1) {
		return nil, firstErr
	}
	return tree, nil
}

// hasSubscriptComma reports whether some parameter expansion's index holds a
// top-level comma at byte offset comma whose left operand ends exactly there,
// which is the only tree the mask can legitimately produce.
func hasSubscriptComma(tree *syntax.File, comma int) bool {
	found := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if found {
			return false
		}
		exp, ok := node.(*syntax.ParamExp)
		if !ok || exp.Index == nil {
			return true
		}
		for _, bin := range commaSpine(exp.Index) {
			if int(bin.OpPos.Offset()) == comma && int(bin.X.End().Offset()) == comma {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// commaSpine returns the comma nodes of a left-associative comma expression,
// outermost first, so the last entry joins the first two operands.
func commaSpine(index syntax.ArithmExpr) []*syntax.BinaryArithm {
	var spine []*syntax.BinaryArithm
	for {
		bin, ok := index.(*syntax.BinaryArithm)
		if !ok || bin.Op != syntax.Comma {
			return spine
		}
		spine = append(spine, bin)
		index = bin.X
	}
}

// bindSecondSubscripts splits every index the mask joined and returns the
// detached subscripts with their owning expansions. A comma whose byte in the
// original source is `]` followed by `[` is a masked subscript boundary; a
// native range comma (`[1,50]`) keeps its `,` and stays in place. The first
// subscript is the left operand of the first masked comma, an existing node.
// Each later subscript reuses the parser's own comma nodes: a node whose
// left operand was the whole prefix is re-pointed at its group's first
// operand, so every node and position in the result comes from the parse.
func bindSecondSubscripts(tree *syntax.File, src []byte) []SecondSubscript {
	var found []SecondSubscript
	syntax.Walk(tree, func(node syntax.Node) bool {
		exp, ok := node.(*syntax.ParamExp)
		if !ok || exp.Index == nil {
			return true
		}
		spine := commaSpine(exp.Index)
		if len(spine) == 0 {
			return true
		}
		// Operands and their joining commas in source order.
		operands := make([]syntax.ArithmExpr, 0, len(spine)+1)
		commas := make([]*syntax.BinaryArithm, 0, len(spine))
		operands = append(operands, spine[len(spine)-1].X)
		for i := len(spine) - 1; i >= 0; i-- {
			operands = append(operands, spine[i].Y)
			commas = append(commas, spine[i])
		}
		var subscripts []syntax.ArithmExpr
		var first syntax.ArithmExpr
		groupStart := 0
		for i, comma := range commas {
			offset := int(comma.OpPos.Offset())
			if offset+1 >= len(src) || src[offset] != ']' || src[offset+1] != '[' {
				continue
			}
			group := foldSubscriptGroup(operands, commas, groupStart, i)
			if first == nil {
				first = group
			} else {
				subscripts = append(subscripts, group)
			}
			groupStart = i + 1
		}
		if first == nil {
			return true
		}
		subscripts = append(subscripts, foldSubscriptGroup(operands, commas, groupStart, len(commas)))
		exp.Index = first
		found = append(found, SecondSubscript{Expansion: exp, Subscripts: subscripts})
		return true
	})
	return found
}

// foldSubscriptGroup rebuilds operands[start:end+1] as one left-associative
// comma expression from the parser's own comma nodes commas[start:end].
func foldSubscriptGroup(operands []syntax.ArithmExpr, commas []*syntax.BinaryArithm, start, end int) syntax.ArithmExpr {
	group := operands[start]
	for i := start; i < end; i++ {
		commas[i].X = group
		group = commas[i]
	}
	return group
}
