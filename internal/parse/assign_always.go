package parse

import (
	"bytes"
	"errors"

	"mvdan.cc/sh/v3/syntax"
)

// invalidAssignAlwaysOperator is the exact error mvdan/sh (LangZsh, v3.14.1)
// reports for the native `${name::=word}` operator: it reads `::` as an
// empty-offset slice and then fails on `=word` as the length expression.
const invalidAssignAlwaysOperator = "`=` must follow a name"

// parseAssignAlways retries only the native-Zsh unconditional assignment
// operator `${name::=word}` (zshexpn, Parameter Expansion). mvdan/sh has no
// operator for it, so the retry masks the middle `:` with `=`, which the
// parser reads as the conditional `${name:==word}`: the `:=` operator it
// already accepts, followed by a word that starts with the original `=`. The
// adapter then removes that leading `=` from the word so its text and every
// position match the original source. The typed tree still says `:=`; Parse
// records the owning expansion as File metadata (see
// bindAssignAlwaysExpansions) because the AST has no field for `::=`.
func parseAssignAlways(src []byte, name string, firstErr error) (*syntax.File, error) {
	return parseAssignAlwaysWithParser(src, name, firstErr, parseWithAdapters)
}

func parseAssignAlwaysWithParser(
	src []byte,
	name string,
	firstErr error,
	parse func([]byte, string) (*syntax.File, error),
) (*syntax.File, error) {
	var parseErr syntax.ParseError
	if !errors.As(firstErr, &parseErr) || parseErr.Text != invalidAssignAlwaysOperator {
		return nil, firstErr
	}

	// The error lands on the `=`. Require exactly `::=` before it and a
	// parameter name (or a closing subscript) before that, so `${::=word}`,
	// which native Zsh rejects, keeps its parser error.
	equals := int(parseErr.Pos.Offset())
	if equals < 3 || equals >= len(src) ||
		src[equals] != '=' || src[equals-1] != ':' || src[equals-2] != ':' ||
		(!isIdentByte(src[equals-3]) && src[equals-3] != ']') {
		return nil, firstErr
	}

	masked := bytes.Clone(src)
	masked[equals-1] = '='

	tree, err := parse(masked, name)
	if err != nil {
		return nil, err
	}
	if !restoreAssignAlwaysWord(tree, equals) {
		return nil, firstErr
	}
	return tree, nil
}

// restoreAssignAlwaysWord finds the expansion the mask produced, whose `:=`
// word begins with the original `=` at offset equals, and drops that byte
// from the word. A literal right-hand side loses its first byte; a
// non-literal one (`${x::=$y}`, `${x::="..."}`) loses a whole `=` literal
// part; an empty one (`${x::=}`) loses the word, matching how the parser
// represents `${x:=}`. It reports false when no expansion matches, so the
// caller returns the original parser error instead of an unverified tree.
func restoreAssignAlwaysWord(tree *syntax.File, equals int) bool {
	restored := false
	syntax.Walk(tree, func(node syntax.Node) bool {
		if restored {
			return false
		}
		exp, ok := node.(*syntax.ParamExp)
		if !ok || exp.Exp == nil || exp.Exp.Op != syntax.AssignUnsetOrNull || exp.Exp.Word == nil {
			return true
		}
		word := exp.Exp.Word
		if len(word.Parts) == 0 || int(word.Pos().Offset()) != equals {
			return true
		}
		lit, ok := word.Parts[0].(*syntax.Lit)
		if !ok || len(lit.Value) == 0 || lit.Value[0] != '=' || int(lit.ValuePos.Offset()) != equals {
			return true
		}
		switch {
		case len(lit.Value) > 1:
			lit.Value = lit.Value[1:]
			lit.ValuePos = syntax.NewPos(lit.ValuePos.Offset()+1, lit.ValuePos.Line(), lit.ValuePos.Col()+1)
		case len(word.Parts) > 1:
			word.Parts = word.Parts[1:]
		default:
			exp.Exp.Word = nil
		}
		restored = true
		return false
	})
	return restored
}

// bindAssignAlwaysExpansions returns every parameter expansion whose source
// operator is `::=`. The chain returns a bare tree, so an adapter nested in
// another adapter's retry cannot hand metadata back; Parse binds it once,
// from the tree and the original source, after the chain has succeeded. The
// three bytes before the assigned word (or before the closing brace when the
// word is empty) are the operator, so `::=` there identifies the construct
// and `x:=` in an ordinary `${x:=word}` does not.
func bindAssignAlwaysExpansions(tree *syntax.File, src []byte) []*syntax.ParamExp {
	var found []*syntax.ParamExp
	syntax.Walk(tree, func(node syntax.Node) bool {
		exp, ok := node.(*syntax.ParamExp)
		if !ok || exp.Exp == nil || exp.Exp.Op != syntax.AssignUnsetOrNull {
			return true
		}
		wordStart := int(exp.End().Offset()) - 1
		if exp.Exp.Word != nil {
			wordStart = int(exp.Exp.Word.Pos().Offset())
		}
		if wordStart >= 3 && wordStart <= len(src) && string(src[wordStart-3:wordStart]) == "::=" {
			found = append(found, exp)
		}
		return true
	})
	return found
}
